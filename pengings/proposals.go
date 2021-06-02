package pengings

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/upgrade"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf"
	"github.com/idena-network/idena-go/log"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
	"sync"
	"time"
)

const (
	DeferFutureProposalsPeriod = 30
)

type Proposals struct {
	log             log.Logger
	chain           *blockchain.Blockchain
	offlineDetector *blockchain.OfflineDetector
	upgrader        *upgrade.Upgrader

	// proposed blocks are grouped by round
	blocksByRound *sync.Map

	pendingProofs *sync.Map
	pendingBlocks *sync.Map

	potentialForkedPeers mapset.Set

	proposeCache *cache.Cache
	// used for requesting blocks by hash from peers
	blockCache *cache.Cache
	appState   *appstate.AppState

	// proposals with worse proof can be skipped
	bestProofs      map[uint64]bestHash
	bestProofsMutex sync.RWMutex
}

type blockPeer struct {
	proposal      *types.BlockProposal
	receivingTime time.Time
	peerId        peer.ID
}

type proposedBlock struct {
	proposal      *types.BlockProposal
	receivingTime time.Time
}

type bestHash struct {
	common.Hash
	// modified by pool size representation of hash
	floatValue     *big.Float
	ProposerPubKey []byte
}

type ProposerByRound func(round uint64) (hash common.Hash, proposer []byte, ok bool)

func NewProposals(chain *blockchain.Blockchain, appState *appstate.AppState, detector *blockchain.OfflineDetector, upgrader *upgrade.Upgrader) (*Proposals, *sync.Map) {
	p := &Proposals{
		chain:                chain,
		appState:             appState,
		offlineDetector:      detector,
		upgrader:             upgrader,
		log:                  log.New(),
		blocksByRound:        &sync.Map{},
		pendingBlocks:        &sync.Map{},
		pendingProofs:        &sync.Map{},
		potentialForkedPeers: mapset.NewSet(),
		proposeCache:         cache.New(30*time.Second, 1*time.Minute),
		blockCache:           cache.New(time.Minute, time.Minute),
		bestProofs:           map[uint64]bestHash{},
	}
	return p, p.pendingProofs
}

func (proposals *Proposals) ProposerByRound(round uint64) (hash common.Hash, proposer []byte, ok bool) {
	proposals.bestProofsMutex.RLock()
	defer proposals.bestProofsMutex.RUnlock()
	bestHash, ok := proposals.bestProofs[round]
	if ok {
		return bestHash.Hash, bestHash.ProposerPubKey, ok
	}
	return common.Hash{}, nil, false
}

func (proposals *Proposals) AddProposeProof(proposal *types.ProofProposal) (added bool, pending bool) {
	currentRound := proposals.chain.Round()

	h, err := vrf.HashFromProof(proposal.Proof)
	if err != nil {
		return false, false
	}
	hash := common.Hash(h)

	if proposal.Round == currentRound {
		if proposals.proposeCache.Add(hash.Hex(), nil, cache.DefaultExpiration) != nil {
			return false, false
		}

		pubKeyBytes, err := types.ProofProposalPubKey(proposal)
		if err != nil {
			return false, false
		}

		pubKey, err := crypto.UnmarshalPubkey(pubKeyBytes)
		if err != nil {
			return false, false
		}

		proposerAddr := crypto.PubkeyToAddress(*pubKey)

		modifier := 1

		if proposals.appState.ValidatorsCache.IsPool(proposerAddr) {
			modifier = proposals.appState.ValidatorsCache.PoolSize(proposerAddr)
		}

		if !proposals.compareWithBestHash(currentRound, hash, modifier) {
			return false, false
		}

		if err := proposals.chain.ValidateProposerProof(proposal.Proof, pubKeyBytes); err != nil {
			log.Warn("Failed proposed proof validation", "err", err)
			return false, false
		}

		proposals.setBestHash(currentRound, hash, pubKeyBytes, modifier)

		return true, false
	} else if currentRound < proposal.Round && proposal.Round-currentRound < DeferFutureProposalsPeriod {
		proposals.pendingProofs.LoadOrStore(hash, proposal)
		return false, true
	}
	return false, false
}

func (proposals *Proposals) GetProposerPubKey(round uint64) []byte {

	proposals.bestProofsMutex.RLock()
	defer proposals.bestProofsMutex.RUnlock()
	bestHash, ok := proposals.bestProofs[round]
	if ok {
		return bestHash.ProposerPubKey
	}
	return nil
}

func (proposals *Proposals) CompleteRound(height uint64) {

	proposals.blocksByRound.Range(func(key, value interface{}) bool {
		if key.(uint64) <= height {
			proposals.blocksByRound.Delete(key)
		}
		return true
	})

	proposals.bestProofsMutex.Lock()
	for round, _ := range proposals.bestProofs {
		if round <= height {
			delete(proposals.bestProofs, round)
		}
	}
	proposals.bestProofsMutex.Unlock()
}

func (proposals *Proposals) ProcessPendingProofs() []*types.ProofProposal {
	var result []*types.ProofProposal

	proposals.pendingProofs.Range(func(key, value interface{}) bool {
		proof := value.(*types.ProofProposal)
		if added, pending := proposals.AddProposeProof(proof); added {
			result = append(result, proof)
		} else if !pending {
			proposals.pendingProofs.Delete(key)
		}

		return true
	})

	return result
}

func (proposals *Proposals) ProcessPendingBlocks() []*types.BlockProposal {
	var result []*types.BlockProposal

	checkState, err := proposals.appState.ForCheck(proposals.chain.Head.Height())
	if err != nil {
		proposals.log.Warn("failed to create checkState", "err", err)
	}
	proposals.pendingBlocks.Range(func(key, value interface{}) bool {

		blockPeer := value.(*blockPeer)
		if added, pending := proposals.AddProposedBlock(blockPeer.proposal, blockPeer.peerId, blockPeer.receivingTime, checkState); added {
			result = append(result, blockPeer.proposal)
		} else if !pending {
			proposals.pendingBlocks.Delete(key)
		}
		if checkState != nil {
			checkState.Reset()
		}
		return true
	})

	return result
}

func (proposals *Proposals) AddProposedBlock(proposal *types.BlockProposal, peerId peer.ID, receivingTime time.Time, checkState *appstate.AppState) (added bool, pending bool) {
	block := proposal.Block
	currentRound := proposals.chain.Round()
	if currentRound == block.Height() {
		if proposals.proposeCache.Add(block.Hash().Hex(), nil, cache.DefaultExpiration) != nil {
			return false, false
		}

		h, err := vrf.HashFromProof(proposal.Proof)
		if err != nil {
			return false, false
		}
		vrfHash := common.Hash(h)

		pubKey, err := crypto.UnmarshalPubkey(block.Header.ProposedHeader.ProposerPubKey)
		if err != nil {
			return false, false
		}

		proposerAddr := crypto.PubkeyToAddress(*pubKey)

		modifier := 1
		if proposals.appState.ValidatorsCache.IsPool(proposerAddr) {
			modifier = proposals.appState.ValidatorsCache.PoolSize(proposerAddr)
		}

		if !proposals.compareWithBestHash(currentRound, vrfHash, modifier) {
			return false, false
		}

		if err := proposals.chain.ValidateProposerProof(proposal.Proof, block.Header.ProposedHeader.ProposerPubKey); err != nil {
			log.Warn("Failed proposed block proof validation", "err", err)
			return false, false
		}

		m, _ := proposals.blocksByRound.LoadOrStore(block.Height(), &sync.Map{})
		round := m.(*sync.Map)

		if _, ok := round.Load(block.Hash()); ok {
			return false, false
		}
		if _, _, _, err := proposals.chain.ValidateBlock(block, checkState); err != nil {
			log.Warn("Failed proposed block validation", "err", err)
			// it might be a signal about a fork
			if err == blockchain.ParentHashIsInvalid && peerId != "" {
				proposals.potentialForkedPeers.Add(peerId)
			}
			return false, false
		}

		if err := proposals.offlineDetector.ValidateBlock(proposals.chain.Head, block); err != nil {
			log.Warn("Failed block offline proposing", "err", err.Error())
			return false, false
		}
		if err := proposals.upgrader.ValidateBlock(block); err != nil {
			log.Warn("Failed block upgrading proposing", "err", err.Error())
			return false, false
		}

		round.Store(block.Hash(), &proposedBlock{proposal: proposal, receivingTime: receivingTime})
		proposals.setBestHash(currentRound, vrfHash, block.Header.ProposedHeader.ProposerPubKey, modifier)
		return true, false
	} else if currentRound < block.Height() && block.Height()-currentRound < DeferFutureProposalsPeriod {
		proposals.pendingBlocks.LoadOrStore(block.Hash(), &blockPeer{
			proposal: proposal, peerId: peerId, receivingTime: receivingTime,
		})
		return false, true
	}
	return false, false
}

func (proposals *Proposals) GetProposedBlock(round uint64, proposerPubKey []byte, timeout time.Duration) (*types.Block, error) {
	for start := time.Now(); time.Since(start) < timeout; {
		m, ok := proposals.blocksByRound.Load(round)
		if ok {
			round := m.(*sync.Map)
			var result *types.Block
			round.Range(func(key, value interface{}) bool {
				p := value.(*proposedBlock)
				if bytes.Compare(p.proposal.Block.Header.ProposedHeader.ProposerPubKey, proposerPubKey) == 0 {
					result = p.proposal.Block
					return false
				}
				return true
			})
			if result != nil {
				return result, nil
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
	return nil, errors.New("Proposed block was not found")
}
func (proposals *Proposals) GetBlockByHash(round uint64, hash common.Hash) (*types.Block, error) {
	if m, ok := proposals.blocksByRound.Load(round); ok {
		roundMap := m.(*sync.Map)
		if block, ok := roundMap.Load(hash); ok {
			return block.(*proposedBlock).proposal.Block, nil
		}
	}
	return nil, errors.New("Block is not found in proposals")
}

func (proposals *Proposals) HasPotentialFork() bool {
	return proposals.potentialForkedPeers.Cardinality() > 0
}

func (proposals *Proposals) GetForkedPeers() mapset.Set {
	return proposals.potentialForkedPeers
}

func (proposals *Proposals) ClearPotentialForks() {
	proposals.potentialForkedPeers.Clear()
}

func (proposals *Proposals) AvgTimeDiff(round uint64, start int64) decimal.Decimal {
	var diffs []decimal.Decimal
	m, ok := proposals.blocksByRound.Load(round)
	if ok {
		round := m.(*sync.Map)
		round.Range(func(key, value interface{}) bool {
			block := value.(*proposedBlock)
			diffs = append(diffs, decimal.New(block.receivingTime.Unix(), 0).Sub(decimal.New(block.proposal.Header.Time(), 0)))
			return true
		})
	}
	if len(diffs) == 0 {
		return decimal.Zero
	}
	return decimal.Avg(diffs[0], diffs[1:]...)
}

// mark block as approved for adding to blockCache
func (proposals *Proposals) ApproveBlock(hash common.Hash) {
	proposals.blockCache.Add(hash.Hex(), nil, cache.DefaultExpiration)
}

func (proposals *Proposals) AddBlock(block *types.Block) {
	if block == nil {
		return
	}
	if _, ok := proposals.blockCache.Get(block.Hash().Hex()); ok {
		proposals.blockCache.Set(block.Hash().Hex(), block, cache.DefaultExpiration)
	}
}

func (proposals *Proposals) GetBlock(hash common.Hash) *types.Block {
	block, _ := proposals.blockCache.Get(hash.Hex())
	if block == nil {
		return nil
	}
	return block.(*types.Block)
}

func (proposals *Proposals) compareWithBestHash(round uint64, hash common.Hash, modifier int) bool {
	proposals.bestProofsMutex.RLock()
	defer proposals.bestProofsMutex.RUnlock()

	q := common.HashToFloat(hash, int64(modifier))

	if bestHash, ok := proposals.bestProofs[round]; ok {
		return q.Cmp(bestHash.floatValue) >= 0
	}
	return true
}

func (proposals *Proposals) setBestHash(round uint64, hash common.Hash, proposerPubKey []byte, modifier int) {
	proposals.bestProofsMutex.Lock()
	defer proposals.bestProofsMutex.Unlock()
	q := common.HashToFloat(hash, int64(modifier))

	if stored, ok := proposals.bestProofs[round]; ok {
		if q.Cmp(stored.floatValue) >= 0 {
			proposals.bestProofs[round] = bestHash{
				hash, q, proposerPubKey,
			}
		}
	} else {
		proposals.bestProofs[round] = bestHash{
			hash, q, proposerPubKey,
		}
	}
}

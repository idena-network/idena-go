package pengings

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"sort"
	"sync"
	"time"
)

type Proof struct {
	Proof  []byte
	Hash   common.Hash
	PubKey []byte
	Round  uint64
}

type Proposals struct {
	log             log.Logger
	chain           *blockchain.Blockchain
	offlineDetector *blockchain.OfflineDetector

	// proofs of proposers which are valid for current round
	proofsByRound *sync.Map

	// proofs for future rounds
	//pendingProofs []*Proof

	newPendingProofs *sync.Map
	newPendingBlocks *sync.Map

	// proposed blocks are grouped by round
	blocksByRound *sync.Map

	// blocks for future rounds
	//pendingBlocks []*blockPeer

	// lock for pendingProofs
	pMutex *sync.Mutex

	// lock for pendingBlocks
	bMutex *sync.Mutex

	potentialForkedPeers mapset.Set
}

type blockPeer struct {
	block  *types.Block
	peerId string
}

func NewProposals(chain *blockchain.Blockchain, detector *blockchain.OfflineDetector) *Proposals {
	return &Proposals{
		chain:                chain,
		offlineDetector:      detector,
		log:                  log.New(),
		proofsByRound:        &sync.Map{},
		blocksByRound:        &sync.Map{},
		newPendingBlocks:     &sync.Map{},
		newPendingProofs:     &sync.Map{},
		bMutex:               &sync.Mutex{},
		pMutex:               &sync.Mutex{},
		potentialForkedPeers: mapset.NewSet(),
	}
}

func (proposals *Proposals) AddProposeProof(p []byte, hash common.Hash, pubKey []byte, round uint64) (added bool, pending bool) {

	proof := &Proof{
		p,
		hash,
		pubKey,
		round,
	}
	if round == proposals.chain.Round() {

		m, _ := proposals.proofsByRound.LoadOrStore(round, &sync.Map{})
		byRound := m.(*sync.Map)
		if _, ok := byRound.Load(hash); ok {
			return false, false
		}

		if err := proposals.chain.ValidateProposerProof(p, hash, pubKey); err != nil {
			log.Warn("Failed Proof proposer validation", "err", err.Error())
			return false, false
		}
		// TODO: maybe there exists better structure for this
		byRound.Store(hash, proof)
		return true, false
	} else if round > proposals.chain.Round() {

		proposals.pMutex.Lock()
		defer proposals.pMutex.Unlock()
		log.Warn("PENDING PROOF")
		// TODO: maybe there exists better structure for this
		proposals.newPendingProofs.LoadOrStore(proof.Hash, proof)
		return false, true
		//proposals.pendingProofs = append(proposals.pendingProofs, proof)
	}
	return false, false
}

func (proposals *Proposals) GetProposerPubKey(round uint64) []byte {
	m, ok := proposals.proofsByRound.Load(round)
	if ok {

		byRound := m.(*sync.Map)
		var list []common.Hash
		byRound.Range(func(key, value interface{}) bool {
			list = append(list, key.(common.Hash))
			return true
		})
		if len(list) == 0 {
			return nil
		}

		sort.SliceStable(list, func(i, j int) bool {
			return bytes.Compare(list[i][:], list[j][:]) < 0
		})

		v, ok := byRound.Load(list[0])
		if ok {
			return v.(*Proof).PubKey
		}
	}
	return nil
}

func (proposals *Proposals) CompleteRound(height uint64) {
	proposals.proofsByRound.Range(func(key, value interface{}) bool {
		if key.(uint64) <= height {
			proposals.proofsByRound.Delete(key)
		}
		return true
	})

	proposals.blocksByRound.Range(func(key, value interface{}) bool {
		if key.(uint64) <= height {
			proposals.blocksByRound.Delete(key)
		}
		return true
	})
}

func (proposals *Proposals) ProcessPendingsProofs() []*Proof {
	var result []*Proof

	proposals.newPendingProofs.Range(func(key, value interface{}) bool {
		proof := value.(*Proof)
		if added, pending := proposals.AddProposeProof(proof.Proof, proof.Hash, proof.PubKey, proof.Round); added {
			result = append(result, proof)
		} else if !pending {
			proposals.newPendingProofs.Delete(proof)
		}

		return true
	})

	return result
}

func (proposals *Proposals) ProcessPendingsBlocks() []*types.Block {
	var result []*types.Block

	proposals.newPendingBlocks.Range(func(key, value interface{}) bool {
		blockPeer := value.(*blockPeer)
		if added, pending := proposals.AddProposedBlock(blockPeer.block, blockPeer.peerId); added {
			result = append(result, blockPeer.block)
		} else if !pending {
			proposals.newPendingBlocks.Delete(blockPeer.block.Hash())
		}

		return true
	})

	return result
}

func (proposals *Proposals) AddProposedBlock(block *types.Block, peerId string) (added bool, pending bool) {
	currentRound := proposals.chain.Round()
	if currentRound == block.Height() {

		m, _ := proposals.blocksByRound.LoadOrStore(block.Height(), &sync.Map{})
		round := m.(*sync.Map)

		if _, ok := round.Load(block.Hash()); ok {
			return false, false
		}

		log.Warn("VALIDATE BLOCK", "currentRound", currentRound, "block", block.Height(), "hash", block.Hash())
		if err := proposals.chain.ValidateBlock(block, nil); err != nil {
			log.Warn("Failed proposed block validation", "err", err.Error())
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

		round.Store(block.Hash(), block)

		return true, false
	} else if currentRound < block.Height() {
		proposals.bMutex.Lock()
		defer proposals.bMutex.Unlock()
		log.Warn("PENDING BLOCK", "currentRound", currentRound, "block", block.Height())
		// TODO: maybe there exists better structure for this
		proposals.newPendingBlocks.LoadOrStore(block.Hash(), &blockPeer{
			block: block, peerId: peerId,
		})
		//proposals.pendingBlocks = append(proposals.pendingBlocks, &blockPeer{
		//	block: block, peerId: peerId,
		//})
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
				block := value.(*types.Block)
				if bytes.Compare(block.Header.ProposedHeader.ProposerPubKey, proposerPubKey) == 0 {
					result = block
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
			return block.(*types.Block), nil
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

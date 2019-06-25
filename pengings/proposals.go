package pengings

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/pkg/errors"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/log"
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
	log   log.Logger
	chain *blockchain.Blockchain

	// proofs of proposers which are valid for current round
	proofsByRound *sync.Map

	// proofs for future rounds
	pendingProofs []*Proof

	// proposed blocks are grouped by round
	blocksByRound *sync.Map

	// blocks for future rounds
	pendingBlocks []*blockPeer

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

func NewProposals(chain *blockchain.Blockchain) *Proposals {
	return &Proposals{
		chain:                chain,
		log:                  log.New(),
		proofsByRound:        &sync.Map{},
		blocksByRound:        &sync.Map{},
		bMutex:               &sync.Mutex{},
		pMutex:               &sync.Mutex{},
		potentialForkedPeers: mapset.NewSet(),
	}
}

func (proposals *Proposals) AddProposeProof(p []byte, hash common.Hash, pubKey []byte, round uint64) bool {

	proof := &Proof{
		p,
		hash,
		pubKey,
		round,
	}
	if round == proposals.chain.Round() {
		if err := proposals.chain.ValidateProposerProof(p, hash, pubKey); err != nil {
			log.Warn("Failed Proof proposer validation", "err", err.Error())
			return false
		}
		// TODO: maybe there exists better structure for this

		m, _ := proposals.proofsByRound.LoadOrStore(round, &sync.Map{})

		byRound := m.(*sync.Map)
		byRound.Store(hash, proof)
		return true
	} else if round > proposals.chain.Round() {

		proposals.pMutex.Lock()
		defer proposals.pMutex.Unlock()

		// TODO: maybe there exists better structure for this
		proposals.pendingProofs = append(proposals.pendingProofs, proof)
	}
	return false
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
	proposals.pMutex.Lock()

	var result []*Proof

	pendings := proposals.pendingProofs
	proposals.pendingProofs = []*Proof{}

	proposals.pMutex.Unlock()
	for _, proof := range pendings {
		if proposals.AddProposeProof(proof.Proof, proof.Hash, proof.PubKey, proof.Round) {
			result = append(result, proof)
		}
	}
	return result
}

func (proposals *Proposals) ProcessPendingsBlocks() []*types.Block {
	proposals.bMutex.Lock()
	var result []*types.Block
	pendings := proposals.pendingBlocks
	proposals.pendingBlocks = []*blockPeer{}

	proposals.bMutex.Unlock()

	for _, pending := range pendings {
		if proposals.AddProposedBlock(pending.block, pending.peerId) {
			result = append(result, pending.block)
		}
	}
	return result
}

func (proposals *Proposals) AddProposedBlock(block *types.Block, peerId string) bool {
	currentRound := proposals.chain.Round()
	if currentRound == block.Height() {
		if err := proposals.chain.ValidateBlock(block, nil); err != nil {
			log.Warn("Failed proposed block validation", "err", err.Error())
			// it might be a signal about a fork
			if err == blockchain.ParentHashIsInvalid && peerId != "" {
				proposals.potentialForkedPeers.Add(peerId)
			}
			return false
		}
		m, _ := proposals.blocksByRound.LoadOrStore(block.Height(), &sync.Map{})

		round := m.(*sync.Map)
		round.Store(block.Hash(), block)

		return true
	} else {
		proposals.bMutex.Lock()
		defer proposals.bMutex.Unlock()

		// TODO: maybe there exists better structure for this
		proposals.pendingBlocks = append(proposals.pendingBlocks, &blockPeer{
			block: block, peerId: peerId,
		})
		return false
	}
	return false
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

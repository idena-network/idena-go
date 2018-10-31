package consensus

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/pkg/errors"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/config"
	"idena-go/core/state"
	"idena-go/core/validators"
	"idena-go/crypto"
	"idena-go/log"
	"idena-go/pengings"
	"idena-go/protocol"
	"time"
)

const (
	ReductionOne = 998
	ReductionTwo = 999
	Final        = 1000
)

type Engine struct {
	chain      *blockchain.Blockchain
	pm         *protocol.ProtocolManager
	log        log.Logger
	state      string
	pubKey     []byte
	config     *config.ConsensusConf
	proposals  *pengings.Proposals
	stateDb    state.Database
	validators *validators.ValidatorsSet
	secretKey  *ecdsa.PrivateKey
	votes      *pengings.Votes
}

func NewEngine(chain *blockchain.Blockchain, pm *protocol.ProtocolManager, proposals *pengings.Proposals, config *config.ConsensusConf,
	stateDb state.Database,
	validators *validators.ValidatorsSet,
	votes *pengings.Votes) *Engine {
	return &Engine{
		chain:      chain,
		pm:         pm,
		log:        log.New(),
		config:     config,
		proposals:  proposals,
		stateDb:    stateDb,
		validators: validators,
		votes:      votes,
	}
}

func (engine *Engine) SetKey(key *ecdsa.PrivateKey) {
	engine.secretKey = key
	engine.pubKey = crypto.FromECDSAPub(key.Public().(*ecdsa.PublicKey))
}

func (engine *Engine) Start() {
	log.Info("Start consensus protocol", "pubKey", hexutil.Encode(engine.pubKey))
	go engine.cycle()
}

func (engine *Engine) cycle() {
	for {
		engine.syncBlockchain()

		head := engine.chain.Head
		round := head.Height() + 1
		engine.log.Info(fmt.Sprintf("Start round %v", round))

		engine.state = "Check if I'm proposer"

		isProposer, proposerHash, proposerProof := engine.chain.GetProposerSortition()

		var block *types.Block
		if isProposer {
			engine.state = "Propose block"
			block = engine.proposeBlock(proposerHash, proposerProof)
			engine.log.Info("Selected as proposer", "block", block.Hash().Hex(), "round", round)
		}

		engine.state = "Calculating highest-priority pubkey"

		proposerPubKey := engine.getHighestProposerPubKey(round)

		var proposer string
		if proposer = hexutil.Encode(proposerPubKey); len(proposerPubKey) == 0 {
			proposer = "NOT FOUND"
		}

		engine.log.Info("Selected proposer", "proposer", proposer)

		if proposerPubKey == nil {
			block = engine.chain.GenerateEmptyBlock()
		} else {

			engine.state = "Waiting for block from proposer"
			block = engine.waitForBlock(proposerPubKey)

			if block == nil {
				block = engine.chain.GenerateEmptyBlock()
			}
		}

		engine.reduction(round, block)

	}
}

func (engine *Engine) proposeBlock(hash common.Hash, proof []byte) *types.Block {

	block := engine.chain.ProposeBlock(hash, proof)
	engine.pm.ProposeProof(block.Height(), hash, proof, engine.pubKey)
	engine.pm.ProposeBlock(block)

	engine.proposals.AddProposedBlock(block)
	engine.proposals.AddProposeProof(proof, hash, engine.pubKey, block.Height())

	return block
}

func (engine *Engine) getHighestProposerPubKey(round uint64) []byte {
	time.Sleep(engine.config.EstimatedBaVariance + engine.config.WaitSortitionProofDelay)
	return engine.proposals.GetProposerPubKey(round)
}

func (engine *Engine) waitForBlock(proposerPubKey []byte) *types.Block {
	engine.log.Info("Wait for block proposal")
	block, err := engine.proposals.GetProposedBlock(engine.chain.Round(), proposerPubKey, engine.config.WaitBlockDelay)
	if err != nil {
		engine.log.Error("Proposed block is not found", err, err.Error())
		return nil
	}
	return block
}

func (engine *Engine) syncBlockchain() {

	for {
		bestHeight, peerId, err := engine.pm.GetBestHeight()

		head := engine.chain.Head
		if err != nil {
			log.Info("We have peers but don't know their heads. Go to sleep")
			time.Sleep(time.Second * 10)
		} else if bestHeight > head.Height() {
			engine.loadFromPeer(peerId, bestHeight)
		} else {
			engine.log.Info(fmt.Sprintf("Node is synced %v", peerId))
			break
		}
	}
}

func (engine *Engine) loadFromPeer(peerId string, height uint64) {

	engine.log.Info(fmt.Sprintf("Start loading blocks from %v", peerId))
	head := engine.chain.Head
	for h := head.Height() + 1; h <= height; h++ {
		block := engine.pm.GetBlockFromPeer(peerId, h)
		if block == nil {
			engine.log.Warn("Can't load block by height. Try resync")
			time.Sleep(time.Second * 5)
			return
		} else {
			if err := engine.chain.AddBlock(block); err != nil {
				engine.log.Warn(fmt.Sprintf("Block from peer %v is invalid: %v", peerId, err))
				return
			}
		}
	}
}

func (engine *Engine) reduction(round uint64, block *types.Block) common.Hash {
	engine.state = "Reduction started"
	engine.log.Info("Reduction started", "block", block.Hash().Hex())

	engine.vote(round, ReductionOne, block.Hash())

	engine.state = fmt.Sprintf("Reduction %v vote commited", ReductionOne)
	hash, err := engine.countVotes(round, ReductionOne, block.Header.ParentHash(), engine.getCommitteeVotesTreshold(), engine.config.WaitForStepDelay)
	engine.state = fmt.Sprintf("Reduction %v votes counted", ReductionOne)

	emptyBlock := engine.chain.GenerateEmptyBlock()

	if err != nil {
		hash := emptyBlock.Hash()
		fmt.Println(hash.Hex())
	}
	engine.vote(round, ReductionTwo, hash)

	engine.state = fmt.Sprintf("Reduction %v vote commited", ReductionTwo)
	hash, err = engine.countVotes(round, ReductionTwo, block.Header.ParentHash(), engine.getCommitteeVotesTreshold(), engine.config.WaitForStepDelay)
	engine.state = fmt.Sprintf("Reduction %v votes counted", ReductionTwo)

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.log.Info("Reduction completed", "block", hash.Hex(), "isEmpty", hash == emptyBlock.Hash())
	return hash
}

func (engine *Engine) vote(round uint64, step uint16, block common.Hash) {

	committeeSize := engine.GetCommitteSize()
	if engine.validators.GetActualValidators(engine.chain.Head.Seed(), round, step, committeeSize).Contains(engine.pubKey) ||
		committeeSize == 0 {
		vote := types.Vote{
			Header: &types.VoteHeader{
				Round:      round,
				Step:       step,
				ParentHash: engine.chain.Head.Hash(),
				VotedHash:  block,
			},
		}
		h := vote.Hash()
		vote.Signature, _ = crypto.Sign(h[:], engine.secretKey)
		engine.pm.SendVote(&vote)

		engine.log.Info("Voted for", "block", block.Hex())

		if !engine.votes.AddVote(&vote) {
			engine.log.Info("Invalid vote", "vote", vote.Hash().Hex())
		}
	}
}

func (engine *Engine) countVotes(round uint64, step uint16, parentHash common.Hash, necessaryVotesCount int, timeout time.Duration) (common.Hash, error) {

	byBlock := make(map[common.Hash]mapset.Set)
	validators := engine.validators.GetActualValidators(engine.chain.Head.Seed(), round, step, engine.GetCommitteSize())

	for start := time.Now(); time.Since(start) < timeout; {
		m := engine.votes.GetVotesOfRound(round)
		if m != nil {

			found := false
			var bestHash common.Hash

			m.Range(func(key, value interface{}) bool {
				vote := value.(*types.Vote)

				roundVotes, ok := byBlock[vote.Header.VotedHash]
				if !ok {
					roundVotes = mapset.NewSet()
					byBlock[vote.Header.VotedHash] = roundVotes
				}

				if !roundVotes.Contains(vote.Hash()) {
					if vote.Header.ParentHash != parentHash {
						return true
					}
					if !validators.Contains(vote.VoterAddr()) && validators.Cardinality() > 0 {
						return true
					}
					roundVotes.Add(vote.Hash())

					if roundVotes.Cardinality() >= necessaryVotesCount {
						found = true
						engine.log.Info(fmt.Sprintf("Has %v/%v votes", roundVotes.Cardinality(), necessaryVotesCount))
						bestHash = vote.Header.VotedHash
						return false
					}
				}
				return true
			})

			if found {
				return bestHash, nil
			}
		}
	}
	return common.Hash{}, errors.New(fmt.Sprintf("votes for step is not received, step=%v", step))
}

func (engine *Engine) GetCommitteSize() int {
	var cnt = engine.validators.GetCountOfValidNodes()

	switch cnt {
	case 1:
		return 1
	case 2:
	case 3:
		return 2
	case 4:
	case 5:
		return 3
	case 6:
		return 4
	case 7:
	case 8:
		return 5
	}
	return int(float64(cnt) * engine.config.CommitteePercent)
}

func (engine *Engine) getCommitteeVotesTreshold() int {

	var cnt = engine.validators.GetCountOfValidNodes()

	switch cnt {
	case 1:
		return 1
	case 2:
	case 3:
		return 2
	case 4:
	case 5:
		return 3
	case 6:
		return 4
	case 7:
	case 8:
		return 5
	}
	return int(float64(cnt) * engine.config.CommitteePercent * engine.config.ThesholdBa)
}

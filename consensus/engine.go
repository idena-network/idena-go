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
	"idena-go/core/mempool"
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
	txpool     *mempool.TxPool
	addr       common.Address
}

func NewEngine(chain *blockchain.Blockchain, pm *protocol.ProtocolManager, proposals *pengings.Proposals, config *config.ConsensusConf,
	stateDb state.Database,
	validators *validators.ValidatorsSet,
	votes *pengings.Votes,
	txpool *mempool.TxPool) *Engine {
	return &Engine{
		chain:      chain,
		pm:         pm,
		log:        log.New(),
		config:     config,
		proposals:  proposals,
		stateDb:    stateDb,
		validators: validators,
		votes:      votes,
		txpool:     txpool,
	}
}

func (engine *Engine) SetKey(key *ecdsa.PrivateKey) {
	engine.secretKey = key
	engine.pubKey = crypto.FromECDSAPub(key.Public().(*ecdsa.PublicKey))
	engine.addr = crypto.PubkeyToAddress(*key.Public().(*ecdsa.PublicKey))
}

func (engine *Engine) Start() {
	log.Info("Start consensus protocol", "pubKey", hexutil.Encode(engine.pubKey))
	go engine.loop()
}

func (engine *Engine) loop() {
	for {
		engine.syncBlockchain()
		if !engine.config.Automine && !engine.pm.HasPeers() {
			time.Sleep(time.Second * 5)
			continue
		}
		engine.requestApprove()
		head := engine.chain.Head
		round := head.Height() + 1
		engine.log.Info("Start loop", "round", round, "head", head.Hash().Hex())

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
		emptyBlock := engine.chain.GenerateEmptyBlock()
		if proposerPubKey == nil {
			block = emptyBlock
		} else {

			engine.state = "Waiting for block from proposer"
			block = engine.waitForBlock(proposerPubKey)

			if block == nil {
				block = emptyBlock
			}
		}

		blockHash := engine.reduction(round, block)
		var err error
		blockHash, err = engine.binaryBa(blockHash)
		if err != nil {
			engine.log.Info("Binary Ba is failed", "err", err)
		}
		engine.state = "Count final votes"

		hash, err := engine.countVotes(round, Final, block.Header.ParentHash(), engine.getCommitteeVotesTreshold(true), engine.config.WaitForStepDelay)

		if blockHash == emptyBlock.Hash() {
			engine.chain.AddBlock(emptyBlock)
			engine.log.Info("Reached consensus on empty block")
		} else {
			block, err := engine.getBlockByHash(round, blockHash)
			if err == nil {
				engine.chain.AddBlock(block)
				if hash == blockHash {
					engine.log.Info("Reached FINAL", "block", blockHash.Hex())
					engine.chain.WriteFinalConsensus(blockHash)
				} else {
					engine.log.Info("Reached TENTATIVE", "block", blockHash.Hex())
				}
			} else {
				engine.log.Warn("Confirmed block is not found", "block", blockHash.Hex())
			}
		}

		for _, proof := range engine.proposals.ProcessPendingsProofs() {
			engine.pm.ProposeProof(proof.Round, proof.Hash, proof.Proof, proof.PubKey)
		}
		engine.log.Info("Pending proposals processed")
		for _, block := range engine.proposals.ProcessPendingsBlocks() {
			engine.pm.ProposeBlock(block)
		}
		engine.log.Info("Pending blocks processed")
	}
}

func (engine *Engine) proposeBlock(hash common.Hash, proof []byte) *types.Block {

	block := engine.chain.ProposeBlock(hash, proof)

	engine.log.Info("Proposed block", "block", block.Hash().Hex(), "txs", len(block.Body.Transactions))

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

	hash, err := engine.countVotes(round, ReductionOne, block.Header.ParentHash(), engine.getCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
	engine.state = fmt.Sprintf("Reduction %v votes counted", ReductionOne)

	emptyBlock := engine.chain.GenerateEmptyBlock()

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.vote(round, ReductionTwo, hash)

	engine.state = fmt.Sprintf("Reduction %v vote commited", ReductionTwo)
	hash, err = engine.countVotes(round, ReductionTwo, block.Header.ParentHash(), engine.getCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
	engine.state = fmt.Sprintf("Reduction %v votes counted", ReductionTwo)

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.log.Info("Reduction completed", "block", hash.Hex(), "isEmpty", hash == emptyBlock.Hash())
	return hash
}

func (engine *Engine) binaryBa(blockHash common.Hash) (common.Hash, error) {
	engine.log.Info("binaryBa started", "block", blockHash.Hex())
	emptyBlock := engine.chain.GenerateEmptyBlock()

	emptyBlockHash := emptyBlock.Hash()

	round := emptyBlock.Height()
	hash := blockHash

	for step := uint16(1); step < engine.config.MaxSteps; {
		engine.state = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, err := engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.getCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
		if err != nil {
			hash = emptyBlockHash
		} else if hash != emptyBlockHash {
			for i := uint16(1); i <= 3; i++ {
				engine.vote(round, step+i, hash)
			}
			if step == 1 {
				engine.vote(round, Final, hash)
			}
			return hash, nil
		}
		step++

		engine.state = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, err = engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.getCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)

		if err != nil {
			hash = emptyBlockHash
		} else if hash == emptyBlockHash {
			for i := uint16(1); i <= 3; i++ {
				engine.vote(round, step+i, hash)
			}
			return hash, nil
		}

		step++

		engine.state = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, err = engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.getCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
		if err != nil {
			if engine.commonCoin(step) {
				hash = blockHash
			} else {
				hash = emptyBlockHash
			}
		}
		step++
	}
	return common.Hash{}, errors.New("No consensus")
}

func (engine *Engine) commonCoin(step uint16) bool {
	data := append(engine.chain.Head.Seed().Bytes(), common.ToBytes(step)...)

	h := crypto.Keccak256Hash(data)
	return h[31]%2 == 0
}

func (engine *Engine) vote(round uint64, step uint16, block common.Hash) {
	committeeSize := engine.GetCommitteSize(step == Final)
	stepValidators := engine.validators.GetActualValidators(engine.chain.Head.Seed(), round, step, committeeSize)
	if stepValidators == nil {
		return
	}
	if stepValidators.Contains(engine.addr) ||
		committeeSize == 0 {
		vote := types.Vote{
			Header: &types.VoteHeader{
				Round:      round,
				Step:       step,
				ParentHash: engine.chain.Head.Hash(),
				VotedHash:  block,
			},
		}
		vote.Signature, _ = crypto.Sign(vote.Header.SignatureHash().Bytes(), engine.secretKey)
		engine.pm.SendVote(&vote)

		engine.log.Info("Voted for", "step", step, "block", block.Hex())

		if !engine.votes.AddVote(&vote) {
			engine.log.Info("Invalid vote", "vote", vote.Hash().Hex())
		}
	}
}

func (engine *Engine) countVotes(round uint64, step uint16, parentHash common.Hash, necessaryVotesCount int, timeout time.Duration) (common.Hash, error) {

	engine.log.Info("Start count votes", "step", step, "min-votes", necessaryVotesCount)
	defer engine.log.Info("Finish count votes", "step", step)

	byBlock := make(map[common.Hash]mapset.Set)
	validators := engine.validators.GetActualValidators(engine.chain.Head.Seed(), round, step, engine.GetCommitteSize(step == Final))
	if validators == nil {
		return common.Hash{}, errors.New(fmt.Sprintf("validators were not setup, step=%v", step))
	}

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
					if vote.Header.Step != step {
						return true
					}
					roundVotes.Add(vote.Hash())

					if roundVotes.Cardinality() >= necessaryVotesCount {
						found = true
						engine.log.Info(fmt.Sprintf("Has %v/%v votes step=%v", roundVotes.Cardinality(), necessaryVotesCount, step))
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

func (engine *Engine) GetCommitteSize(final bool) int {
	var cnt = engine.validators.GetCountOfValidNodes()
	percent := engine.config.CommitteePercent
	if final {
		percent = engine.config.FinalCommitteeConsensusPercent
	}
	switch cnt {
	case 1, 2,3,4:
		return 1
		return 2
	case 5, 6:
		return 2
	case 7, 8:
		return 3
	}
	return int(float64(cnt) * percent)
}

func (engine *Engine) getCommitteeVotesTreshold(final bool) int {

	var cnt = engine.validators.GetCountOfValidNodes()
	percent := engine.config.CommitteePercent
	if final {
		percent = engine.config.FinalCommitteeConsensusPercent
	}

	switch cnt {
	case 1, 2,3,4:
		return 1
		return 2
	case 5, 6:
		return 2
	case 7, 8:
		return 3
	}
	return int(float64(cnt) * percent * engine.config.ThesholdBa)
}

func (engine *Engine) getBlockByHash(round uint64, hash common.Hash) (*types.Block, error) {
	block, err := engine.proposals.GetBlockByHash(round, hash)
	if err == nil {
		return block, nil
	}
	engine.pm.RequestBlockByHash(hash)

	for start := time.Now(); time.Since(start) < engine.config.WaitBlockDelay; {
		block, err := engine.proposals.GetBlockByHash(round, hash)
		if err == nil {
			return block, nil
		} else {
			time.Sleep(200)
		}
	}

	return nil, errors.New("Block is not found")
}
func (engine *Engine) requestApprove() {

	if engine.validators.Contains(engine.pubKey) {
		return
	}

	tx := &types.Transaction{
		PubKey: engine.pubKey,
	}
	tx.Signature, _ = crypto.Sign(tx.Hash().Bytes(), engine.secretKey)

	engine.txpool.Add(tx)
}

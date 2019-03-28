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
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/crypto"
	"idena-go/ipfs"
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
	process    string
	pubKey     []byte
	config     *config.ConsensusConf
	proposals  *pengings.Proposals
	secretKey  *ecdsa.PrivateKey
	votes      *pengings.Votes
	txpool     *mempool.TxPool
	addr       common.Address
	appState   *appstate.AppState
	downloader *protocol.Downloader
}

func NewEngine(chain *blockchain.Blockchain, pm *protocol.ProtocolManager, proposals *pengings.Proposals, config *config.ConsensusConf,
	appState *appstate.AppState,
	votes *pengings.Votes,
	txpool *mempool.TxPool, ipfs ipfs.Proxy) *Engine {
	return &Engine{
		chain:      chain,
		pm:         pm,
		log:        log.New(),
		config:     config,
		proposals:  proposals,
		appState:   appState,
		votes:      votes,
		txpool:     txpool,
		downloader: protocol.NewDownloader(pm, chain, ipfs),
	}
}

func (engine *Engine) SetKey(key *ecdsa.PrivateKey) {
	engine.secretKey = key
	engine.pubKey = crypto.FromECDSAPub(key.Public().(*ecdsa.PublicKey))
	engine.addr = crypto.PubkeyToAddress(*key.Public().(*ecdsa.PublicKey))
}

func (engine *Engine) GetKey() *ecdsa.PrivateKey {
	return engine.secretKey
}

func (engine *Engine) Start() {
	log.Info("Start consensus protocol", "pubKey", hexutil.Encode(engine.pubKey))
	go engine.loop()
}

func (engine *Engine) GetProcess() string {
	return engine.process
}

func (engine *Engine) GetAppState() *appstate.AppState {
	return engine.appState.ForCheck(engine.chain.Head.Height())
}

func (engine *Engine) loop() {
	for {
		if err := engine.ensureIntegrity(); err != nil {
			engine.log.Error("Failed to recover stateDb", "err", err)
			time.Sleep(time.Second * 30)
			continue
		}
		engine.downloader.SyncBlockchain()

		if !engine.config.Automine && !engine.pm.HasPeers() {
			time.Sleep(time.Second * 5)
			continue
		}
		head := engine.chain.Head
		round := head.Height() + 1
		engine.log.Info("Start loop", "round", round, "head", head.Hash().Hex(), "peers", engine.pm.PeersCount(), "valid-nodes", engine.appState.ValidatorsCache.GetCountOfValidNodes())

		engine.process = "Check if I'm proposer"

		isProposer, proposerHash, proposerProof := engine.chain.GetProposerSortition()

		var block *types.Block
		if isProposer {
			engine.process = "Propose block"
			block = engine.proposeBlock(proposerHash, proposerProof)
			if block != nil {
				engine.log.Info("Selected as proposer", "block", block.Hash().Hex(), "round", round)
			}
		}

		engine.process = "Calculating highest-priority pubkey"

		proposerPubKey := engine.getHighestProposerPubKey(round)

		proposer := engine.fmtProposer(proposerPubKey)

		engine.log.Info("Selected proposer", "proposer", proposer)
		emptyBlock := engine.chain.GenerateEmptyBlock()
		if proposerPubKey == nil {
			block = emptyBlock
		} else {

			engine.process = "Waiting for block from proposer"
			block = engine.waitForBlock(proposerPubKey)

			if block == nil {
				block = emptyBlock
			}
		}

		blockHash := engine.reduction(round, block)
		blockHash, err := engine.binaryBa(blockHash)
		if err != nil {
			engine.log.Info("Binary Ba is failed", "err", err)
			continue
		}
		engine.process = "Count final votes"
		var hash common.Hash
		var cert *types.BlockCert
		if blockHash != emptyBlock.Hash() {
			hash, cert, _ = engine.countVotes(round, Final, block.Header.ParentHash(), engine.chain.GetCommitteeVotesTreshold(true), engine.config.WaitForStepDelay)
		}
		if blockHash == emptyBlock.Hash() {
			if err := engine.chain.AddBlock(emptyBlock); err != nil {
				engine.log.Error("Add empty block", "err", err)
				continue
			}
			engine.log.Info("Reached consensus on empty block")
		} else {
			block, err := engine.getBlockByHash(round, blockHash)
			if err == nil {
				if err := engine.chain.AddBlock(block); err != nil {
					engine.log.Error("Add block", "err", err)
					continue
				}
				if hash == blockHash {
					engine.log.Info("Reached FINAL", "block", blockHash.Hex(), "txs", len(block.Body.Transactions))
					engine.chain.WriteFinalConsensus(blockHash, cert)
				} else {
					engine.log.Info("Reached TENTATIVE", "block", blockHash.Hex(), "txs", len(block.Body.Transactions))
				}
			} else {
				engine.log.Warn("Confirmed block is not found", "block", blockHash.Hex())
			}
		}

		engine.completeRound(round)
	}
}

func (engine *Engine) fmtProposer(proposerPubKey []byte) string {
	var proposer string
	if proposer = hexutil.Encode(proposerPubKey); len(proposerPubKey) == 0 {
		proposer = "NOT FOUND"
	} else {
		addr, _ := crypto.PubKeyBytesToAddress(proposerPubKey)
		proposer = addr.Hex()
	}
	return proposer
}

func (engine *Engine) completeRound(round uint64) {

	engine.proposals.CompleteRound(round)

	for _, proof := range engine.proposals.ProcessPendingsProofs() {
		engine.pm.ProposeProof(proof.Round, proof.Hash, proof.Proof, proof.PubKey)
	}
	engine.log.Debug("Pending proposals processed")
	for _, block := range engine.proposals.ProcessPendingsBlocks() {
		engine.pm.ProposeBlock(block)
	}
	engine.log.Debug("Pending blocks processed")

	engine.votes.CompleteRound(round)
}

func (engine *Engine) proposeBlock(hash common.Hash, proof []byte) *types.Block {
	block := engine.chain.ProposeBlock()

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

func (engine *Engine) reduction(round uint64, block *types.Block) common.Hash {
	engine.process = "Reduction started"
	engine.log.Info("Reduction started", "block", block.Hash().Hex())

	engine.vote(round, ReductionOne, block.Hash())
	engine.process = fmt.Sprintf("Reduction %v vote commited", ReductionOne)

	hash, _, err := engine.countVotes(round, ReductionOne, block.Header.ParentHash(), engine.chain.GetCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
	engine.process = fmt.Sprintf("Reduction %v votes counted", ReductionOne)

	emptyBlock := engine.chain.GenerateEmptyBlock()

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.vote(round, ReductionTwo, hash)

	engine.process = fmt.Sprintf("Reduction %v vote commited", ReductionTwo)
	hash, _, err = engine.countVotes(round, ReductionTwo, block.Header.ParentHash(), engine.chain.GetCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
	engine.process = fmt.Sprintf("Reduction %v votes counted", ReductionTwo)

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
		engine.process = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, _, err := engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
		if err != nil {
			hash = blockHash
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

		engine.process = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, _, err = engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)

		if err != nil {
			hash = emptyBlockHash
		} else if hash == emptyBlockHash {
			for i := uint16(1); i <= 3; i++ {
				engine.vote(round, step+i, hash)
			}
			return hash, nil
		}

		step++

		engine.process = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, _, err = engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesTreshold(false), engine.config.WaitForStepDelay)
		if err != nil {
			if engine.commonCoin(step) {
				hash = blockHash
			} else {
				hash = emptyBlockHash
			}
		}
		step++

		if engine.votes.FutureBlockExist(round, engine.chain.GetCommitteeVotesTreshold(false)) {
			return common.Hash{}, errors.New("Detected future block")
		}
	}
	return common.Hash{}, errors.New("No consensus")
}

func (engine *Engine) commonCoin(step uint16) bool {
	data := append(engine.chain.Head.Seed().Bytes(), common.ToBytes(step)...)

	h := crypto.Keccak256Hash(data)
	return h[31]%2 == 0
}

func (engine *Engine) vote(round uint64, step uint16, block common.Hash) {
	committeeSize := engine.chain.GetCommitteSize(step == Final)
	stepValidators := engine.appState.ValidatorsCache.GetActualValidators(engine.chain.Head.Seed(), round, step, committeeSize)
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

func (engine *Engine) countVotes(round uint64, step uint16, parentHash common.Hash, necessaryVotesCount int, timeout time.Duration) (common.Hash, *types.BlockCert, error) {

	engine.log.Debug("Start count votes", "step", step, "min-votes", necessaryVotesCount)
	defer engine.log.Debug("Finish count votes", "step", step)

	byBlock := make(map[common.Hash]mapset.Set)
	validators := engine.appState.ValidatorsCache.GetActualValidators(engine.chain.Head.Seed(), round, step, engine.chain.GetCommitteSize(step == Final))
	if validators == nil {
		return common.Hash{}, nil, errors.Errorf("validators were not setup, step=%v", step)
	}

	for start := time.Now(); time.Since(start) < timeout; {
		m := engine.votes.GetVotesOfRound(round)
		if m != nil {

			found := false
			var bestHash common.Hash
			var cert types.BlockCert
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
						list := make([]*types.Vote, 0, necessaryVotesCount)
						roundVotes.Each(func(value interface{}) bool {
							v := engine.votes.GetVoteByHash(value.(common.Hash))
							if v != nil {
								list = append(list, v)
							}
							return len(list) >= necessaryVotesCount
						})
						cert = types.BlockCert(list)
						bestHash = vote.Header.VotedHash
						found = cert.Len() >= necessaryVotesCount
						engine.log.Debug("Has votes", "cnt", roundVotes.Cardinality(), "need", necessaryVotesCount, "step", step, "hash", bestHash.Hex())
						return !found
					}
				}
				return true
			})

			if found {
				return bestHash, &cert, nil
			}
		}
		time.Sleep(200)
	}
	return common.Hash{}, nil, errors.New(fmt.Sprintf("votes for step is not received, step=%v", step))
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

func (engine *Engine) ensureIntegrity() error {
	for engine.chain.Head.Root() != engine.appState.State.Root() ||
		engine.chain.Head.IdentityRoot() != engine.appState.IdentityState.Root() {
		if err := engine.appState.ResetTo(engine.chain.Head.Height() - 1); err != nil {
			return errors.WithMessage(err, "state is corrupted, try to resync from scratch")
		}
		engine.chain.SetHead(engine.chain.Head.Height() - 1)
		engine.log.Warn("blockchain was reseted", "new head", engine.chain.Head.Height())
	}
	return nil
}

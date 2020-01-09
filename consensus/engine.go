package consensus

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/pengings"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/secstore"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	math2 "math"
	"time"
)

const (
	MaxStoredAvgTimeDiffs = 20
)

var (
	ForkDetected = errors.New("fork is detected")
)

type Engine struct {
	chain             *blockchain.Blockchain
	pm                *protocol.IdenaGossipHandler
	log               log.Logger
	process           string
	pubKey            []byte
	config            *config.ConsensusConf
	proposals         *pengings.Proposals
	votes             *pengings.Votes
	txpool            *mempool.TxPool
	addr              common.Address
	appState          *appstate.AppState
	downloader        *protocol.Downloader
	secStore          *secstore.SecStore
	peekingBlocks     chan *types.Block
	forkResolver      *ForkResolver
	offlineDetector   *blockchain.OfflineDetector
	prevRoundDuration time.Duration
	avgTimeDiffs      []decimal.Decimal
	timeDrift         time.Duration
	synced            bool
	nextBlockDetector *nextBlockDetector
}

func NewEngine(chain *blockchain.Blockchain, gossipHandler *protocol.IdenaGossipHandler, proposals *pengings.Proposals, config *config.ConsensusConf,
	appState *appstate.AppState,
	votes *pengings.Votes,
	txpool *mempool.TxPool, secStore *secstore.SecStore, downloader *protocol.Downloader,
	offlineDetector *blockchain.OfflineDetector) *Engine {
	return &Engine{
		chain:             chain,
		pm:                gossipHandler,
		log:               log.New(),
		config:            config,
		proposals:         proposals,
		appState:          appState,
		votes:             votes,
		txpool:            txpool,
		downloader:        downloader,
		secStore:          secStore,
		forkResolver:      NewForkResolver([]ForkDetector{proposals, downloader}, downloader, chain),
		offlineDetector:   offlineDetector,
		nextBlockDetector: newNextBlockDetector(gossipHandler, downloader, chain),
	}
}

func (engine *Engine) Start() {
	engine.pubKey = engine.secStore.GetPubKey()
	engine.addr = engine.secStore.GetAddress()
	log.Info("Start consensus protocol", "pubKey", hexutil.Encode(engine.pubKey))
	engine.forkResolver.Start()
	go engine.loop()
	go engine.ntpTimeDriftUpdate()
}

func (engine *Engine) GetProcess() string {
	return engine.process
}

func (engine *Engine) GetAppState() *appstate.AppState {
	return engine.appState.Readonly(engine.chain.Head.Height())
}

func (engine *Engine) alignTime() {
	if engine.prevRoundDuration > engine.config.MinBlockDistance {
		return
	}

	now := time.Now().UTC()
	var offset time.Duration
	if len(engine.avgTimeDiffs) > 0 {
		f, _ := decimal.Avg(engine.avgTimeDiffs[0], engine.avgTimeDiffs[1:]...).Float64()
		offset = time.Duration(f * float64(time.Second))
		if (offset < 0 && engine.timeDrift < 0 || offset > 0 && engine.timeDrift > 0) && math2.Abs(float64(engine.timeDrift-offset)) < float64(time.Second*2) {
			offset = (offset + engine.timeDrift) / 2
		} else {
			offset = 0
		}
	}
	correctedNow := now.Add(-offset)
	headTime := time.Unix(engine.chain.Head.Time().Int64(), 0)

	if correctedNow.After(headTime) {
		maxDelay := engine.config.MinBlockDistance - engine.config.EstimatedBaVariance - engine.config.WaitSortitionProofDelay
		diff := engine.config.MinBlockDistance - correctedNow.Sub(headTime)
		diff = time.Duration(math.MinInt(int(diff), int(maxDelay)))
		if diff > 0 {
			time.Sleep(diff)
		}
	}
}

func (engine *Engine) calculateTimeDiff(round uint64, roundStart time.Time) {
	engine.avgTimeDiffs = append(engine.avgTimeDiffs, engine.proposals.AvgTimeDiff(round, roundStart.Unix()))
	if len(engine.avgTimeDiffs) > MaxStoredAvgTimeDiffs {
		engine.avgTimeDiffs = engine.avgTimeDiffs[1:]
	}
}

func (engine *Engine) loop() {
	for {
		if err := engine.chain.EnsureIntegrity(); err != nil {
			engine.log.Error("Failed to recover blockchain", "err", err)
			time.Sleep(time.Second * 30)
			continue
		}
		if err := engine.downloader.SyncBlockchain(engine.forkResolver); err != nil {
			engine.synced = false
			if engine.forkResolver.HasLoadedFork() {
				engine.forkResolver.ApplyFork()
			} else {
				engine.log.Warn("syncing error", "err", err)
				time.Sleep(time.Second * 5)
			}
			continue
		}

		if !engine.config.Automine && !engine.pm.HasPeers() {
			time.Sleep(time.Second * 5)
			engine.synced = false
			continue
		}
		engine.synced = true
		head := engine.chain.Head

		engine.alignTime()

		engine.prevRoundDuration = 0
		roundStart := time.Now().UTC()

		round := head.Height() + 1
		engine.log.Info("Start loop", "round", round, "head", head.Hash().Hex(), "peers",
			engine.pm.PeersCount(), "online-nodes", engine.appState.ValidatorsCache.OnlineSize(),
			"network", engine.appState.ValidatorsCache.NetworkSize())

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
		engine.calculateTimeDiff(round, roundStart)
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
		blockHash, cert, err := engine.binaryBa(blockHash)
		if err != nil {
			engine.log.Info("Binary Ba is failed", "err", err)

			if err == ForkDetected {
				if err = engine.forkResolver.ApplyFork(); err != nil {
					engine.log.Error("error occurred during applying of fork", "err", err)
				}
			}
			continue
		}
		engine.process = "Count final votes"
		var hash common.Hash
		var finalCert *types.BlockCert
		if blockHash != emptyBlock.Hash() {
			hash, finalCert, _ = engine.countVotes(round, types.Final, block.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, true), engine.config.WaitForStepDelay)
		}
		if blockHash == emptyBlock.Hash() {
			if err := engine.chain.AddBlock(emptyBlock, nil); err != nil {
				engine.log.Error("Add empty block", "err", err)
				continue
			}

			engine.chain.WriteCertificate(blockHash, cert, engine.chain.IsPermanentCert(emptyBlock.Header))
			engine.log.Info("Reached consensus on empty block")
		} else {
			block, err := engine.getBlockByHash(round, blockHash)
			if err == nil {
				if err := engine.chain.AddBlock(block, nil); err != nil {
					engine.log.Error("Add block", "err", err)
					continue
				}
				if hash == blockHash {
					engine.log.Info("Reached FINAL", "block", blockHash.Hex(), "txs", len(block.Body.Transactions))
					engine.chain.WriteFinalConsensus(blockHash)
					cert = finalCert
				} else {
					engine.log.Info("Reached TENTATIVE", "block", blockHash.Hex(), "txs", len(block.Body.Transactions))
				}
				engine.chain.WriteCertificate(blockHash, cert, engine.chain.IsPermanentCert(block.Header))
			} else {
				engine.log.Warn("Confirmed block is not found", "block", blockHash.Hex())
			}
		}
		engine.completeRound(round)
		engine.prevRoundDuration = time.Now().UTC().Sub(roundStart)
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

	for _, proof := range engine.proposals.ProcessPendingProofs() {
		engine.pm.ProposeProof(proof.Round, proof.Hash, proof.Proof, proof.PubKey)
	}
	engine.log.Debug("Pending proposals processed")
	for _, block := range engine.proposals.ProcessPendingBlocks() {
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

	engine.proposals.AddProposedBlock(block, "", time.Now().UTC())
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
		engine.log.Error("Proposed block is not found", "err", err.Error())
		return nil
	}
	return block
}

func (engine *Engine) reduction(round uint64, block *types.Block) common.Hash {
	engine.process = "Reduction started"
	engine.log.Info("Reduction started", "block", block.Hash().Hex())

	engine.vote(round, types.ReductionOne, block.Hash())
	engine.process = fmt.Sprintf("Reduction %v vote commited", types.ReductionOne)

	hash, _, err := engine.countVotes(round, types.ReductionOne, block.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.config.WaitForStepDelay)
	engine.process = fmt.Sprintf("Reduction %v votes counted", types.ReductionOne)

	emptyBlock := engine.chain.GenerateEmptyBlock()

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.vote(round, types.ReductionTwo, hash)

	engine.process = fmt.Sprintf("Reduction %v vote commited", types.ReductionTwo)
	hash, _, err = engine.countVotes(round, types.ReductionTwo, block.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.config.WaitForStepDelay)
	engine.process = fmt.Sprintf("Reduction %v votes counted", types.ReductionTwo)

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.log.Info("Reduction completed", "block", hash.Hex(), "isEmpty", hash == emptyBlock.Hash())
	return hash
}

func (engine *Engine) completeBA() {
	engine.nextBlockDetector.complete()
}

func (engine *Engine) binaryBa(blockHash common.Hash) (common.Hash, *types.BlockCert, error) {
	defer engine.completeBA()
	engine.log.Info("binaryBa started", "block", blockHash.Hex())
	emptyBlock := engine.chain.GenerateEmptyBlock()

	emptyBlockHash := emptyBlock.Hash()

	round := emptyBlock.Height()
	hash := blockHash

	for step := uint16(1); step < engine.config.MaxSteps; {
		engine.process = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, cert, err := engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.config.WaitForStepDelay)
		if err != nil {
			hash = blockHash
		} else if hash != emptyBlockHash {
			for i := uint16(1); i <= 2; i++ {
				engine.vote(round, step+i, hash)
			}
			if step == 1 {
				engine.vote(round, types.Final, hash)
			}
			return hash, cert, nil
		}
		step++

		engine.process = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, cert, err = engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.config.WaitForStepDelay)

		if err != nil {
			hash = emptyBlockHash
		} else if hash == emptyBlockHash {
			for i := uint16(1); i <= 2; i++ {
				engine.vote(round, step+i, hash)
			}
			return hash, cert, nil
		}

		step++

		if engine.nextBlockDetector.nextBlockExist(round, emptyBlockHash) {
			return common.Hash{}, nil, errors.New("Detected future block")
		}
		if engine.forkResolver.HasLoadedFork() {
			return common.Hash{}, nil, ForkDetected
		}
	}
	return common.Hash{}, nil, errors.New("No consensus")
}

func (engine *Engine) vote(round uint64, step uint16, block common.Hash) {
	committeeSize := engine.chain.GetCommitteeSize(engine.appState.ValidatorsCache, step == types.Final)
	stepValidators := engine.appState.ValidatorsCache.GetOnlineValidators(engine.chain.Head.Seed(), round, step, committeeSize)
	if stepValidators == nil {
		return
	}
	if stepValidators.Contains(engine.addr) {
		vote := types.Vote{
			Header: &types.VoteHeader{
				Round:      round,
				Step:       step,
				ParentHash: engine.chain.Head.Hash(),
				VotedHash:  block,
			},
		}
		if b, err := engine.proposals.GetBlockByHash(round, block); err == nil {
			vote.Header.TurnOffline = engine.offlineDetector.VoteForOffline(b)
		}
		vote.Signature = engine.secStore.Sign(vote.Header.SignatureHash().Bytes())
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
	validators := engine.appState.ValidatorsCache.GetOnlineValidators(engine.chain.Head.Seed(), round, step, engine.chain.GetCommitteeSize(engine.appState.ValidatorsCache, step == types.Final))
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
					if !validators.Contains(vote.VoterAddr()) {
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
						cert = types.BlockCert{Votes: list}
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
		time.Sleep(100 * time.Millisecond)
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
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil, errors.New("Block is not found")
}

func (engine *Engine) ntpTimeDriftUpdate() {
	for {
		if drift, err := protocol.SntpDrift(3); err == nil {
			engine.timeDrift = drift
		}
		time.Sleep(time.Minute)
	}
}

func (engine *Engine) Synced() bool {
	return engine.synced
}

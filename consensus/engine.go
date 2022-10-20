package consensus

import (
	"fmt"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/upgrade"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/pengings"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	math2 "math"
	"sync"
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
	cfg               *config.Config
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
	ipfsProxy         ipfs.Proxy
	nextIpfsGC        time.Time
	prevRoundDuration time.Duration
	avgTimeDiffs      []decimal.Decimal
	timeDrift         time.Duration
	timeDriftMutex    sync.Mutex
	synced            bool
	nextBlockDetector *nextBlockDetector
	upgrader          *upgrade.Upgrader
	eventBus          eventbus.Bus
	statsCollector    collector.StatsCollector
}

func NewEngine(chain *blockchain.Blockchain, gossipHandler *protocol.IdenaGossipHandler, proposals *pengings.Proposals, config *config.Config,
	appState *appstate.AppState,
	votes *pengings.Votes,
	txpool *mempool.TxPool, secStore *secstore.SecStore, downloader *protocol.Downloader,
	offlineDetector *blockchain.OfflineDetector,
	upgrader *upgrade.Upgrader,
	ipfsProxy ipfs.Proxy,
	eventBus eventbus.Bus,
	statsCollector collector.StatsCollector) *Engine {
	return &Engine{
		chain:             chain,
		pm:                gossipHandler,
		log:               log.New(),
		cfg:               config,
		proposals:         proposals,
		appState:          appState,
		votes:             votes,
		txpool:            txpool,
		downloader:        downloader,
		secStore:          secStore,
		forkResolver:      NewForkResolver([]ForkDetector{proposals, downloader}, downloader, chain, statsCollector),
		offlineDetector:   offlineDetector,
		nextBlockDetector: newNextBlockDetector(gossipHandler, downloader, chain),
		upgrader:          upgrader,
		ipfsProxy:         ipfsProxy,
		eventBus:          eventBus,
		statsCollector:    statsCollector,
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

func (engine *Engine) ReadonlyAppState() (*appstate.AppState, error) {
	return engine.appState.Readonly(engine.chain.Head.Height())
}

func (engine *Engine) AppStateForCheck() (*appstate.AppState, error) {
	return engine.appState.ForCheck(engine.chain.Head.Height())
}

func (engine *Engine) alignTime() {
	if engine.prevRoundDuration > engine.cfg.Consensus.MinBlockDistance {
		return
	}

	now := time.Now().UTC()
	var offset time.Duration
	if len(engine.avgTimeDiffs) > 0 {
		f, _ := decimal.Avg(engine.avgTimeDiffs[0], engine.avgTimeDiffs[1:]...).Float64()
		offset = time.Duration(f * float64(time.Second))
		engine.timeDriftMutex.Lock()
		if (offset < 0 && engine.timeDrift < 0 || offset > 0 && engine.timeDrift > 0) && math2.Abs(float64(engine.timeDrift-offset)) < float64(time.Second*2) {
			offset = (offset + engine.timeDrift) / 2
		} else {
			offset = 0
		}
		engine.timeDriftMutex.Unlock()
	}
	correctedNow := now.Add(-offset)
	headTime := time.Unix(engine.chain.Head.Time(), 0)

	if correctedNow.After(headTime) {
		maxDelay := engine.cfg.Consensus.MinBlockDistance - engine.cfg.Consensus.EstimatedBaVariance - engine.cfg.Consensus.WaitSortitionProofDelay
		diff := engine.cfg.Consensus.MinBlockDistance - correctedNow.Sub(headTime)
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

		engine.maybeIpfsGC()

		if err := engine.downloader.SyncBlockchain(engine.forkResolver); err != nil {
			engine.synced = false
			if engine.forkResolver.HasLoadedFork() {
				if revertedTxs, err := engine.forkResolver.ApplyFork(); err == nil {
					if len(revertedTxs) > 0 {
						engine.txpool.AddExternalTxs(validation.MempoolTx, revertedTxs...)
					}
				} else {
					engine.log.Warn("fork apply error", "err", err)
				}
			} else {
				engine.log.Warn("syncing error", "err", err)
				time.Sleep(time.Second * 5)
			}
			continue
		}

		if !engine.cfg.Consensus.Automine && !engine.pm.HasPeers() {
			time.Sleep(time.Second * 5)
			engine.synced = false
			continue
		}
		if err := engine.checkOnlineStatus(); err != nil {
			log.Warn("error while trying to ensure online status", "err", err)
		}

		engine.synced = true
		head := engine.chain.Head

		startMiningTime := time.Date(2022, 10, 19, 0, 0, 0, 0, time.UTC)
		if diff := startMiningTime.Sub(time.Now().UTC()); diff > 0 {
			log.Info("sleep before mining due to hotfix", "duration", diff.String())
			time.Sleep(diff)
		}

		round := head.Height() + 1
		engine.completeRound(round - 1)

		engine.alignTime()

		engine.prevRoundDuration = 0
		roundStart := time.Now().UTC()

		shardId, _ := engine.chain.CoinbaseShard()
		engine.log.Info("Start loop", "round", round, "head", head.Hash().Hex(), "shardId", shardId, "p2p-shardId", engine.pm.OwnPeeringShardId(), "total-peers",
			engine.pm.PeersCount(), "own-shard-peers", engine.pm.OwnShardPeersCount(), "online-nodes", engine.appState.ValidatorsCache.OnlineSize(),
			"network", engine.appState.ValidatorsCache.NetworkSize())

		engine.process = "Check if I'm proposer"

		isProposer, proposerProof := engine.chain.GetProposerSortition()

		var block *types.Block
		if isProposer {
			engine.process = "Propose block"
			block = engine.proposeBlock(proposerProof)
			if block != nil {
				engine.log.Info("Selected as proposer", "block", block.Hash().Hex(), "round", round, "thresholdVrf", engine.appState.State.VrfProposerThreshold())
			}
		}

		engine.process = "Calculating highest-priority pubkey"

		proposerPubKey := engine.getHighestProposerPubKey(round)
		engine.calculateTimeDiff(round, roundStart)
		proposer := engine.fmtProposer(proposerPubKey)

		engine.log.Info("Selected proposer", "proposer", proposer)
		emptyBlock := engine.chain.GenerateEmptyBlock()

		var extraDelayForReductionOne time.Duration
		if proposerPubKey == nil {
			extraDelayForReductionOne = engine.cfg.Consensus.WaitBlockDelay
			block = emptyBlock
		} else {

			engine.process = "Waiting for block from proposer"
			block, extraDelayForReductionOne = engine.waitForBlock(proposerPubKey)

			if block == nil {
				block = emptyBlock
			}
		}

		blockHash := engine.reduction(round, block, extraDelayForReductionOne)
		blockHash, cert, err := engine.binaryBa(blockHash)
		if err != nil {
			engine.log.Info("Binary Ba is failed", "err", err)

			if err == ForkDetected {
				if revertedTxs, err := engine.forkResolver.ApplyFork(); err != nil {
					engine.log.Error("error occurred during applying of fork", "err", err)
				} else {
					if len(revertedTxs) > 0 {
						engine.txpool.AddExternalTxs(validation.MempoolTx, revertedTxs...)
					}
				}
			}
			continue
		}
		engine.process = "Count final votes"
		var hash common.Hash
		var finalCert *types.FullBlockCert
		if blockHash != emptyBlock.Hash() {
			hash, finalCert, err = engine.countVotes(round, types.Final, block.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, true), engine.cfg.Consensus.WaitForStepDelay)
			if err == nil && hash != blockHash {
				engine.log.Info("Switched to final", "prev", blockHash.Hex(), "final", hash.Hex())
				blockHash = hash
			}
		}
		if blockHash == emptyBlock.Hash() {
			if err := engine.chain.AddBlock(emptyBlock, nil, engine.statsCollector); err != nil {
				engine.log.Error("Add empty block", "err", err)
				continue
			}

			engine.chain.WriteCertificate(blockHash, cert.Compress(), engine.chain.IsPermanentCert(emptyBlock.Header))
			engine.log.Info("Reached consensus on empty block")
		} else {
			block, err := engine.getBlockByHash(round, blockHash)
			if err == nil {
				if err := engine.chain.AddBlock(block, nil, engine.statsCollector); err != nil {
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
				engine.chain.WriteCertificate(blockHash, cert.Compress(), engine.chain.IsPermanentCert(block.Header))
			} else {
				engine.log.Warn("Confirmed block is not found", "block", blockHash.Hex())
			}
		}
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
		engine.pm.ProposeProof(proof)
	}
	engine.log.Debug("Pending proposals processed")
	for _, block := range engine.proposals.ProcessPendingBlocks() {
		engine.pm.ProposeBlock(block)
	}
	engine.log.Debug("Pending blocks processed")

	engine.votes.CompleteRound(round)
}

func (engine *Engine) proposeBlock(proof []byte) *types.Block {
	proposal := engine.chain.ProposeBlock(proof)

	engine.log.Info("Proposed block", "block", proposal.Hash().Hex(), "txs", len(proposal.Body.Transactions))

	proofProposal := &types.ProofProposal{
		Proof: proof,
		Round: proposal.Height(),
	}

	hash := crypto.SignatureHash(proofProposal)
	proofProposal.Signature = engine.secStore.Sign(hash[:])

	engine.pm.ProposeProof(proofProposal)
	engine.pm.ProposeBlock(proposal)

	engine.proposals.AddProposedBlock(proposal, "", time.Now().UTC())
	engine.proposals.AddProposeProof(proofProposal)

	return proposal.Block
}

func (engine *Engine) getHighestProposerPubKey(round uint64) []byte {
	time.Sleep(engine.cfg.Consensus.EstimatedBaVariance + engine.cfg.Consensus.WaitSortitionProofDelay)
	return engine.proposals.GetProposerPubKey(round)
}

func (engine *Engine) waitForBlock(proposerPubKey []byte) (*types.Block, time.Duration) {
	engine.log.Info("Wait for block proposal")
	now := time.Now()
	block, err := engine.proposals.GetProposedBlock(engine.chain.Round(), proposerPubKey, engine.cfg.Consensus.WaitBlockDelay)
	notUsedDelay := engine.cfg.Consensus.WaitBlockDelay - time.Since(now)
	if err != nil {
		engine.log.Error("Proposed block is not found", "err", err.Error())
		return nil, notUsedDelay
	}
	return block, notUsedDelay
}

func (engine *Engine) reduction(round uint64, block *types.Block, extraDelayForReductionOne time.Duration) common.Hash {
	engine.process = "Reduction started"
	engine.log.Info("Reduction started", "block", block.Hash().Hex())

	engine.vote(round, types.ReductionOne, block.Hash())
	engine.process = fmt.Sprintf("Reduction %v vote commited", types.ReductionOne)

	hash, _, err := engine.countVotes(round, types.ReductionOne, block.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.cfg.Consensus.ReductionOneDelay+extraDelayForReductionOne)
	engine.process = fmt.Sprintf("Reduction %v votes counted", types.ReductionOne)

	emptyBlock := engine.chain.GenerateEmptyBlock()

	if err != nil {
		hash = emptyBlock.Hash()
	}
	engine.vote(round, types.ReductionTwo, hash)

	engine.process = fmt.Sprintf("Reduction %v vote commited", types.ReductionTwo)
	hash, _, err = engine.countVotes(round, types.ReductionTwo, block.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.cfg.Consensus.WaitForStepDelay)
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

func (engine *Engine) binaryBa(blockHash common.Hash) (common.Hash, *types.FullBlockCert, error) {
	defer engine.completeBA()
	engine.log.Info("binaryBa started", "block", blockHash.Hex())
	emptyBlock := engine.chain.GenerateEmptyBlock()

	emptyBlockHash := emptyBlock.Hash()

	round := emptyBlock.Height()
	hash := blockHash

	for step := uint8(1); step < engine.cfg.Consensus.MaxSteps; {
		engine.process = fmt.Sprintf("BA step %v", step)

		engine.vote(round, step, hash)

		hash, cert, err := engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.cfg.Consensus.WaitForStepDelay)
		if err != nil {
			hash = blockHash
		} else if hash != emptyBlockHash {
			for i := uint8(1); i <= 2; i++ {
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

		hash, cert, err = engine.countVotes(round, step, emptyBlock.Header.ParentHash(), engine.chain.GetCommitteeVotesThreshold(engine.appState.ValidatorsCache, false), engine.cfg.Consensus.WaitForStepDelay)

		if err != nil {
			hash = emptyBlockHash
		} else if hash == emptyBlockHash {
			for i := uint8(1); i <= 2; i++ {
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

func (engine *Engine) vote(round uint64, step uint8, block common.Hash) {
	committeeSize := engine.chain.GetCommitteeSize(engine.appState.ValidatorsCache, step == types.Final)
	stepValidators := engine.appState.ValidatorsCache.GetOnlineValidators(engine.chain.Head.Seed(), round, step, committeeSize)
	if stepValidators == nil {
		return
	}
	if stepValidators.CanVote(engine.addr) {
		vote := types.Vote{
			Header: &types.VoteHeader{
				Round:      round,
				Step:       step,
				ParentHash: engine.chain.Head.Hash(),
				VotedHash:  block,
				Upgrade:    engine.upgrader.UpgradeBits(),
			},
		}
		if b, err := engine.proposals.GetBlockByHash(round, block); err == nil {
			vote.Header.TurnOffline = engine.offlineDetector.VoteForOffline(b)
		}
		hash := crypto.SignatureHash(&vote)
		vote.Signature = engine.secStore.Sign(hash[:])
		engine.pm.SendVote(&vote)

		engine.log.Info("Voted for", "step", step, "block", block.Hex())

		if !engine.votes.AddVote(&vote) {
			engine.log.Info("Invalid vote", "vote", vote.Hash().Hex())
		}
	}
}

func (engine *Engine) countVotes(round uint64, step uint8, parentHash common.Hash, necessaryVotesCount int, timeout time.Duration) (common.Hash, *types.FullBlockCert, error) {

	engine.log.Debug("Start count votes", "step", step, "min-votes", necessaryVotesCount)
	defer engine.log.Debug("Finish count votes", "step", step)

	byBlock := make(map[common.Hash]map[common.Address]*types.Vote)
	validators := engine.appState.ValidatorsCache.GetOnlineValidators(engine.chain.Head.Seed(), round, step, engine.chain.GetCommitteeSize(engine.appState.ValidatorsCache, step == types.Final))
	if validators == nil {
		hash := common.Hash{}
		err := errors.Errorf("validators were not setup, step=%v", step)
		engine.statsCollector.SubmitVoteCountingResult(round, step, validators, hash, nil, err)
		return hash, nil, err
	}

	engine.offlineDetector.PushValidators(round, step, validators)

	necessaryVotesCount -= validators.VotesCountSubtrahend(engine.cfg.Consensus.AgreementThreshold)

	for start := time.Now(); time.Since(start) < timeout; {
		m := engine.votes.GetVotesOfRound(round)
		var checkedRoundVotes int
		if m != nil {

			found := false
			var bestHash common.Hash
			var cert types.FullBlockCert
			m.Range(func(key, value interface{}) bool {
				checkedRoundVotes++
				vote := value.(*types.Vote)

				roundVotes, ok := byBlock[vote.Header.VotedHash]
				if !ok {
					roundVotes = make(map[common.Address]*types.Vote)
					byBlock[vote.Header.VotedHash] = roundVotes
				}

				if _, ok := roundVotes[vote.VoterAddr()]; !ok {
					if vote.Header.ParentHash != parentHash {
						return true
					}
					if vote.Header.Step != step {
						return true
					}
					if !validators.Approved(vote.VoterAddr()) {
						return true
					}
					roundVotes[vote.VoterAddr()] = vote

					if len(roundVotes) >= necessaryVotesCount {
						list := make([]*types.Vote, 0, necessaryVotesCount)

						for _, v := range roundVotes {
							list = append(list, v)
							if len(list) >= necessaryVotesCount {
								break
							}
						}
						cert = types.FullBlockCert{Votes: list}
						bestHash = vote.Header.VotedHash
						found = len(cert.Votes) >= necessaryVotesCount
						engine.log.Debug("Has votes", "cnt", len(roundVotes), "need", necessaryVotesCount, "step", step, "hash", bestHash.Hex())
						return !found
					}
				}
				return true
			})

			engine.statsCollector.SubmitVoteCountingStepResult(round, step, byBlock, necessaryVotesCount, checkedRoundVotes)

			if found {
				engine.statsCollector.SubmitVoteCountingResult(round, step, validators, bestHash, &cert, nil)
				return bestHash, &cert, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	hash := common.Hash{}
	err := errors.New(fmt.Sprintf("votes for step is not received, step=%v", step))
	engine.statsCollector.SubmitVoteCountingResult(round, step, validators, hash, nil, err)
	return hash, nil, err
}

func (engine *Engine) getBlockByHash(round uint64, hash common.Hash) (*types.Block, error) {
	block, err := engine.proposals.GetBlockByHash(round, hash)
	if err == nil {
		return block, nil
	}
	engine.proposals.ApproveBlock(hash)
	engine.pm.RequestBlockByHash(hash)

	for start := time.Now(); time.Since(start) < engine.cfg.Consensus.WaitBlockDelay; {
		block := engine.proposals.GetBlock(hash)
		if block != nil {
			engine.log.Info("Block was received successfully", "hash", block.Hash().Hex())
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
			engine.timeDriftMutex.Lock()
			engine.timeDrift = drift
			engine.timeDriftMutex.Unlock()
		}
		time.Sleep(time.Minute)
	}
}

func (engine *Engine) Synced() bool {
	return engine.synced
}

func (engine *Engine) checkOnlineStatus() error {

	if !engine.cfg.AutoOnline {
		return nil
	}

	appState, err := engine.ReadonlyAppState()
	if err != nil {
		return err
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return nil
	}

	coinbase := engine.secStore.GetAddress()
	if appState.ValidatorsCache.IsOnlineIdentity(coinbase) {
		return nil
	}
	if !appState.ValidatorsCache.IsValidated(coinbase) && !appState.ValidatorsCache.IsPool(coinbase) {
		return nil
	}
	if appState.State.HasStatusSwitchAddresses(coinbase) {
		return nil
	}

	for _, existedTx := range engine.txpool.GetPendingByAddress(coinbase) {
		if existedTx.Type == types.OnlineStatusTx {
			return nil
		}
	}

	tx := blockchain.BuildTxWithFeeEstimating(appState, coinbase, nil, types.OnlineStatusTx, decimal.Zero, decimal.Zero, decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true))

	tx, err = engine.secStore.SignTx(tx)
	if err != nil {
		return err
	}

	return engine.txpool.AddInternalTx(tx)
}

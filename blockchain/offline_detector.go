package blockchain

import (
	"errors"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/secstore"
	dbm "github.com/tendermint/tm-db"
	"sync"
	"time"
)

const (
	OfflineVotesCacheLength = 10
)

type OfflineDetector struct {
	appState *appstate.AppState
	bus      eventbus.Bus
	repo     *database.Repo
	config   *config.OfflineDetectionConfig
	secStore *secstore.SecStore

	votesChan               chan *types.Vote
	offlineVoting           map[common.Hash]*voteList
	activityMap             map[common.Address]time.Time
	offlineProposals        map[common.Address]time.Time
	mutex                   sync.Mutex
	lastPersistBlock        uint64
	startTime               time.Time
	selfAddress             common.Address
	offlineCommitteeMaxSize int
}

type voteList struct {
	round  uint64
	voters mapset.Set
}

func NewOfflineDetector(config *config.Config, db dbm.DB, appState *appstate.AppState, secStore *secstore.SecStore, bus eventbus.Bus) *OfflineDetector {
	return &OfflineDetector{
		config:                  config.OfflineDetection,
		repo:                    database.NewRepo(db),
		votesChan:               make(chan *types.Vote, 10000),
		activityMap:             make(map[common.Address]time.Time),
		offlineProposals:        make(map[common.Address]time.Time),
		offlineVoting:           make(map[common.Hash]*voteList),
		appState:                appState,
		bus:                     bus,
		startTime:               time.Now().UTC(),
		secStore:                secStore,
		offlineCommitteeMaxSize: config.Consensus.MaxCommitteeSize * 3,
	}
}

func (dt *OfflineDetector) Start(head *types.Header) {
	dt.selfAddress = dt.secStore.GetAddress()
	dt.lastPersistBlock = head.Height()
	dt.restore()

	_ = dt.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			block := e.(*events.NewBlockEvent).Block
			if block.Header.Flags().HasFlag(types.ValidationFinished) {
				dt.restart()
			}
			go dt.processBlock(block)
		})

	go dt.startListening()
}

func (dt *OfflineDetector) ProcessVote(vote *types.Vote) {
	select {
	case dt.votesChan <- vote:
	default:
	}
}

func (dt *OfflineDetector) VoteForOffline(block *types.Block) bool {
	if block == nil {
		return false
	}
	if !block.Header.Flags().HasFlag(types.OfflinePropose) {
		return false
	}
	addr := block.Header.OfflineAddr()
	if addr == nil || *addr == dt.selfAddress {
		return false
	}

	if time.Now().UTC().Sub(dt.startTime) < dt.config.OfflineVoteInterval {
		return false
	}

	if dt.appState.State.ValidationPeriod() != state.NonePeriod {
		return false
	}

	if dt.appState.State.HasStatusSwitchAddresses(*addr) {
		return false
	}

	dt.mutex.Lock()
	defer dt.mutex.Unlock()

	if activityTime, ok := dt.activityMap[*addr]; ok {
		if time.Now().UTC().Sub(activityTime) > dt.config.OfflineVoteInterval {
			return true
		}
	} else {
		dt.activityMap[*addr] = time.Now().UTC()
	}

	return false
}

func (dt *OfflineDetector) ValidateBlock(head *types.Header, block *types.Block) error {

	offlineProposeSet := block.Header.Flags().HasFlag(types.OfflinePropose)
	offlineCommitSet := block.Header.Flags().HasFlag(types.OfflineCommit)

	if offlineProposeSet && offlineCommitSet {
		return errors.New("only one Offline flag can be set")
	}

	if offlineProposeSet || offlineCommitSet {
		addr := block.Header.OfflineAddr()
		if addr == nil {
			return errors.New("offline addr should be filled if Offline* flag is set")
		}
		if !dt.appState.ValidatorsCache.IsOnlineIdentity(*addr) {
			return errors.New("offline voting works only for online identities")
		}
		if dt.appState.State.ValidationPeriod() != state.NonePeriod {
			return errors.New("block cannot be accepted due to validation ceremony")
		}
		if dt.appState.State.HasStatusSwitchAddresses(*addr) {
			return errors.New("address already has pending status switch")
		}

		if offlineCommitSet {
			prevAddr := head.OfflineAddr()
			if !head.Flags().HasFlag(types.OfflinePropose) || *prevAddr != *addr {
				return errors.New("no offline proposal found")
			}
			if !block.Header.Flags().HasFlag(types.IdentityUpdate) {
				return errors.New("if OfflineCommit is set, IdentityUpdate should be set too")
			}

			if !dt.verifyOfflineProposing(block.Header.ParentHash()) {
				return errors.New(fmt.Sprintf("addr %v should not be set offline, block %v", addr.Hex(), block.Hash().Hex()))
			}
		}
	}

	return nil
}

func (dt *OfflineDetector) ProposeOffline(head *types.Header) (*common.Address, types.BlockFlag) {
	if dt.appState.State.ValidationPeriod() != state.NonePeriod {
		return nil, 0
	}
	if head.Flags().HasFlag(types.OfflinePropose) {
		if dt.verifyOfflineProposing(head.Hash()) {
			return head.OfflineAddr(), types.OfflineCommit
		}
		return nil, 0
	}

	if time.Now().UTC().Sub(dt.startTime) < dt.config.OfflineProposeInterval {
		return nil, 0
	}

	dt.mutex.Lock()
	defer dt.mutex.Unlock()

	minActivityTime := time.Now().UTC().Add(-dt.config.OfflineProposeInterval).Unix()
	onlineNodesSet := dt.appState.ValidatorsCache.GetAllOnlineValidators()

	for v := range onlineNodesSet.Iter() {
		if addr, ok := v.(common.Address); ok {
			if addr == dt.selfAddress {
				continue
			}
			shouldBecomeOffline := false
			if value, ok := dt.activityMap[addr]; ok {
				if value.Unix() < minActivityTime {
					shouldBecomeOffline = true
				}
			} else {
				dt.activityMap[addr] = time.Now().UTC()
			}

			if shouldBecomeOffline {
				if dt.appState.State.HasStatusSwitchAddresses(addr) {
					continue
				}
				if prevProposeTime, ok := dt.offlineProposals[addr]; ok {
					if time.Now().UTC().Sub(prevProposeTime) < dt.config.IntervalBetweenOfflineRetry {
						continue
					}
				}
				dt.offlineProposals[addr] = time.Now().UTC()
				return &addr, types.OfflinePropose
			}
		}
	}

	return nil, 0
}

func (dt *OfflineDetector) GetActivityMap() map[common.Address]time.Time {
	dt.mutex.Lock()
	defer dt.mutex.Unlock()
	res := make(map[common.Address]time.Time, len(dt.activityMap))
	for key, value := range dt.activityMap {
		res[key] = value
	}
	return res
}

func (dt *OfflineDetector) startListening() {
	for {
		select {
		case vote := <-dt.votesChan:
			dt.processVote(vote)
		}
	}
}

func (dt *OfflineDetector) processBlock(block *types.Block) {
	dt.mutex.Lock()
	defer dt.mutex.Unlock()

	if dt.lastPersistBlock+uint64(dt.config.PersistInterval) <= block.Height() {

		// clear old offline proposals
		for key, value := range dt.offlineProposals {
			if time.Now().UTC().Sub(value) > dt.config.IntervalBetweenOfflineRetry*5 {
				delete(dt.offlineProposals, key)
			}
		}
		// clear old votings
		for key, value := range dt.offlineVoting {
			if value.round < block.Height()-OfflineVotesCacheLength {
				delete(dt.offlineVoting, key)
			}
		}
		dt.lastPersistBlock = block.Height()
		dt.persist()
	}

	if block.Header.Coinbase() != (common.Address{}) {
		dt.activityMap[block.Header.Coinbase()] = time.Now().UTC()
	}

	for _, tx := range block.Body.Transactions {
		sender, _ := types.Sender(tx)

		switch tx.Type {
		case types.OnlineStatusTx:
			dt.activityMap[sender] = time.Now().UTC()
		}
	}
}

func (dt *OfflineDetector) processVote(vote *types.Vote) {
	dt.mutex.Lock()
	defer dt.mutex.Unlock()

	votesAddr := vote.VoterAddr()

	// process offline\online vote
	if vote.Header.TurnOffline {
		if dt.offlineVoting[vote.Header.VotedHash] == nil {
			dt.offlineVoting[vote.Header.VotedHash] = &voteList{
				round:  vote.Header.Round,
				voters: mapset.NewSet(),
			}
		}
		dt.offlineVoting[vote.Header.VotedHash].voters.Add(votesAddr)
	}

	// record activity
	dt.activityMap[votesAddr] = time.Now().UTC()
}

func (dt *OfflineDetector) persist() {
	data := make([]*types.AddrActivity, 0, len(dt.activityMap))
	for addr, activityTime := range dt.activityMap {
		data = append(data, &types.AddrActivity{
			Time: activityTime,
			Addr: addr,
		})
	}

	result := &types.ActivityMonitor{
		UpdateDt: time.Now().UTC(),
		Data:     data,
	}

	dt.repo.WriteActivity(result)
}

func (dt *OfflineDetector) restore() {
	activityMonitor := dt.repo.ReadActivity()

	if activityMonitor == nil {
		return
	}

	diff := time.Now().UTC().Sub(activityMonitor.UpdateDt)
	// skip saved data
	if diff > dt.config.MaxSelfOffline {
		return
	}

	for _, item := range activityMonitor.Data {
		dt.activityMap[item.Addr] = item.Time.Add(diff)
	}
}

func (dt *OfflineDetector) verifyOfflineProposing(hash common.Hash) bool {
	if dt.offlineVoting[hash] == nil {
		return false
	}

	onlineSize := dt.appState.ValidatorsCache.OnlineSize()

	threshold := onlineSize/2 + 1
	if threshold > dt.offlineCommitteeMaxSize {
		threshold = dt.offlineCommitteeMaxSize
	}

	return dt.offlineVoting[hash].voters.Cardinality() >= threshold
}

func (dt *OfflineDetector) restart() {
	dt.mutex.Lock()
	defer dt.mutex.Unlock()

	dt.startTime = time.Now().UTC()

	dt.offlineProposals = make(map[common.Address]time.Time)
	dt.activityMap = make(map[common.Address]time.Time)
	dt.offlineVoting = make(map[common.Hash]*voteList)
}

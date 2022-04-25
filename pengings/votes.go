package pengings

import (
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/upgrade"
	"github.com/idena-network/idena-go/events"
	"sync"
)

const (
	MaxKnownVotes              = 10000
	VotesLag                   = 3
	PropagateFutureVotesPeriod = 30
)

type Votes struct {
	votesByRound    *sync.Map
	knownVotes      mapset.Set
	state           *appstate.AppState
	head            *types.Header
	headMutex       sync.RWMutex
	bus             eventbus.Bus
	offlineDetector *blockchain.OfflineDetector
	upgrade         *upgrade.Upgrader
}

func NewVotes(state *appstate.AppState, bus eventbus.Bus, offlineDetector *blockchain.OfflineDetector, upgrade *upgrade.Upgrader) *Votes {
	v := &Votes{
		votesByRound:    &sync.Map{},
		knownVotes:      mapset.NewSet(),
		state:           state,
		bus:             bus,
		offlineDetector: offlineDetector,
		upgrade:         upgrade,
	}
	v.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			v.headMutex.Lock()
			v.head = newBlockEvent.Block.Header
			v.headMutex.Unlock()
		})
	return v
}

func (votes *Votes) Initialize(head *types.Header) {
	votes.head = head
}

func (votes *Votes) AddVote(vote *types.Vote) bool {
	votes.headMutex.RLock()
	head := votes.head
	votes.headMutex.RUnlock()

	minRound := head.Height() - VotesLag

	if head.Height() > VotesLag && vote.Header.Round < minRound {
		return false
	}

	if head.Height() < vote.Header.Round && vote.Header.Round-head.Height() > PropagateFutureVotesPeriod {
		return false
	}

	if votes.knownVotes.Contains(vote.Hash()) {
		return false
	}

	if votes.state.ValidatorsCache.OnlineSize() > 0 && !votes.state.ValidatorsCache.IsOnlineIdentity(vote.VoterAddr()) {
		return false
	}

	m, _ := votes.votesByRound.LoadOrStore(vote.Header.Round, &sync.Map{})
	byRound := m.(*sync.Map)

	byRound.Store(vote.Hash(), vote)
	if votes.knownVotes.Cardinality() > MaxKnownVotes {
		votes.knownVotes.Pop()
	}
	votes.knownVotes.Add(vote.Hash())
	votes.offlineDetector.ProcessVote(vote)
	votes.upgrade.ProcessVote(vote)
	return true
}

func (votes *Votes) GetVotesOfRound(round uint64) *sync.Map {
	if m, ok := votes.votesByRound.Load(round); ok {
		return m.(*sync.Map)
	}
	return nil
}

func (votes *Votes) CompleteRound(round uint64) {
	votes.votesByRound.Range(func(key, value interface{}) bool {
		if key.(uint64) <= round {
			votes.votesByRound.Delete(key)
		}
		return true
	})
}

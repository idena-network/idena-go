package pengings

import (
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/appstate"
	"sync"
)

const (
	MaxKnownVotes = 100000
)

type Votes struct {
	votesByRound *sync.Map
	votesByHash  *sync.Map
	knownVotes   mapset.Set
	state        *appstate.AppState
}

func NewVotes(state *appstate.AppState) *Votes {
	return &Votes{
		votesByRound: &sync.Map{},
		votesByHash:  &sync.Map{},
		knownVotes:   mapset.NewSet(),
		state:        state,
	}
}

func (votes *Votes) AddVote(vote *types.Vote) bool {

	if votes.knownVotes.Contains(vote.Hash()) {
		return false
	}

	if votes.state.ValidatorsCache.GetCountOfValidNodes() > 0 && !votes.state.ValidatorsCache.Contains(vote.VoterAddr()) {
		return false
	}

	m, _ := votes.votesByRound.LoadOrStore(vote.Header.Round, &sync.Map{})
	byRound := m.(*sync.Map)

	byRound.Store(vote.Hash(), vote)
	if votes.knownVotes.Cardinality() > MaxKnownVotes {
		votes.knownVotes.Pop()
	}
	votes.knownVotes.Add(vote.Hash())
	votes.votesByHash.Store(vote.Hash(), vote)
	return true
}

func (votes *Votes) GetVoteByHash(hash common.Hash) *types.Vote {
	if value, ok := votes.votesByHash.Load(hash); ok {
		return value.(*types.Vote)
	}
	return nil
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

	votes.votesByHash.Range(func(key, value interface{}) bool {
		vote := value.(*types.Vote)
		if vote.Header.Round <= round {
			votes.votesByHash.Delete(key)
		}
		return true
	})
}

func (votes *Votes) FutureBlockExist(round uint64, neccessaryVotes int) bool {
	maxRound := round

	votes.votesByRound.Range(func(key, value interface{}) bool {
		if key.(uint64) > round {
			byRound := value.(*sync.Map)

			votesCount := make(map[common.Hash]map[uint16]int)

			var byHash map[uint16]int

			byRound.Range(func(key, v interface{}) bool {
				vote := v.(*types.Vote)
				var ok bool
				byHash, ok = votesCount[vote.Header.VotedHash]
				if !ok {
					byHash = make(map[uint16]int)
					votesCount[vote.Header.VotedHash] = byHash
				}
				if _, ok := byHash[vote.Header.Step]; ok {
					byHash[vote.Header.Step]++
				} else {
					byHash[vote.Header.Step] = 1
				}
				return true
			})

			for _, v := range byHash {
				if v >= neccessaryVotes {
					maxRound = key.(uint64)
					return false
				}
			}

		}
		return true
	})
	return maxRound > round
}

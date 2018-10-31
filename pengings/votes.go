package pengings

import (
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/crypto"
	"sync"
)

const (
	MaxKnownVotes = 100000
)

type Votes struct {
	votesByRound *sync.Map
	knownVotes   mapset.Set
}

func NewVotes() *Votes {
	return &Votes{
		votesByRound: &sync.Map{},
		knownVotes:   mapset.NewSet(),
	}
}

func (votes *Votes) AddVote(vote *types.Vote) bool {

	if votes.knownVotes.Contains(vote.Hash()) {
		return false
	}

	if !validateVote(vote) {
		return false
	}
	m, _ := votes.votesByRound.LoadOrStore(vote.Header.Round, &sync.Map{})
	byRound := m.(*sync.Map)

	byRound.Store(vote.Hash(), vote)
	if votes.knownVotes.Cardinality() > MaxKnownVotes {
		votes.knownVotes.Pop()
	}
	votes.knownVotes.Add(vote.Hash())
	return true
}

func (votes *Votes) GetVotesOfRound(round uint64) *sync.Map {
	if m, ok := votes.votesByRound.Load(round); ok {
		return m.(*sync.Map)
	}
	return nil
}

func validateVote(vote *types.Vote) bool {
	hash := vote.Hash()
	_, err := crypto.Ecrecover(hash[:], vote.Signature)
	return err == nil
}

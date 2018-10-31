package pengings

import (
	"github.com/deckarep/golang-set"
	"idena-go/blockchain"
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

func (votes *Votes) AddVote(vote *blockchain.Vote) bool {

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
	votes.knownVotes.Add(vote)
	return true
}

func validateVote(vote *blockchain.Vote) bool {
	hash := vote.Hash()
	return crypto.VerifySignature(vote.CommitteePubKey, hash[:], vote.Signature)
}

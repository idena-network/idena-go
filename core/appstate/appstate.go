package appstate

import (
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/core/state"
	"idena-go/core/validators"
)

type AppState struct {
	ValidatorsCache *validators.ValidatorsCache
	State           *state.StateDB
	NonceCache      *state.NonceCache
	IdentityState   *state.IdentityStateDB
}

func NewAppState(db dbm.DB) *AppState {
	stateDb := state.NewLazy(db)
	identityStateDb := state.NewLazyIdentityState(db)
	return &AppState{
		State:         stateDb,
		IdentityState: identityStateDb,
	}
}

func (s *AppState) ForCheck(height uint64) *AppState {
	return &AppState{
		State:           s.State.ForCheck(height),
		IdentityState:   s.IdentityState.ForCheckIdentityState(height),
		ValidatorsCache: s.ValidatorsCache,
		NonceCache:      s.NonceCache,
	}
}

func (s *AppState) Initialize(height uint64) {
	s.State.Load(height)
	s.IdentityState.Load(height)
	s.ValidatorsCache = validators.NewValidatorsCache(s.State)
	s.ValidatorsCache.Load()
	s.NonceCache = state.NewNonceCache(s.State)
}

func (s *AppState) Precommit() {
	s.State.Precommit(true)
	s.IdentityState.Precommit(true)
}

func (s *AppState) Reset() {
	s.State.Reset()
	s.IdentityState.Reset()
}
func (s *AppState) Commit() {
	s.State.Commit(true)
	s.IdentityState.Commit(true)
}

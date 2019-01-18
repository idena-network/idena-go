package appstate

import (
	"idena-go/core/state"
	"idena-go/core/validators"
)

type AppState struct {
	ValidatorsCache *validators.ValidatorsCache
	State           *state.StateDB
	NonceCache      *state.NonceCache
}

func NewAppState(s *state.StateDB) *AppState {
	return &AppState{
		State: s,
	}
}

func (s *AppState) Initialize(height uint64) {
	s.State.Load(height)
	s.ValidatorsCache = validators.NewValidatorsCache(s.State)
	s.NonceCache = state.NewNonceCache(s.State)
}

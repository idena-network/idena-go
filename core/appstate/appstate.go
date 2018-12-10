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
		ValidatorsCache: validators.NewValidatorsCache(s),
		State:           s,
		NonceCache:      state.NewNonceCache(s),
	}
}

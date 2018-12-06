package appstate

import (
	"idena-go/core/state"
	"idena-go/core/validators"
)

type AppState struct {
	ValidatorsState *validators.ValidatorsSet
	State           *state.StateDB
	NonceCache      *state.NonceCache
}

func NewAppState(validatorsSet *validators.ValidatorsSet, s *state.StateDB) *AppState {
	return &AppState{
		ValidatorsState: validatorsSet,
		State:           s,
		NonceCache:      state.NewNonceCache(s),
	}
}

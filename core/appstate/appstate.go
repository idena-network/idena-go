package appstate

import (
	"idena-go/core/state"
	"idena-go/core/validators"
)

type AppState struct {
	ValidatorsState *validators.ValidatorsSet
	State           *state.StateDB
}

func NewAppState(validatorsSet *validators.ValidatorsSet, state *state.StateDB) *AppState {
	return &AppState{
		ValidatorsState: validatorsSet,
		State:           state,
	}
}

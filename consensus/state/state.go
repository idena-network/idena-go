package state

import "idena-go/core/validators"

type ConsensusState struct {
	Validators *validators.ValidatorsSet
}

func NewConsensusState(validatorsSet *validators.ValidatorsSet) *ConsensusState {
	return &ConsensusState{
		Validators: validatorsSet,
	}
}



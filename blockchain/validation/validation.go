package validation

import (
	"github.com/pkg/errors"
	"idena-go/blockchain/types"
	"idena-go/common"
	cstate "idena-go/consensus/state"
	"idena-go/core/state"
)

var (
	NodeApprovedAlready = errors.New("Node is in validator set")
	InvalidSignature    = errors.New("Invalid signature")
	InvalidNonce        = errors.New("Invalid Nonce")
)

func ValidateTx(consensusState *cstate.ConsensusState, state *state.StateDB, tx *types.Transaction) error {
	if !validateApprovingTx(consensusState, tx) {
		return NodeApprovedAlready
	}

	hash := tx.Hash()

	if tx.Sender() == (common.Address{}) {
		return InvalidSignature
	}

	if state.GetNonce(tx.Sender()) > tx.AccountNonce {
		return InvalidNonce
	}
	return nil
}

func validateApprovingTx(consensusState *cstate.ConsensusState, tx *types.Transaction) bool {
	if consensusState.Validators.Contains(tx.Sender()) {
		return false
	}
	return true
}

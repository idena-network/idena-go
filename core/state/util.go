package state

import "github.com/idena-network/idena-go/blockchain/types"

func validationTxBitMask(txType types.TxType) byte {
	switch txType {
	case types.SubmitAnswersHashTx:
		return 1 << 0
	case types.SubmitShortAnswersTx:
		return 1 << 1
	case types.EvidenceTx:
		return 1 << 2
	case types.SubmitLongAnswersTx:
		return 1 << 3
	default:
		return 0
	}
}
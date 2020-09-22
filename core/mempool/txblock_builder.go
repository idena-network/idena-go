package mempool

import (
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
)

type buildingContext struct {
	appState           *appstate.AppState
	sortedTxs          []*types.Transaction
	sortedPriorityTxs  []*types.Transaction
	sortedTxsPerSender map[common.Address][]*types.Transaction
	curNoncesPerSender map[common.Address]uint32
	blockTxs           []*types.Transaction
	blockGas           int
}

func newBuildingContext(
	appState *appstate.AppState,
	sortedTxs []*types.Transaction,
	sortedPriorityTxs []*types.Transaction,
	sortedTxsPerSender map[common.Address][]*types.Transaction,
	curNoncesPerSender map[common.Address]uint32,
) *buildingContext {

	ctx := &buildingContext{
		appState:           appState,
		sortedTxs:          sortedTxs,
		sortedPriorityTxs:  sortedPriorityTxs,
		sortedTxsPerSender: sortedTxsPerSender,
		curNoncesPerSender: curNoncesPerSender,
	}
	return ctx
}

func (ctx *buildingContext) addPriorityTxsToBlock() {
	for len(ctx.sortedPriorityTxs) > 0 {
		ctx.addNextPriorityTxToBlock()
	}
}

func (ctx *buildingContext) addNextPriorityTxToBlock() {
	priorityTx := ctx.sortedPriorityTxs[0]
	ctx.sortedPriorityTxs = ctx.sortedPriorityTxs[1:]

	sender, _ := types.Sender(priorityTx)
	senderSortedTxs := ctx.sortedTxsPerSender[sender]
	currentNonce := ctx.curNoncesPerSender[sender]
	isPriorityTxReached := false
	i := 0
	var txsToAdd []*types.Transaction
	gasToAdd := 0
	for !isPriorityTxReached {
		tx := senderSortedTxs[i]
		if currentNonce+1 != tx.AccountNonce {
			break
		}
		if !ctx.checkFee(tx) {
			break
		}
		gasToAdd += fee.CalculateGas(tx)
		if ctx.blockGas+gasToAdd > types.MaxBlockGas {
			break
		}
		txsToAdd = append(txsToAdd, tx)
		isPriorityTxReached = tx == priorityTx
		currentNonce = tx.AccountNonce
		i++
	}

	if !isPriorityTxReached {
		return
	}

	ctx.blockTxs = append(ctx.blockTxs, txsToAdd...)
	ctx.blockGas += gasToAdd
	ctx.curNoncesPerSender[sender] = currentNonce
	ctx.sortedTxsPerSender[sender] = ctx.sortedTxsPerSender[sender][i:]
}

func (ctx *buildingContext) addTxsToBlock() {
	txs := ctx.sortedTxs
	for _, tx := range txs {
		if !ctx.checkFee(tx) {
			continue
		}
		sender, _ := types.Sender(tx)
		if ctx.curNoncesPerSender[sender]+1 != tx.AccountNonce {
			continue
		}
		if ctx.blockGas+fee.CalculateGas(tx) > types.MaxBlockGas {
			return
		}
		ctx.blockTxs = append(ctx.blockTxs, tx)
		ctx.blockGas += fee.CalculateGas(tx)
		ctx.curNoncesPerSender[sender] = tx.AccountNonce
	}
}

func (ctx *buildingContext) checkFee(tx *types.Transaction) bool {
	return validation.ValidateFee(ctx.appState, tx, validation.InBlockTx) == nil
}

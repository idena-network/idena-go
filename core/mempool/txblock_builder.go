package mempool

import (
	"idena-go/blockchain/types"
	"idena-go/common"
)

type buildingContext struct {
	sortedTxs          []*types.Transaction
	sortedPriorityTxs  []*types.Transaction
	sortedTxsPerSender map[common.Address][]*types.Transaction
	curNoncesPerSender map[common.Address]uint32
	blockTxs           []*types.Transaction
	blockSize          int
}

func newBuildingContext(sortedTxs []*types.Transaction,
	sortedPriorityTxs []*types.Transaction,
	sortedTxsPerSender map[common.Address][]*types.Transaction,
	curNoncesPerSender map[common.Address]uint32) *buildingContext {

	ctx := &buildingContext{
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
	sizeToAdd := 0
	for !isPriorityTxReached {
		tx := senderSortedTxs[i]
		if currentNonce+1 != tx.AccountNonce {
			break
		}
		sizeToAdd += tx.Size()
		if ctx.blockSize+sizeToAdd > BlockBodySize {
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
	ctx.blockSize += sizeToAdd
	ctx.curNoncesPerSender[sender] = currentNonce
	ctx.sortedTxsPerSender[sender] = ctx.sortedTxsPerSender[sender][i:]
}

func (ctx *buildingContext) addTxsToBlock() {
	txs := ctx.sortedTxs
	for _, tx := range txs {
		if ctx.blockSize > BlockBodySize {
			return
		}
		sender, _ := types.Sender(tx)
		if ctx.curNoncesPerSender[sender]+1 != tx.AccountNonce {
			continue
		}
		ctx.blockTxs = append(ctx.blockTxs, tx)
		ctx.blockSize += tx.Size()
		ctx.curNoncesPerSender[sender] = tx.AccountNonce
	}
}

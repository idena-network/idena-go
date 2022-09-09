package deferredtx

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm"
	"github.com/idena-network/idena-go/vm/embedded"
	"github.com/shopspring/decimal"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"sync"
)

const (
	Folder = "/deferred-txs"
)

type Job struct {
	bus  eventbus.Bus
	head *types.Header

	txs       *DeferredTxs
	mutex     sync.Mutex
	datadir   string
	bc        *blockchain.Blockchain
	txpool    mempool.TransactionPool
	appState  *appstate.AppState
	ks        *keystore.KeyStore
	secStore  *secstore.SecStore
	vmCreator vm.VmCreator
}

func NewJob(bus eventbus.Bus, datadir string, appState *appstate.AppState, bc *blockchain.Blockchain, txpool mempool.TransactionPool,
	ks *keystore.KeyStore, secStore *secstore.SecStore, vmCreator vm.VmCreator) (*Job, error) {
	job := &Job{
		bus:       bus,
		datadir:   datadir,
		appState:  appState,
		bc:        bc,
		txpool:    txpool,
		ks:        ks,
		secStore:  secStore,
		txs:       new(DeferredTxs),
		vmCreator: vmCreator,
	}

	file, err := job.openFile()

	if err != nil {
		return nil, err
	}

	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if err := job.txs.FromBytes(data); err != nil {
		return nil, err
	}

	log.Info("Deferred txs loaded", "cnt", len(job.txs.Txs))

	bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			job.head = newBlockEvent.Block.Header
			job.broadcast()
		})
	return job, nil
}

func (j *Job) broadcast() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	if j.txpool.IsSyncing() {
		return
	}
	newTxs := new(DeferredTxs)
	for _, tx := range j.txs.Txs {
		if tx.BroadcastBlock <= j.head.Height() {
			log.Info("Sending deferred tx", logCtx(tx)...)
			err := j.sendTx(tx)
			if err != nil {
				log.Error("error while sending deferred tx", "err", err)
				tx.sendTry++
				contractErr, ok := err.(*embedded.ContractError)
				tryLater := ok && contractErr.TryLater() || err == validation.LateTx
				if tryLater {
					tx.BroadcastBlock = calculateBroadcastBlock(j.head.Height(), tx.sendTry)
				} else if tx.sendTry > 3 {
					tx.removed = true
				}
			} else {
				tx.removed = true
			}

		}
		if !tx.removed {
			newTxs.Txs = append(newTxs.Txs, tx)
		}
	}
	prev := j.txs
	j.txs = newTxs
	if len(prev.Txs) != len(j.txs.Txs) {
		if err := j.persist(); err != nil {
			log.Warn("cannot persist deferred txs", "err", err)
		}
	}
}

func logCtx(tx *DeferredTx) []interface{} {
	res := []interface{}{"from", tx.From.Hex(), "amount", blockchain.ConvertToFloat(tx.Amount), "tips", blockchain.ConvertToFloat(tx.Tips), "payloadSize", len(tx.Payload), "broadcastBlock", tx.BroadcastBlock}
	if tx.To != nil {
		res = append(res, "to")
		res = append(res, tx.To.Hex())
	}
	return res
}

func (j *Job) AddDeferredTx(from common.Address, to *common.Address, amount *big.Int, payload []byte, tips *big.Int, broadcastBlock uint64) error {
	tx := &DeferredTx{
		From:           from,
		To:             to,
		Amount:         amount,
		Payload:        payload,
		Tips:           tips,
		BroadcastBlock: broadcastBlock,
	}
	j.mutex.Lock()
	defer j.mutex.Unlock()

	log.Info("Adding new deferred tx", logCtx(tx)...)

	j.txs.Txs = append(j.txs.Txs, tx)

	err := j.persist()
	if err != nil {
		log.Warn("cannot persist deferred txs", "err", err)
	}
	return err
}

func (j *Job) persist() error {
	file, err := j.openFile()
	defer file.Close()
	if err != nil {
		return err
	}
	data := j.txs.ToBytes()
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	return nil
}

func (j *Job) openFile() (file *os.File, err error) {
	newpath := filepath.Join(j.datadir, Folder)
	if err := os.MkdirAll(newpath, os.ModePerm); err != nil {
		return nil, err
	}
	filePath := filepath.Join(newpath, "txs")
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (j *Job) sendTx(dtx *DeferredTx) error {

	tx, err := j.getSignedTx(dtx, decimal.Zero)
	if err != nil {
		return err
	}

	readonlyAppState, err := j.appState.Readonly(j.head.Height())
	if err != nil {
		return err
	}

	vm := j.vmCreator(readonlyAppState, j.head, nil, j.bc.Config())
	r := vm.Run(tx, nil, -1)
	if r.Error != nil {
		return r.Error
	}
	r.GasCost = j.bc.GetGasCost(j.appState, r.GasUsed)

	fee := fee.CalculateFee(readonlyAppState.ValidatorsCache.NetworkSize(), readonlyAppState.State.FeePerGas(), tx)
	fee.Add(fee, r.GasCost)
	maxFee := big.NewInt(0).Mul(fee, big.NewInt(2))

	tx, err = j.getSignedTx(dtx, blockchain.ConvertToFloat(maxFee))
	if err != nil {
		return err
	}
	return j.txpool.AddInternalTx(tx)
}

func (j *Job) getSignedTx(dtx *DeferredTx, maxFee decimal.Decimal) (*types.Transaction, error) {
	tx := blockchain.BuildTx(j.appState, dtx.From, dtx.To, types.CallContractTx, blockchain.ConvertToFloat(dtx.Amount), maxFee, blockchain.ConvertToFloat(dtx.Tips),
		0, 0, dtx.Payload)
	var signedTx *types.Transaction
	var err error
	if dtx.From == j.secStore.GetAddress() {
		signedTx, err = j.secStore.SignTx(tx)
		if err != nil {
			return nil, err
		}
	} else {
		account, err := j.ks.Find(keystore.Account{Address: dtx.From})
		if err != nil {
			return nil, err
		}
		signedTx, err = j.ks.SignTx(account, tx)
		if err != nil {
			return nil, err
		}
	}
	return signedTx, nil
}

type DeferredTx struct {
	From           common.Address
	To             *common.Address
	Amount         *big.Int
	Payload        []byte
	Tips           *big.Int
	BroadcastBlock uint64
	sendTry        int
	removed        bool
}

func (d *DeferredTx) ToProto() *models.ProtoDeferredTxs_ProtoDeferredTx {
	protoObj := &models.ProtoDeferredTxs_ProtoDeferredTx{}
	protoObj.From = d.From.Bytes()
	if d.To != nil {
		protoObj.To = d.To.Bytes()
	}
	if d.Amount != nil {
		protoObj.Amount = d.Amount.Bytes()
	}
	protoObj.Payload = d.Payload
	if d.Tips != nil {
		protoObj.Tips = d.Tips.Bytes()
	}
	protoObj.Block = d.BroadcastBlock

	return protoObj
}

func (d *DeferredTx) FromProto(protoObj *models.ProtoDeferredTxs_ProtoDeferredTx) {
	d.From.SetBytes(protoObj.From)
	if protoObj.To != nil {
		d.To = &common.Address{}
		d.To.SetBytes(protoObj.To)
	}
	d.Payload = protoObj.Payload
	if protoObj.Amount != nil {
		d.Amount = new(big.Int)
		d.Amount.SetBytes(protoObj.Amount)
	}
	if protoObj.Tips != nil {
		d.Tips = new(big.Int)
		d.Tips.SetBytes(protoObj.Tips)
	}
	d.BroadcastBlock = protoObj.Block
}

type DeferredTxs struct {
	Txs []*DeferredTx
}

func (d *DeferredTxs) ToBytes() []byte {
	protoOBj := &models.ProtoDeferredTxs{}
	for _, tx := range d.Txs {
		protoOBj.Txs = append(protoOBj.Txs, tx.ToProto())
	}
	data, _ := proto.Marshal(protoOBj)
	return data
}

func (d *DeferredTxs) FromBytes(data []byte) error {
	protoOBj := &models.ProtoDeferredTxs{}
	if err := proto.Unmarshal(data, protoOBj); err != nil {
		return err
	}
	for _, tx := range protoOBj.Txs {
		deferredTx := &DeferredTx{}
		deferredTx.FromProto(tx)
		d.Txs = append(d.Txs, deferredTx)
	}
	return nil
}

func calculateBroadcastBlock(prevBlock uint64, try int) uint64 {
	add := uint64(1)
	switch try {
	case 1:
		add = 1
	case 2:
		add = 2
	case 3:
		add = 4
	default:
		add = 8
	}
	return prevBlock + add
}

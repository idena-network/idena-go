package mempool

import (
	"encoding/json"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

const (
	Folder = "own-mempool-txs"
)

type txKeeper struct {
	txs     map[common.Hash]hexutil.Bytes
	datadir string
	mutex   sync.Mutex
}

func NewTxKeeper(datadir string) *txKeeper {
	return &txKeeper{datadir: datadir, txs: make(map[common.Hash]hexutil.Bytes)}
}

func (k *txKeeper) persist() error {
	file, err := k.openFile()
	defer file.Close()
	if err != nil {
		return err
	}
	var list []hexutil.Bytes
	for _, d := range k.txs {
		list = append(list, d)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	return nil
}
func (k *txKeeper) Load() {
	file, err := k.openFile()
	defer file.Close()
	if err == nil {
		data, err := ioutil.ReadAll(file)
		if err == nil {
			list := []hexutil.Bytes{}
			if err := json.Unmarshal(data, &list); err != nil {
				log.Warn("cannot parse txs.json", "err", err)
			}
			k.mutex.Lock()
			for _, hex := range list {
				k.txs[crypto.Hash(hex)] = hex
			}
			k.mutex.Unlock()
		}
	}
}

func (k *txKeeper) openFile() (file *os.File, err error) {
	newpath := filepath.Join(k.datadir, Folder)
	if err := os.MkdirAll(newpath, os.ModePerm); err != nil {
		return nil, err
	}
	filePath := filepath.Join(newpath, "txs.json")
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (k *txKeeper) AddTx(tx *types.Transaction) {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	if _, ok := k.txs[tx.Hash()]; ok {
		return
	}
	data, _ := tx.ToBytes()
	k.txs[tx.Hash()] = data
	if err := k.persist(); err != nil {
		log.Warn("error while save mempool tx", "err", err)
	}
}

func (k *txKeeper) RemoveTx(hash common.Hash) {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	if _, ok := k.txs[hash]; ok {
		delete(k.txs, hash)
		if err := k.persist(); err != nil {
			log.Warn("error while remove mempool tx", "err", err)
		}
	}
}

func (k *txKeeper) List() []*types.Transaction {
	var result []*types.Transaction
	for _, hex := range k.txs {
		tx := new(types.Transaction)
		tx.FromBytes(hex)
		result = append(result, tx)
	}
	return result
}

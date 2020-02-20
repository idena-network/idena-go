package database

import (
	"bytes"
	"encoding/binary"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"sort"
	"time"
)

const (
	MaxWeakCertificatesCount = 100
)

type Repo struct {
	db dbm.DB
}

func NewRepo(db dbm.DB) *Repo {
	return &Repo{
		db: db,
	}
}

func encodeUint16Number(number uint16) []byte {
	enc := make([]byte, 2)
	binary.BigEndian.PutUint16(enc, number)
	return enc
}

func encodeUint32Number(number uint32) []byte {
	enc := make([]byte, 4)
	binary.BigEndian.PutUint32(enc, number)
	return enc
}

func encodeUint64Number(number uint64) []byte {
	enc := make([]byte, 8)
	binary.BigEndian.PutUint64(enc, number)
	return enc
}

// headerKey = headerPrefix + hash
func headerKey(hash common.Hash) []byte {
	return append(headerPrefix, hash.Bytes()...)
}

func certKey(hash common.Hash) []byte {
	return append(certPrefix, hash.Bytes()...)
}

func headerHashKey(number uint64) []byte {
	return append(append(headerPrefix, encodeUint64Number(number)...), headerHashSuffix...)
}

func finalConsensusKey(hash common.Hash) []byte {
	return append(finalConsensusPrefix, hash.Bytes()...)
}

func txIndexKey(hash common.Hash) []byte {
	return append(transactionIndexPrefix, hash.Bytes()...)
}

func savedTxKey(sender common.Address, timestamp uint64, nonce uint32, hash common.Hash) []byte {
	key := append(ownTransactionIndexPrefix, sender[:]...)
	key = append(key, encodeUint64Number(timestamp)...)
	key = append(key, encodeUint32Number(nonce)...)
	return append(key, hash[:]...)
}

func burntCoinsKey(height uint64, hash common.Hash) []byte {
	key := append(burntCoinsPrefix, encodeUint64Number(height)...)
	return append(key, hash[:]...)
}

func burntCoinsMinKey() []byte {
	return burntCoinsKey(0, common.BytesToHash(common.MinHash[:]))
}

func identityStateDiffKey(height uint64) []byte {
	return append(identityStateDiffPrefix, encodeUint64Number(height)...)
}

func (r *Repo) ReadBlockHeader(hash common.Hash) *types.Header {
	data, err := r.db.Get(headerKey(hash))
	assertNoError(err)
	if data == nil {
		return nil
	}
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "hash", hash, "err", err)
		return nil
	}
	return header
}

func (r *Repo) ReadHead() *types.Header {
	data, err := r.db.Get(headBlockKey)
	assertNoError(err)
	if data == nil {
		return nil
	}
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "err", err)
		return nil
	}
	return header
}

func (r *Repo) WriteHead(batch dbm.Batch, header *types.Header) {
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
		return
	}
	if batch != nil {
		batch.Set(headBlockKey, data)
	} else {
		r.db.Set(headBlockKey, data)
	}
}

func (r *Repo) WriteBlockHeader(header *types.Header) {
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}

	r.db.Set(headerKey(header.Hash()), data)
}

func (r *Repo) RemoveHeader(hash common.Hash) {
	r.db.Delete(headerKey(hash))
}

func (r *Repo) WriteCertificate(hash common.Hash, cert *types.BlockCert) {
	data, err := rlp.EncodeToBytes(cert)
	if err != nil {
		log.Crit("failed to RLP encode block cert", "err", err)
	}
	r.db.Set(certKey(hash), data)
}

func (r *Repo) WriteCanonicalHash(height uint64, hash common.Hash) {
	key := headerHashKey(height)
	r.db.Set(key, hash.Bytes())
}

func (r *Repo) RemoveCanonicalHash(height uint64) {
	key := headerHashKey(height)
	r.db.Delete(key)
}

func (r *Repo) ReadCanonicalHash(height uint64) common.Hash {
	key := headerHashKey(height)
	data, err := r.db.Get(key)
	assertNoError(err)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

func (r *Repo) WriteFinalConsensus(hash common.Hash) {
	key := finalConsensusKey(hash)
	r.db.Set(key, []byte{0x1})
}

func (r *Repo) SetHead(batch dbm.Batch, height uint64) {
	hash := r.ReadCanonicalHash(height)
	if hash != (common.Hash{}) {
		header := r.ReadBlockHeader(hash)
		if header != nil {
			r.WriteHead(batch, header)
		}
	}
}

func (r *Repo) WriteTxIndex(txHash common.Hash, index *types.TransactionIndex) {
	data, err := rlp.EncodeToBytes(index)
	if err != nil {
		log.Crit("failed to RLP encode transaction index", "err", err)
		return
	}
	r.db.Set(txIndexKey(txHash), data)
}

func (r *Repo) ReadTxIndex(hash common.Hash) *types.TransactionIndex {
	key := txIndexKey(hash)
	data, err := r.db.Get(key)
	assertNoError(err)
	if data == nil {
		return nil
	}
	index := new(types.TransactionIndex)
	if err := rlp.DecodeBytes(data, index); err != nil {
		log.Error("invalid transaction index RLP", "err", err)
		return nil
	}
	return index
}

func (r *Repo) ReadCertificate(hash common.Hash) *types.BlockCert {
	data, err := r.db.Get(certKey(hash))
	assertNoError(err)
	if data == nil {
		return nil
	}
	cert := new(types.BlockCert)
	if err := rlp.DecodeBytes(data, cert); err != nil {
		log.Error("Invalid block cert RLP", "err", err)
		return nil
	}
	return cert
}

type weakCeritificates struct {
	Hashes []common.Hash
}

func (r *Repo) readWeakCertificates() *weakCeritificates {
	data, err := r.db.Get(weakCertificatesKey)
	assertNoError(err)
	if data == nil {
		return nil
	}
	w := new(weakCeritificates)
	if err := rlp.Decode(bytes.NewReader(data), w); err != nil {
		log.Error("invalid weak certificates RLP", "err", err)
		return nil
	}
	return w
}

func (r *Repo) writeWeakCertificate(w *weakCeritificates) {
	data, err := rlp.EncodeToBytes(w)
	if err != nil {
		log.Crit("failed to RLP encode weak certificates", "err", err)
		return
	}
	r.db.Set(weakCertificatesKey, data)
}

func (r *Repo) removeCertificate(hash common.Hash) {
	r.db.Delete(certKey(hash))
}

func (r *Repo) WriteWeakCertificate(hash common.Hash) {
	weakCerts := r.readWeakCertificates()
	if weakCerts == nil {
		weakCerts = &weakCeritificates{}
	}
	weakCerts.Hashes = append(weakCerts.Hashes, hash)

	if len(weakCerts.Hashes) > MaxWeakCertificatesCount {
		r.removeCertificate(weakCerts.Hashes[0])
		weakCerts.Hashes = weakCerts.Hashes[1:]
	}
	r.writeWeakCertificate(weakCerts)
}

type dbSnapshotManifest struct {
	Cid      []byte
	Height   uint64
	FileName string
	Root     common.Hash
}

func (r *Repo) LastSnapshotManifest() (cid []byte, root common.Hash, height uint64, fileName string) {
	data, err := r.db.Get(lastSnapshotKey)
	assertNoError(err)
	if data == nil {
		return nil, common.Hash{}, 0, ""
	}
	manifest := new(dbSnapshotManifest)
	if err := rlp.Decode(bytes.NewReader(data), manifest); err != nil {
		log.Error("invalid snapshot manifest RLP", "err", err)
		return nil, common.Hash{}, 0, ""
	}
	return manifest.Cid, manifest.Root, manifest.Height, manifest.FileName
}

func (r *Repo) WriteLastSnapshotManifest(cid []byte, root common.Hash, height uint64, fileName string) error {
	manifest := dbSnapshotManifest{
		Cid:      cid,
		Height:   height,
		FileName: fileName,
		Root:     root,
	}
	data, err := rlp.EncodeToBytes(manifest)
	if err != nil {
		log.Crit("failed to RLP encode snapshot manifest", "err", err)
		return err
	}
	r.db.Set(lastSnapshotKey, data)
	return nil
}

func (r *Repo) WriteIdentityStateDiff(height uint64, diff []byte) {
	r.db.Set(identityStateDiffKey(height), diff)
}

func (r *Repo) ReadIdentityStateDiff(height uint64) []byte {
	data, err := r.db.Get(identityStateDiffKey(height))
	assertNoError(err)
	return data
}

func (r *Repo) WritePreliminaryHead(header *types.Header) {
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
		return
	}
	assertNoError(r.db.Set(preliminaryHeadKey, data))
}

func (r *Repo) ReadPreliminaryHead() *types.Header {
	data, err := r.db.Get(preliminaryHeadKey)
	assertNoError(err)
	if data == nil {
		return nil
	}
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "err", err)
		return nil
	}
	return header
}

func (r *Repo) RemovePreliminaryHead(batch dbm.Batch) {
	if batch != nil {
		batch.Delete(preliminaryHeadKey)
	} else {
		assertNoError(r.db.Delete(preliminaryHeadKey))
	}
}

type activityMonitorDb struct {
	UpdateDt uint64
	Data     []*addrActivityDb
}

type addrActivityDb struct {
	Addr common.Address
	Time uint64
}

func (r *Repo) ReadActivity() *types.ActivityMonitor {
	data, err := r.db.Get(activityMonitorKey)
	assertNoError(err)
	if data == nil {
		return nil
	}
	dbMonitor := new(activityMonitorDb)
	if err := rlp.Decode(bytes.NewReader(data), dbMonitor); err != nil {
		log.Error("invalid activity monitor RLP", "err", err)
		return nil
	}
	monitor := &types.ActivityMonitor{
		UpdateDt: time.Unix(int64(dbMonitor.UpdateDt), 0),
	}
	for _, item := range dbMonitor.Data {
		monitor.Data = append(monitor.Data, &types.AddrActivity{
			Addr: item.Addr,
			Time: time.Unix(int64(item.Time), 0),
		})
	}

	return monitor
}

func (r *Repo) WriteActivity(monitor *types.ActivityMonitor) {
	dbMonitor := &activityMonitorDb{
		UpdateDt: uint64(monitor.UpdateDt.Unix()),
	}
	for _, item := range monitor.Data {
		dbMonitor.Data = append(dbMonitor.Data, &addrActivityDb{
			Addr: item.Addr,
			Time: uint64(item.Time.Unix()),
		})
	}
	data, err := rlp.EncodeToBytes(dbMonitor)
	if err != nil {
		log.Crit("failed to RLP encode activity monitor", "err", err)
		return
	}
	r.db.Set(activityMonitorKey, data)
}

func (r *Repo) SaveTx(address common.Address, blockHash common.Hash, timestamp uint64, feePerByte *big.Int, transaction *types.Transaction) {
	s := &types.SavedTransaction{
		Tx:         transaction,
		FeePerByte: feePerByte,
		BlockHash:  blockHash,
		Timestamp:  timestamp,
	}
	data, err := rlp.EncodeToBytes(s)
	if err != nil {
		log.Crit("failed to RLP encode saved transaction", "err", err)
		return
	}

	r.db.Set(savedTxKey(address, timestamp, transaction.AccountNonce, transaction.Hash()), data)
}

func (r *Repo) GetSavedTxs(address common.Address, count int, token []byte) (txs []*types.SavedTransaction, nextToken []byte) {

	if token == nil {
		token = savedTxKey(address, uint64(math.MaxUint64), uint32(math.MaxUint32), common.BytesToHash(common.MaxHash))
	} else {
		token = append(token, common.MaxHash...)
	}

	it, err := r.db.ReverseIterator(savedTxKey(address, 0, 0, common.BytesToHash(common.MinHash[:])), token)
	assertNoError(err)
	defer it.Close()
	for ; it.Valid(); it.Next() {
		key, value := it.Key(), it.Value()
		if len(txs) == count {
			return txs, key[:len(key)-common.HashLength]
		}
		tx := new(types.SavedTransaction)
		if err := rlp.DecodeBytes(value, tx); err != nil {
			log.Error("cannot parse tx", "key", key)
			continue
		}
		txs = append(txs, tx)
	}

	return txs, nil
}

func (r *Repo) DeleteOutdatedBurntCoins(blockHeight uint64, blockRange uint64) {
	if blockHeight <= blockRange {
		return
	}
	it, err := r.db.Iterator(burntCoinsMinKey(), burntCoinsKey(blockHeight-blockRange,
		common.BytesToHash(common.MaxHash[:])))
	assertNoError(err)
	defer it.Close()
	for ; it.Valid(); it.Next() {
		r.db.Delete(it.Key())
	}
}

func (r *Repo) SaveBurntCoins(blockHeight uint64, txHash common.Hash, address common.Address, key string, amount *big.Int) {
	s := &types.BurntCoins{
		Address: address,
		Key:     key,
		Amount:  amount,
	}
	data, err := rlp.EncodeToBytes(s)
	if err != nil {
		log.Crit("failed to RLP encode saved burnt coins", "err", err)
		return
	}
	r.db.Set(burntCoinsKey(blockHeight, txHash), data)
}

func (r *Repo) GetTotalBurntCoins() []*types.BurntCoins {
	it, err := r.db.Iterator(burntCoinsMinKey(), burntCoinsKey(math.MaxUint64, common.BytesToHash(common.MaxHash[:])))
	assertNoError(err)
	defer it.Close()

	var coinsByAddressAndKey map[common.Address]map[string]*types.BurntCoins
	var res []*types.BurntCoins
	for ; it.Valid(); it.Next() {
		key, value := it.Key(), it.Value()
		burntCoins := new(types.BurntCoins)
		if err := rlp.DecodeBytes(value, burntCoins); err != nil {
			log.Error("cannot parse burnt coins", "key", key)
			continue
		}
		if coinsByAddressAndKey == nil {
			coinsByAddressAndKey = make(map[common.Address]map[string]*types.BurntCoins)
		}
		if addressCoinsByKey, ok := coinsByAddressAndKey[burntCoins.Address]; ok {
			if total, ok := addressCoinsByKey[burntCoins.Key]; ok {
				total.Amount = new(big.Int).Add(total.Amount, burntCoins.Amount)
			} else {
				addressCoinsByKey[burntCoins.Key] = burntCoins
				res = append(res, burntCoins)
			}
		} else {
			addressCoinsByKey = make(map[string]*types.BurntCoins)
			addressCoinsByKey[burntCoins.Key] = burntCoins
			coinsByAddressAndKey[burntCoins.Address] = addressCoinsByKey
			res = append(res, burntCoins)
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Amount.Cmp(res[j].Amount) == 1
	})

	return res
}

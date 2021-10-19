package database

import (
	"encoding/binary"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	dbm "github.com/tendermint/tm-db"
	math2 "math"
	"math/big"
	"sort"
)

const (
	MaxWeakCertificatesCount = 100
	maxEventName             = "\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F\u007F"
)

type Repo struct {
	db dbm.DB
}

func NewRepo(db dbm.DB) *Repo {
	return &Repo{
		db: db,
	}
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

func receiptIndexKey(hash common.Hash) []byte {
	return append(receiptIndexPrefix, hash.Bytes()...)
}

func savedTxKey(sender common.Address, timestamp int64, nonce uint32, hash common.Hash) []byte {
	key := append(ownTransactionIndexPrefix, sender[:]...)
	key = append(key, encodeUint64Number(uint64(timestamp))...)
	key = append(key, encodeUint32Number(nonce)...)
	return append(key, hash[:]...)
}

func savedEventKey(contact common.Address, txHash []byte, idx uint32, event string) []byte {
	key := append(eventPrefix, contact.Bytes()...)
	key = append(key, txHash...)
	key = append(key, common.ToBytes(idx)...)
	key = append(key, []byte(event)...)
	return key
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
	if err := header.FromBytes(data); err != nil {
		log.Error("Invalid block header proto", "hash", hash, "err", err)
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
	if err := header.FromBytes(data); err != nil {
		log.Error("Invalid block header proto", "err", err)
		return nil
	}
	return header
}

func (r *Repo) WriteHead(batch dbm.Batch, header *types.Header) {
	// Write the encoded header
	data, err := header.ToBytes()
	if err != nil {
		log.Crit("Failed to proto encode header", "err", err)
		return
	}
	if batch != nil {
		batch.Set(headBlockKey, data)
	} else {
		r.db.Set(headBlockKey, data)
	}
}

func (r *Repo) WriteBlockHeader(header *types.Header) {
	data, err := header.ToBytes()
	if err != nil {
		log.Crit("Failed to proto encode header", "err", err)
	}

	r.db.Set(headerKey(header.Hash()), data)
}

func (r *Repo) RemoveHeader(hash common.Hash) {
	r.db.Delete(headerKey(hash))
}

func (r *Repo) WriteCertificate(hash common.Hash, cert *types.BlockCert) {
	data, err := cert.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode block cert", "err", err)
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
	data, err := index.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode transaction index", "err", err)
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
	if err := index.FromBytes(data); err != nil {
		log.Error("invalid transaction index proto", "err", err)
		return nil
	}
	return index
}

func (r *Repo) WriteReceiptIndex(hash common.Hash, idx *types.TxReceiptIndex) {
	data, err := idx.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode receipt index", "err", err)
		return
	}
	r.db.Set(receiptIndexKey(hash), data)
}

func (r *Repo) ReadReceiptIndex(hash common.Hash) *types.TxReceiptIndex {
	key := receiptIndexKey(hash)
	data, err := r.db.Get(key)
	assertNoError(err)
	if data == nil {
		return nil
	}
	index := new(types.TxReceiptIndex)
	if err := index.FromBytes(data); err != nil {
		log.Error("invalid receipt index proto", "err", err)
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
	if err := cert.FromBytes(data); err != nil {
		log.Error("Invalid block cert proto", "err", err)
		return nil
	}
	return cert
}

func (r *Repo) readWeakCertificates() *models.ProtoWeakCertificates {
	data, err := r.db.Get(weakCertificatesKey)
	assertNoError(err)
	if data == nil {
		return nil
	}
	w := new(models.ProtoWeakCertificates)
	if err := proto.Unmarshal(data, w); err != nil {
		log.Error("invalid weak certificates proto", "err", err)
		return nil
	}
	return w
}

func (r *Repo) writeWeakCertificate(w *models.ProtoWeakCertificates) {
	data, err := proto.Marshal(w)
	if err != nil {
		log.Crit("failed to proto encode weak certificates", "err", err)
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
		weakCerts = &models.ProtoWeakCertificates{}
	}
	weakCerts.Hashes = append(weakCerts.Hashes, hash[:])

	if len(weakCerts.Hashes) > MaxWeakCertificatesCount {
		r.removeCertificate(common.BytesToHash(weakCerts.Hashes[0]))
		weakCerts.Hashes = weakCerts.Hashes[1:]
	}
	r.writeWeakCertificate(weakCerts)
}

func (r *Repo) LastSnapshotManifest() (cidV2 []byte, root common.Hash, height uint64, fileName string) {
	data, err := r.db.Get(lastSnapshotKey)
	assertNoError(err)
	if data == nil {
		return nil, common.Hash{}, 0, ""
	}
	manifest := new(models.ProtoSnapshotManifestDb)
	if err := proto.Unmarshal(data, manifest); err != nil {
		log.Error("invalid snapshot manifest proto", "err", err)
		return nil, common.Hash{}, 0, ""
	}
	return manifest.CidV2, common.BytesToHash(manifest.Root), manifest.Height, manifest.FileNameV2
}

func (r *Repo) WriteLastSnapshotManifest(cidV2 []byte, root common.Hash, height uint64, fileNameV2 string) error {
	manifest := &models.ProtoSnapshotManifestDb{
		CidV2:      cidV2,
		Height:     height,
		FileNameV2: fileNameV2,
		Root:       root[:],
	}
	data, err := proto.Marshal(manifest)
	if err != nil {
		log.Crit("failed to proto encode snapshot manifest", "err", err)
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
	data, err := header.ToBytes()
	if err != nil {
		log.Crit("Failed to proto encode header", "err", err)
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
	if err := header.FromBytes(data); err != nil {
		log.Error("Invalid block header proto", "err", err)
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

func (r *Repo) ReadActivity() *types.ActivityMonitor {
	data, err := r.db.Get(activityMonitorKey)
	assertNoError(err)
	if data == nil {
		return nil
	}
	monitor := new(types.ActivityMonitor)
	if err := monitor.FromBytes(data); err != nil {
		log.Error("invalid activity monitor proto", "err", err)
		return nil
	}
	return monitor
}

func (r *Repo) WriteActivity(monitor *types.ActivityMonitor) {
	data, err := monitor.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode activity monitor", "err", err)
		return
	}
	r.db.Set(activityMonitorKey, data)
}

func (r *Repo) SaveTx(address common.Address, blockHash common.Hash, timestamp int64, feePerGas *big.Int, transaction *types.Transaction) {
	s := &types.SavedTransaction{
		Tx:        transaction,
		FeePerGas: feePerGas,
		BlockHash: blockHash,
		Timestamp: timestamp,
	}
	data, err := s.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode saved transaction", "err", err)
		return
	}

	r.db.Set(savedTxKey(address, timestamp, transaction.AccountNonce, transaction.Hash()), data)
}

func (r *Repo) GetSavedTxs(address common.Address, count int, token []byte) (txs []*types.SavedTransaction, nextToken []byte) {

	if token == nil {
		token = savedTxKey(address, math.MaxInt64, uint32(math.MaxUint32), common.BytesToHash(common.MaxHash))
	} else {
		token = append(token, common.MaxHash...)
	}

	it, err := r.db.ReverseIterator(savedTxKey(address, 0, 0, common.BytesToHash(common.MinHash[:])), token)
	assertNoError(err)
	defer it.Close()
	for ; it.Valid(); it.Next() {
		key, value := it.Key(), it.Value()
		if len(txs) == count {
			continuationToken := make([]byte, len(key)-common.HashLength)
			copy(continuationToken, key[:len(key)-common.HashLength])
			return txs, continuationToken
		}
		tx := new(types.SavedTransaction)
		if err := tx.FromBytes(value); err != nil {
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
	data, err := s.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode saved burnt coins", "err", err)
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
		if err := burntCoins.FromBytes(value); err != nil {
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

func (r *Repo) WriteEvent(contract common.Address, txHash common.Hash, idx uint32, event *types.TxEvent) {
	e := types.SavedEvent{
		Contract: contract,
		Event:    event.EventName,
		Args:     event.Data,
	}
	data, err := e.ToBytes()
	if err != nil {
		log.Crit("failed to proto encode saved event", "err", err)
		return
	}
	r.db.Set(savedEventKey(contract, txHash.Bytes(), idx, event.EventName), data)
}

func (r *Repo) GetSavedEvents(contract common.Address) (events []*types.SavedEvent) {

	it, err := r.db.ReverseIterator(savedEventKey(contract, common.MinHash[:], 0, "\x00"), savedEventKey(contract, common.MaxHash, math2.MaxUint32, maxEventName))
	assertNoError(err)
	defer it.Close()
	for ; it.Valid(); it.Next() {
		key, value := it.Key(), it.Value()
		e := new(types.SavedEvent)
		if err := e.FromBytes(value); err != nil {
			log.Error("cannot parse tx", "key", key)
			continue
		}
		events = append(events, e)
	}
	return events
}

func (r *Repo) WriteIntermediateGenesis(batch dbm.Batch, height uint64) {
	if batch != nil {
		batch.Set(intermediateGenesisKey, common.ToBytes(height))
	} else {
		r.db.Set(intermediateGenesisKey, common.ToBytes(height))
	}
}

func (r *Repo) ReadIntermediateGenesis() uint64 {
	data, err := r.db.Get(intermediateGenesisKey)
	if err != nil || len(data) == 0 {
		return 0
	}
	return binary.LittleEndian.Uint64(data)
}

func (r *Repo) WriteUpgradeVotes(votes *types.UpgradeVotes) {
	data, _ := votes.ToBytes()
	r.db.Set(upgradeVotesKey, data)
}

func (r *Repo) ReadUpgradeVotes() *types.UpgradeVotes {
	data, _ := r.db.Get(upgradeVotesKey)
	if len(data) > 0 {
		v := types.NewUpgradeVotes()
		v.FromBytes(data)
		return v
	}
	return nil
}

func (r *Repo) WriteConsensusVersion(batch dbm.Batch, v uint32) {
	if batch != nil {
		batch.Set(consensusVersionKey, common.ToBytes(v))
	} else {
		r.db.Set(consensusVersionKey, common.ToBytes(v))
	}
}

func (r *Repo) ReadConsensusVersion() uint32 {
	data, err := r.db.Get(consensusVersionKey)
	if err != nil || len(data) == 0 {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

func (r *Repo) WritePreliminaryConsensusVersion(v uint32) {
	r.db.Set(preliminaryConsVersionKey, common.ToBytes(v))
}

func (r *Repo) ReadPreliminaryConsensusVersion() uint32 {
	data, err := r.db.Get(preliminaryConsVersionKey)
	if err != nil || len(data) == 0 {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

func (r *Repo) RemovePreliminaryConsensusVersion(batch dbm.Batch) {
	if batch != nil {
		batch.Delete(preliminaryConsVersionKey)
	} else {
		r.db.Delete(preliminaryConsVersionKey)
	}
}

func (r *Repo) WritePreliminaryIntermediateGenesis(height uint64) {
	r.db.Set(preliminaryIntermediateGenesisKey, common.ToBytes(height))
}

func (r *Repo) ReadPreliminaryIntermediateGenesis() uint64 {
	data, err := r.db.Get(preliminaryIntermediateGenesisKey)
	if err != nil || len(data) == 0 {
		return 0
	}
	return binary.LittleEndian.Uint64(data)
}

func (r *Repo) RemovePreliminaryIntermediateGenesis(batch dbm.Batch) {
	if batch != nil {
		batch.Delete(preliminaryIntermediateGenesisKey)
	} else {
		r.db.Delete(preliminaryIntermediateGenesisKey)
	}
}

package database

import (
	"bytes"
	"encoding/binary"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	dbm "github.com/tendermint/tendermint/libs/db"
)

const (
	MaxWeakCertificatesCount = 300
)

type Repo struct {
	db dbm.DB
}

func NewRepo(db dbm.DB) *Repo {
	return &Repo{
		db: db,
	}
}

// encodeBlockNumber encodes a block number as big endian uint64
func encodeBlockNumber(number uint64) []byte {
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
	return append(append(headerPrefix, encodeBlockNumber(number)...), headerHashSuffix...)
}

func finalConsensusKey(hash common.Hash) []byte {
	return append(finalConsensusPrefix, hash.Bytes()...)
}

func txIndexKey(hash common.Hash) []byte {
	return append(transactionIndexPrefix, hash.Bytes()...)
}

func identityStateDiffKey(height uint64) []byte {
	return append(identityStateDiffPrefix, encodeBlockNumber(height)...)
}

func (r *Repo) ReadBlockHeader(hash common.Hash) *types.Header {
	data := r.db.Get(headerKey(hash))
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
	data := r.db.Get(headBlockKey)
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

func (r *Repo) WriteHead(header *types.Header) {
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
		return
	}
	r.db.Set(headBlockKey, data)
}

func (r *Repo) WriteBlockHeader(header *types.Header) {
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}

	r.db.Set(headerKey(header.Hash()), data)
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

func (r *Repo) ReadCanonicalHash(height uint64) common.Hash {
	key := headerHashKey(height)
	data := r.db.Get(key)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

func (r *Repo) WriteFinalConsensus(hash common.Hash) {
	key := finalConsensusKey(hash)
	r.db.Set(key, []byte{0x1})
}

func (r *Repo) SetHead(height uint64) {
	hash := r.ReadCanonicalHash(height)
	if hash != (common.Hash{}) {
		header := r.ReadBlockHeader(hash)
		if header != nil {
			r.WriteHead(header)
		}
	}
}

func (r *Repo) WriteTxIndex(txHash common.Hash, index *types.TransactionIndex) {
	data, err := rlp.EncodeToBytes(index)
	if err != nil {
		log.Crit("failed to RLP encode tranction index", "err", err)
		return
	}
	r.db.Set(txIndexKey(txHash), data)
}

func (r *Repo) ReadTxIndex(hash common.Hash) *types.TransactionIndex {
	key := txIndexKey(hash)
	data := r.db.Get(key)
	if data == nil {
		return nil
	}
	index := new(types.TransactionIndex)
	if err := rlp.Decode(bytes.NewReader(data), index); err != nil {
		log.Error("invalid transaction index RLP", "err", err)
		return nil
	}
	return index
}

func (r *Repo) ReadCertificate(hash common.Hash) types.BlockCert {
	data := r.db.Get(certKey(hash))
	if data == nil {
		return nil
	}
	cert := new(types.BlockCert)
	if err := rlp.Decode(bytes.NewReader(data), cert); err != nil {
		log.Error("Invalid block cert RLP", "err", err)
		return nil
	}
	return *cert
}

type weakCeritificates struct {
	Hashes []common.Hash
}

func (r *Repo) readWeakCertificates() *weakCeritificates {
	data := r.db.Get(weakCertificatesKey)
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
	data := r.db.Get(lastSnapshotKey)
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
	return r.db.Get(identityStateDiffKey(height))
}

func (r *Repo) WritePreliminaryHead(header *types.Header) {
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
		return
	}
	r.db.Set(preliminaryHeadKey, data)
}

func (r *Repo) ReadPreliminaryHead() *types.Header {
	data := r.db.Get(preliminaryHeadKey)
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

func (r *Repo) RemovePreliminaryHead() {
	r.db.Delete(preliminaryHeadKey)
}

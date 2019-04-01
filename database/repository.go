package database

import (
	"bytes"
	"encoding/binary"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/log"
	"idena-go/rlp"
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

func flipEncryptionKey(epoch uint16) []byte {
	enc := make([]byte, 2)
	binary.BigEndian.PutUint16(enc, epoch)
	return append(flipEncryptionPrefix, enc...)
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

func (r *Repo) WriteBlockHeader(block *types.Block) {
	data, err := rlp.EncodeToBytes(block.Header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}

	r.db.Set(headerKey(block.Hash()), data)
}

func (r *Repo) WriteCert(hash common.Hash, cert *types.BlockCert) {
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

func (r *Repo) ReadFlipKey(epoch uint16) []byte {
	key := flipEncryptionKey(epoch)
	return r.db.Get(key)
}

func (r *Repo) WriteFlipKey(epoch uint16, encKey []byte) {
	key := flipEncryptionKey(epoch)
	r.db.Set(key, encKey)
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

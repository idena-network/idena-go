package blockchain

import (
	"bytes"
	"encoding/binary"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/log"
	"idena-go/rlp"
)

type repo struct {
	db dbm.DB
}

func NewRepo(db dbm.DB) *repo {
	return &repo{
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

func bodyKey(hash common.Hash) []byte {
	return append(bodyPrefix, hash.Bytes()...)
}

func certKey(hash common.Hash) [] byte {
	return append(certPrefix, hash.Bytes()...)
}

func headerHashKey(number uint64) []byte {
	return append(append(headerPrefix, encodeBlockNumber(number)...), headerHashSuffix...)
}
func finalConsensusKey(hash common.Hash) []byte {
	return append(finalConsensusPrefix, hash.Bytes()...)
}

func (r *repo) ReadBlockHeader(hash common.Hash) *types.Header {
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

func (r *repo) ReadBlockBody(hash common.Hash) *types.Body {
	data := r.db.Get(bodyKey(hash))
	if data == nil {
		return nil
	}
	body := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	return body
}

func (r *repo) ReadBlock(hash common.Hash) *types.Block {
	header := r.ReadBlockHeader(hash)
	if header == nil {
		return nil
	}

	return &types.Block{
		Header: header,
		Body:   r.ReadBlockBody(hash),
	}
}

func (r *repo) ReadHead() *types.Header {
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

func (r *repo) WriteHead(header *types.Header) {
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	r.db.Set(headBlockKey, data)
}

func (r *repo) WriteBlock(block *types.Block) {
	data, err := rlp.EncodeToBytes(block.Header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}

	r.db.Set(headerKey(block.Hash()), data)
	// body doesn't exist for empty block
	if block.Body == nil {
		return
	}
	data, err = rlp.EncodeToBytes(block.Body)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	r.db.Set(bodyKey(block.Hash()), data)
}

func (r *repo) WriteCert(hash common.Hash, cert *types.BlockCert) {
	data, err := rlp.EncodeToBytes(cert)
	if err != nil {
		log.Crit("Failed to RLP encode block cert", "err", err)
	}
	r.db.Set(certKey(hash), data)
}

func (r *repo) WriteCanonicalHash(height uint64, hash common.Hash) {
	key := headerHashKey(height)
	r.db.Set(key, hash.Bytes())
}

func (r *repo) ReadCanonicalHash(height uint64) common.Hash {
	key := headerHashKey(height)
	data := r.db.Get(key)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

func (r *repo) WriteFinalConsensus(hash common.Hash) {
	key := finalConsensusKey(hash)
	r.db.Set(key, []byte{0x1})
}

func (r *repo) SetHead(height uint64) {
	hash := r.ReadCanonicalHash(height)
	if hash != (common.Hash{}) {
		header := r.ReadBlockHeader(hash)
		if header != nil {
			r.WriteHead(header)
		}
	}
}

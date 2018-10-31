package blockchain

import (
	"bytes"
	"encoding/binary"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/idenadb"
	"idena-go/log"
	"idena-go/rlp"
)


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

func headerHashKey(number uint64) []byte {
	return append(append(headerPrefix, encodeBlockNumber(number)...), headerHashSuffix...)
}

// headerKey = headerPrefix + hash
func blockBodyKey(hash common.Hash) []byte {
	return append(headerPrefix, hash.Bytes()...)
}

func ReadBlockHeader(db idenadb.Database, hash common.Hash ) *types.Header{
	data, _ :=db.Get(headerKey(hash))
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

func ReadBlock (db idenadb.Database, hash common.Hash) *types.Block{
	header := ReadBlockHeader(db, hash)
	if header == nil {
		return nil
	}
	return &types.Block{
		Header : header,
	}
}

func ReadHead(db idenadb.Database) *types.Header{
	data, _ :=db.Get(headBlockKey)
	if data == nil {
		return nil
	}
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP",  "err", err)
		return nil
	}
	return header
}


func WriteHead(db idenadb.Database, header *types.Header){
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}

	if err := db.Put(headBlockKey, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}

func WriteBlock(db idenadb.Database, block *types.Block){
	data, err := rlp.EncodeToBytes(block.Header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}

	if err := db.Put(headerKey(block.Hash()), data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}

func WriteCanonicalHash(db idenadb.Database, height uint64, hash common.Hash ){
	key := headerHashKey(height)
	if err := db.Put(key, hash.Bytes()); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}


func ReadCanonicalHash(db idenadb.Database, height uint64 ) common.Hash{
	key := headerHashKey(height)
	data, _ := db.Get(key)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}


package database

import (
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/rlp"
)

var (
	ShortAnswerKey   = []byte("short")
	AnswerHashPrefix = []byte("hash")
)

type EpochDb struct {
	db dbm.DB
}

func NewEpochDb(db dbm.DB, epoch uint16) *EpochDb {
	prefix := []byte("epoch")
	prefix = append(prefix, uint8(epoch>>8), uint8(epoch&0xff))
	return &EpochDb{db: dbm.NewPrefixDB(db, prefix)}
}

func (edb EpochDb) Clear() {
	it := edb.db.Iterator(nil, nil)
	var keys [][]byte

	for it.Next(); it.Valid(); {
		keys = append(keys, it.Key())
	}
	for _, key := range keys {
		edb.db.Delete(key)
	}
}

func (edb EpochDb) WriteAnswerHash(address common.Address, hash common.Hash) {
	edb.db.Set(append(AnswerHashPrefix, address.Bytes()...), hash[:])
}

func (edb *EpochDb) GetAnswers() map[common.Address]common.Hash {
	it := edb.db.Iterator(append(AnswerHashPrefix, common.MinHash[:]...), append(AnswerHashPrefix, common.MaxHash[:]...))
	answers := make(map[common.Address]common.Hash)

	for it.Next(); it.Valid(); {
		answers[common.BytesToAddress(it.Key())] = common.BytesToHash(it.Value())
	}
	return answers
}

func (edb EpochDb) WriteOwnShortAnswers(answers []*types.FlipAnswer) error {
	data, err := rlp.EncodeToBytes(answers)
	if err != nil {
		return errors.New("failed to RLP encode answers")
	}
	edb.db.Set(ShortAnswerKey, data)
	return nil
}

package database

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	dbm "github.com/tendermint/tm-db"
	"time"
)

var (
	OwnShortAnswerKey     = []byte("own-short")
	AnswerHashPrefix      = []byte("hash")
	ShortAnswersKey       = []byte("answers-short")
	LongShortAnswersKey   = []byte("answers-long")
	TxOwnPrefix           = []byte("tx")
	SuccessfulTxOwnPrefix = []byte("s-tx")
	EvidencePrefix        = []byte("evi")
	LotterySeedKey        = []byte("ls")
	FlipCidPrefix         = []byte("cid")
)

type EpochDb struct {
	db dbm.DB
}

type shortAnswerDb struct {
	Hash      common.Hash
	Timestamp uint64
}

type DbAnswer struct {
	Addr common.Address
	Ans  []byte
}

type DbProof struct {
	Addr  common.Address
	Proof []byte
}

func NewEpochDb(db dbm.DB, epoch uint16) *EpochDb {
	prefix := append([]byte("epoch"), uint8(epoch>>8), uint8(epoch&0xff))
	return &EpochDb{db: dbm.NewPrefixDB(db, prefix)}
}

func assertNoError(err error) {
	if err != nil {
		panic(err)
	}
}

func (edb *EpochDb) Clear() {
	it, err := edb.db.Iterator(nil, nil)
	assertNoError(err)
	defer it.Close()

	var keys [][]byte

	for ; it.Valid(); it.Next() {
		keys = append(keys, it.Key())
	}
	for _, key := range keys {
		assertNoError(edb.db.Delete(key))
	}
}

func (edb *EpochDb) WriteAnswerHash(address common.Address, hash common.Hash, timestamp time.Time) {

	a := shortAnswerDb{
		Hash:      hash,
		Timestamp: uint64(timestamp.Unix()),
	}
	encoded, _ := rlp.EncodeToBytes(a)
	assertNoError(edb.db.Set(append(AnswerHashPrefix, address.Bytes()...), encoded))
}

func (edb *EpochDb) GetAnswers() map[common.Address]common.Hash {
	it, err := edb.db.Iterator(append(AnswerHashPrefix, common.MinAddr[:]...), append(AnswerHashPrefix, common.MaxAddr[:]...))
	assertNoError(err)
	defer it.Close()

	answers := make(map[common.Address]common.Hash)

	for ; it.Valid(); it.Next() {
		a := &shortAnswerDb{}
		if err := rlp.DecodeBytes(it.Value(), a); err != nil {
			log.Error("invalid short answers rlp", "err", err)
		} else {
			answers[common.BytesToAddress(it.Key())] = a.Hash
		}
	}
	return answers
}

func (edb *EpochDb) GetAnswerHash(address common.Address) common.Hash {
	key := append(AnswerHashPrefix, address.Bytes()...)
	data, err := edb.db.Get(key)
	assertNoError(err)
	if data == nil {
		return common.Hash{}
	}
	a := &shortAnswerDb{}
	if err := rlp.DecodeBytes(data, a); err != nil {
		log.Error("invalid short answers rlp", "err", err)
		return common.Hash{}
	}
	return a.Hash
}

func (edb *EpochDb) GetConfirmedRespondents(start time.Time, end time.Time) []common.Address {
	it, err := edb.db.Iterator(append(AnswerHashPrefix, common.MinAddr[:]...), append(AnswerHashPrefix, common.MaxAddr[:]...))
	assertNoError(err)
	defer it.Close()
	var result []common.Address
	for ; it.Valid(); it.Next() {
		a := &shortAnswerDb{}
		if err := rlp.DecodeBytes(it.Value(), a); err != nil {
			log.Error("invalid short answers rlp", "err", err)
		}

		t := time.Unix(int64(a.Timestamp), 0)
		if t.After(start) && t.Before(end) {
			result = append(result, common.BytesToAddress(it.Key()))
		}
	}
	return result
}

func (edb *EpochDb) WriteOwnShortAnswers(answers *types.Answers) {
	assertNoError(edb.db.Set(OwnShortAnswerKey, answers.Bytes()))
}

func (edb *EpochDb) ReadOwnShortAnswersBits() []byte {
	data, err := edb.db.Get(OwnShortAnswerKey)
	assertNoError(err)
	return data
}

func (edb *EpochDb) WriteAnswers(short []DbAnswer, long []DbAnswer) {

	shortRlp, _ := rlp.EncodeToBytes(short)
	longRlp, _ := rlp.EncodeToBytes(long)

	assertNoError(edb.db.Set(ShortAnswersKey, shortRlp))
	assertNoError(edb.db.Set(LongShortAnswersKey, longRlp))
}

func (edb *EpochDb) ReadAnswers() (short []DbAnswer, long []DbAnswer) {
	shortRlp, err := edb.db.Get(ShortAnswersKey)
	assertNoError(err)
	longRlp, err := edb.db.Get(LongShortAnswersKey)
	assertNoError(err)

	if shortRlp != nil {
		if err := rlp.Decode(bytes.NewReader(shortRlp), &short); err != nil {
			log.Error("invalid short answers rlp", "err", err)
		}
	}
	if longRlp != nil {
		if err := rlp.Decode(bytes.NewReader(longRlp), &long); err != nil {
			log.Error("invalid long answers rlp", "err", err)
		}
	}
	return short, long
}

func (edb *EpochDb) ReadEvidenceMaps() [][]byte {
	it, err := edb.db.Iterator(append(EvidencePrefix, common.MinAddr[:]...), append(EvidencePrefix, common.MaxAddr[:]...))
	assertNoError(err)
	defer it.Close()
	var result [][]byte
	for ; it.Valid(); it.Next() {
		result = append(result, it.Value())
	}
	return result
}

func (edb *EpochDb) WriteEvidenceMap(addr common.Address, bitmap []byte) {
	edb.db.Set(append(EvidencePrefix, addr[:]...), bitmap)
}

func (edb *EpochDb) WriteOwnTx(txType uint16, tx []byte) {
	key := append(TxOwnPrefix, uint8(txType>>8), uint8(txType&0xff))
	edb.db.Set(key, tx)
}

func (edb *EpochDb) RemoveOwnTx(txType uint16) {
	key := append(TxOwnPrefix, uint8(txType>>8), uint8(txType&0xff))
	edb.db.Delete(key)
}

func (edb *EpochDb) ReadOwnTx(txType uint16) []byte {
	key := append(TxOwnPrefix, uint8(txType>>8), uint8(txType&0xff))
	data, err := edb.db.Get(key)
	assertNoError(err)
	return data
}

func (edb *EpochDb) WriteSuccessfulOwnTx(txHash common.Hash) {
	key := append(SuccessfulTxOwnPrefix, txHash.Bytes()...)
	assertNoError(edb.db.Set(key, []byte{}))
}

func (edb *EpochDb) HasSuccessfulOwnTx(txHash common.Hash) bool {
	key := append(SuccessfulTxOwnPrefix, txHash.Bytes()...)
	has, err := edb.db.Has(key)
	assertNoError(err)
	return has
}

func (edb *EpochDb) WriteLotterySeed(seed []byte) {
	assertNoError(edb.db.Set(LotterySeedKey, seed))
}

func (edb *EpochDb) ReadLotterySeed() []byte {
	data, err := edb.db.Get(LotterySeedKey)
	assertNoError(err)
	return data
}

func (edb *EpochDb) WriteFlipCid(cid []byte) {
	assertNoError(edb.db.Set(append(FlipCidPrefix, cid...), []byte{}))
}

func (edb *EpochDb) HasFlipCid(cid []byte) bool {
	has, err := edb.db.Has(append(FlipCidPrefix, cid...))
	assertNoError(err)
	return has
}

func (edb *EpochDb) DeleteFlipCid(cid []byte) {
	edb.db.Delete(append(FlipCidPrefix, cid...))
}

func (edb *EpochDb) IterateOverFlipCids(callback func(cid []byte)) {
	it, err := edb.db.Iterator(append(FlipCidPrefix, ipfs.MinCid[:]...), append(FlipCidPrefix, ipfs.MaxCid[:]...))
	assertNoError(err)
	defer it.Close()
	for ; it.Valid(); it.Next() {
		callback(it.Key()[len(FlipCidPrefix):])
	}
}

func (edb *EpochDb) HasEvidenceMap(addr common.Address) bool {
	key := append(EvidencePrefix, addr[:]...)
	has, err := edb.db.Has(key)
	assertNoError(err)
	return has
}

func (edb *EpochDb) HasAnswerHash(addr common.Address) bool {
	key := append(AnswerHashPrefix, addr.Bytes()...)
	has, err := edb.db.Has(key)
	assertNoError(err)
	return has
}

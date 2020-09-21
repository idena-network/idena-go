package database

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
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
	PublicFlipKeyPrefix   = []byte("pubk")
	PrivateFlipKeyPrefix  = []byte("pk")
)

type EpochDb struct {
	db dbm.DB
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
	protoAnswer := &models.ProtoShortAnswerDb{
		Hash:      hash[:],
		Timestamp: timestamp.Unix(),
	}
	data, _ := proto.Marshal(protoAnswer)
	assertNoError(edb.db.Set(append(AnswerHashPrefix, address.Bytes()...), data))
}

func (edb *EpochDb) GetAnswers() map[common.Address]common.Hash {
	it, err := edb.db.Iterator(append(AnswerHashPrefix, common.MinAddr[:]...), append(AnswerHashPrefix, common.MaxAddr[:]...))
	assertNoError(err)
	defer it.Close()

	answers := make(map[common.Address]common.Hash)

	for ; it.Valid(); it.Next() {
		protoAnswer := new(models.ProtoShortAnswerDb)
		if err := proto.Unmarshal(it.Value(), protoAnswer); err != nil {
			log.Error("invalid short answers protobuf model", "err", err)
		} else {
			answers[common.BytesToAddress(it.Key())] = common.BytesToHash(protoAnswer.Hash)
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
	protoAnswer := new(models.ProtoShortAnswerDb)
	if err := proto.Unmarshal(data, protoAnswer); err != nil {
		log.Error("invalid short answers protobuf model", "err", err)
		return common.Hash{}
	}
	return common.BytesToHash(protoAnswer.Hash)
}

func (edb *EpochDb) GetConfirmedRespondents(start time.Time, end time.Time) []common.Address {
	it, err := edb.db.Iterator(append(AnswerHashPrefix, common.MinAddr[:]...), append(AnswerHashPrefix, common.MaxAddr[:]...))
	assertNoError(err)
	defer it.Close()
	var result []common.Address
	for ; it.Valid(); it.Next() {
		protoAnswer := new(models.ProtoShortAnswerDb)
		if err := proto.Unmarshal(it.Value(), protoAnswer); err != nil {
			log.Error("invalid short answers protobuf model", "err", err)
			continue
		}

		t := time.Unix(int64(protoAnswer.Timestamp), 0)
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

	toBytes := func(a []DbAnswer) ([]byte, error) {
		protoAnswers := new(models.ProtoAnswersDb)
		for idx := range a {
			protoAnswers.Answers = append(protoAnswers.Answers, &models.ProtoAnswersDb_Answer{
				Address: a[idx].Addr[:],
				Answers: a[idx].Ans,
			})
		}
		return proto.Marshal(protoAnswers)
	}

	shortRlp, _ := toBytes(short)
	longRlp, _ := toBytes(long)

	assertNoError(edb.db.Set(ShortAnswersKey, shortRlp))
	assertNoError(edb.db.Set(LongShortAnswersKey, longRlp))
}

func (edb *EpochDb) ReadAnswers() (short []DbAnswer, long []DbAnswer) {

	fromBytes := func(data []byte) ([]DbAnswer, error) {
		protoAnswers := new(models.ProtoAnswersDb)
		if err := proto.Unmarshal(data, protoAnswers); err != nil {
			return nil, err
		}
		var a []DbAnswer
		for _, item := range protoAnswers.Answers {
			a = append(a, DbAnswer{
				Addr: common.BytesToAddress(item.Address),
				Ans:  item.Answers,
			})
		}
		return a, nil
	}

	shortProto, err := edb.db.Get(ShortAnswersKey)
	assertNoError(err)
	longProto, err := edb.db.Get(LongShortAnswersKey)
	assertNoError(err)

	if shortProto != nil {
		if short, err = fromBytes(shortProto); err != nil {
			log.Error("invalid short answers protobuf model", "err", err)
		}
	}
	if longProto != nil {
		if long, err = fromBytes(longProto); err != nil {
			log.Error("invalid long answers protobuf model", "err", err)
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

func (edb *EpochDb) WritePublicFlipKey(key *types.PublicFlipKey) {
	data, _ := key.ToBytes()
	hash := key.Hash()
	err := edb.db.Set(append(PublicFlipKeyPrefix, hash.Bytes()...), data)
	assertNoError(err)
}


func (edb *EpochDb) ReadPublicFlipKeys() []*types.PublicFlipKey {
	it, err := edb.db.Iterator(append(PublicFlipKeyPrefix, common.MinHash[:]...), append(PublicFlipKeyPrefix, common.MaxHash...))
	assertNoError(err)
	defer it.Close()
	var result []*types.PublicFlipKey
	for ; it.Valid(); it.Next() {

		key := &types.PublicFlipKey{}
		key.FromBytes(it.Value())
		result = append(result, key)
	}
	return result
}

func (edb *EpochDb) WritePrivateFlipKey(key *types.PrivateFlipKeysPackage) {
	data, _ := key.ToBytes()
	hash := key.Hash128()
	err := edb.db.Set(append(PrivateFlipKeyPrefix, hash.Bytes()...), data)
	assertNoError(err)
}


func (edb *EpochDb) ReadPrivateFlipKeys() []*types.PrivateFlipKeysPackage {
	it, err := edb.db.Iterator(append(PrivateFlipKeyPrefix, common.MinHash128[:]...), append(PrivateFlipKeyPrefix, common.MaxHash128...))
	assertNoError(err)
	defer it.Close()
	var result []*types.PrivateFlipKeysPackage
	for ; it.Valid(); it.Next() {

		key := &types.PrivateFlipKeysPackage{}
		key.FromBytes(it.Value())
		result = append(result, key)
	}
	return result
}



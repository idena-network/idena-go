package database

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-cmn/db"
	"testing"
	"time"
)

func TestEpochDb_Write_Read_ShortSessionTime(t *testing.T) {
	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)

	timestamp := time.Now()

	edb.WriteShortSessionTime(timestamp)

	read := edb.ReadShortSessionTime()

	require.Equal(t, timestamp.Unix(), read.Unix())
}

func TestEpochDb_GetConfirmedRespondents(t *testing.T) {

	require := require.New(t)

	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)

	timestamp := time.Now()

	addr1 := tests.GetRandAddr()
	addr2 := tests.GetRandAddr()
	addr3 := tests.GetRandAddr()

	edb.WriteAnswerHash(addr1, common.Hash{}, timestamp.Add(time.Second))
	edb.WriteAnswerHash(addr2, common.Hash{}, timestamp.Add(time.Second*2))
	edb.WriteAnswerHash(addr3, common.Hash{}, timestamp.Add(time.Second*5))

	respondents := edb.GetConfirmedRespondents(timestamp, timestamp.Add(time.Second*4))

	require.Equal(2, len(respondents))

	require.Contains(respondents, addr1)
	require.Contains(respondents, addr2)
	require.NotContains(respondents, addr3)
}

func TestEpochDb_IterateOverFlipCids(t *testing.T) {
	require := require.New(t)
	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)

	edb.WriteFlipCid([]byte{0x1}, 0)
	edb.WriteFlipCid([]byte{0x2}, 1)

	//write trash
	edb.WriteLotterySeed([]byte{0x3})
	edb.WriteOwnTx(1, []byte{0x1})
	edb.WriteShortSessionTime(time.Now())

	cids := make([][1]byte, 0)
	edb.IterateOverFlipCids(func(cid []byte) {
		var arr [1]byte

		copy(arr[:], cid)
		cids = append(cids, arr)
	})

	require.Contains(cids, [1]byte{0x1})
	require.Contains(cids, [1]byte{0x2})
	require.Len(cids, 2)
}

func TestEpochDb_Write_Read_FlipPairs(t *testing.T) {
	require := require.New(t)
	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)

	edb.WriteFlipCid([]byte{0x1}, 0)
	edb.WriteFlipCid([]byte{0x2}, 1)
	edb.WriteFlipCid([]byte{0x3}, 3)

	require.Equal(uint8(0), *edb.ReadFlipPair([]byte{0x1}))
	require.Equal(uint8(1), *edb.ReadFlipPair([]byte{0x2}))
	require.Equal(uint8(3), *edb.ReadFlipPair([]byte{0x3}))
	require.Nil(edb.ReadFlipPair([]byte{0x5}))
}

func TestEpochDb_Write_Read_FlipKeyWordPairs(t *testing.T) {
	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)

	words := []uint32{3000, 2000, 10}
	proof := []byte{225, 111, 33, 5, 19}

	edb.WriteFlipKeyWordPairs(words, proof)

	readWords, readProof := edb.ReadFlipKeyWordPairs()

	require.Equal(t, 3, len(readWords))
	require.Equal(t, 5, len(readProof))

	for i := 0; i < len(readWords); i++ {
		require.Equal(t, words[i], readWords[i])
	}
	for i := 0; i < len(readProof); i++ {
		require.Equal(t, proof[i], readProof[i])
	}
}

func TestEpochDb_GetAnswers(t *testing.T) {
	require := require.New(t)

	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)

	timestamp := time.Now()

	addr1 := tests.GetRandAddr()
	addr2 := tests.GetRandAddr()
	addr3 := tests.GetRandAddr()

	edb.WriteAnswerHash(addr1, common.Hash{}, timestamp.Add(time.Second))
	edb.WriteAnswerHash(addr2, common.Hash{}, timestamp.Add(time.Second*2))
	edb.WriteAnswerHash(addr3, common.Hash{}, timestamp.Add(time.Second*5))

	answers := edb.GetAnswers()
	require.Len(answers, 3)
	require.Contains(answers, addr1, addr2, addr3)
}

func TestEpochDb_HasEvidenceMap(t *testing.T) {
	require := require.New(t)

	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)
	addr := tests.GetRandAddr()

	edb.WriteEvidenceMap(addr, []byte{0x1})
	require.True(edb.HasEvidenceMap(addr))
}

func TestEpochDb_HasAnswerHash(t *testing.T) {
	require := require.New(t)

	mdb := db.NewMemDB()

	edb := NewEpochDb(mdb, 1)
	addr := tests.GetRandAddr()
	edb.WriteAnswerHash(addr, common.Hash{0x1}, time.Now())
	require.True(edb.HasAnswerHash(addr))
}

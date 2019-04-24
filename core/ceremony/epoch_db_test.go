package ceremony

import (
	"github.com/google/tink/go/subtle/random"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"testing"
	"time"
)

func getRandAddr() common.Address {
	addr := common.Address{}
	addr.SetBytes(random.GetRandomBytes(20))
	return addr
}

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

	addr1 := getRandAddr()
	addr2 := getRandAddr()
	addr3 := getRandAddr()

	edb.WriteAnswerHash(addr1, common.Hash{}, timestamp.Add(time.Second))
	edb.WriteAnswerHash(addr2, common.Hash{}, timestamp.Add(time.Second*2))
	edb.WriteAnswerHash(addr3, common.Hash{}, timestamp.Add(time.Second*5))

	respondents := edb.GetConfirmedRespondents(timestamp, timestamp.Add(time.Second*4))

	require.Equal(2, len(respondents))

	require.Contains(respondents, addr1)
	require.Contains(respondents, addr2)
	require.NotContains(respondents, addr3)
}

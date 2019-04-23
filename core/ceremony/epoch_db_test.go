package ceremony

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
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

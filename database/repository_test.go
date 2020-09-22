package database

import (
	"crypto/rand"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"testing"
	"time"
)

func getRandHash() common.Hash {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	hash := common.Hash{}
	hash.SetBytes(bytes)
	return hash
}

func TestRepo_WriteWeakCertificate(t *testing.T) {
	database := db.NewMemDB()
	repo := NewRepo(database)

	hash1 := getRandHash()
	repo.WriteCertificate(hash1, &types.BlockCert{})
	repo.WriteWeakCertificate(hash1)

	hash2 := getRandHash()
	cert := types.FullBlockCert{Votes: []*types.Vote{{
		Header:    &types.VoteHeader{},
		Signature: []byte("123"),
	}, {
		Header:    &types.VoteHeader{},
		Signature: []byte("1234"),
	}}}
	repo.WriteCertificate(hash2, cert.Compress())
	repo.WriteWeakCertificate(hash2)

	for i := 0; i < MaxWeakCertificatesCount-1; i++ {
		h := getRandHash()
		repo.WriteCertificate(h, &types.BlockCert{})
		repo.WriteWeakCertificate(h)
	}
	require := require.New(t)

	require.Nil(repo.ReadCertificate(hash1))
	require.NotNil(repo.ReadCertificate(hash2))
	require.Len(repo.ReadCertificate(hash2).Signatures, 2)
	weakCerts := repo.readWeakCertificates()
	require.Equal(MaxWeakCertificatesCount, len(weakCerts.Hashes))
}

func TestRepo_WritActivityMonitor(t *testing.T) {
	database := db.NewMemDB()
	repo := NewRepo(database)

	addr := common.Address{0x1}

	monitor := &types.ActivityMonitor{
		UpdateDt: time.Now().UTC(),
		Data:     []*types.AddrActivity{{Time: time.Now().UTC(), Addr: addr}},
	}

	repo.WriteActivity(monitor)

	readActivity := repo.ReadActivity()

	require := require.New(t)

	require.Equal(monitor.UpdateDt.Unix(), readActivity.UpdateDt.Unix())
	require.Equal(len(monitor.Data), len(readActivity.Data))
	require.Equal(monitor.Data[0].Addr, readActivity.Data[0].Addr)
	require.Equal(monitor.Data[0].Time.Unix(), readActivity.Data[0].Time.Unix())
}

func TestRepo_GetSavedEvents(t *testing.T) {
	database := db.NewMemDB()
	repo := NewRepo(database)

	addr1 := common.Address{1}
	addr2 := common.Address{2}

	tx1 := common.Hash{0x1}
	tx2 := common.Hash{0x2}

	repo.WriteEvent(addr1, tx1, 1, &types.TxEvent{
		EventName: "event2",
		Data:      [][]byte{{0x1}},
	})
	repo.WriteEvent(addr1, tx1, 2, &types.TxEvent{
		EventName: "event1",
		Data:      [][]byte{{0x1}},
	})
	repo.WriteEvent(addr1, tx1, 3, &types.TxEvent{
		EventName: "event1",
		Data:      [][]byte{{0x1}},
	})
	repo.WriteEvent(addr2, tx2, 1, &types.TxEvent{
		EventName: "ZZZZZZZZZZZZZZZ ZZZZZZZZZZZZZZZZZZ",
		Data:      [][]byte{{0x1}},
	})
	repo.WriteEvent(addr2, tx2, 2, &types.TxEvent{
		EventName: "e",
		Data:      [][]byte{{0x1}},
	})
	repo.WriteEvent(addr2, tx2, 3, &types.TxEvent{
		EventName: "ev2",
		Data:      [][]byte{{0x1}},
	})

	events1 := repo.GetSavedEvents(addr1)
	events2 := repo.GetSavedEvents(addr2)
	require.Len(t, events1, 3)
	require.Len(t, events2, 3)

	require.Equal(t, "event1", events1[0].Event)
	require.Equal(t, "event1", events1[1].Event)
	require.Equal(t, "event2", events1[2].Event)
	require.Equal(t, "ev2", events2[0].Event)
	require.Equal(t, "e", events2[1].Event)
	require.Equal(t, "ZZZZZZZZZZZZZZZ ZZZZZZZZZZZZZZZZZZ", events2[2].Event)

}

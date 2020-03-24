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

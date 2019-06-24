package database

import (
	"crypto/rand"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"testing"
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
	repo.WriteCertificate(hash2, &types.BlockCert{})
	repo.WriteWeakCertificate(hash2)

	for i := 0; i < MaxWeakCertificatesCount-1; i++ {
		h := getRandHash()
		repo.WriteCertificate(h, &types.BlockCert{})
		repo.WriteWeakCertificate(h)
	}
	require := require.New(t)

	require.Nil(repo.ReadCertificate(hash1))
	require.NotNil(repo.ReadCertificate(hash2))

	weakCerts := repo.readWeakCertificates()
	require.Equal(MaxWeakCertificatesCount, len(weakCerts.Hashes))

}

package mempool

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_getPrivateKeysPackage(t *testing.T) {
	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	publicEncKey := ecies.ImportECDSA(key1)
	privateEncKey := ecies.ImportECDSA(key2)

	var pk []*ecdsa.PrivateKey
	var pubkeys [][]byte
	for i := 0; i < 10; i++ {
		k, _ := crypto.GenerateKey()
		pk = append(pk, k)
		pubkeys = append(pubkeys, crypto.FromECDSAPub(&k.PublicKey))
	}

	dataToAssert := crypto.FromECDSA(privateEncKey.ExportECDSA())

	for i := 0; i < 10; i++ {
		encryptedData := EncryptPrivateKeysPackage(publicEncKey, privateEncKey, pubkeys)

		encryptedKey, err := getEncryptedKeyFromPackage(publicEncKey, encryptedData, i)
		require.NoError(t, err)

		result, err := ecies.ImportECDSA(pk[i]).Decrypt(encryptedKey, nil, nil)
		require.NoError(t, err)

		require.Equal(t, dataToAssert, result)
	}
}

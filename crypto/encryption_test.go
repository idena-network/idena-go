package crypto

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	data := []byte{0x1, 0x2, 0x3}
	pass := "123456abc"
	encrypted := Encrypt(data, pass)
	decrypted := Decrypt(encrypted, pass)

	require.Equal(t, data, decrypted)
}

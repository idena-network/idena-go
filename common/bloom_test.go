package common

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSerializableBF_Has(t *testing.T) {
	b := NewSerializableBF(100)
	addr1 := Address{0x1}
	addr2 := Address{0x2}
	addr3 := Address{0x3}
	addr4 := Address{0x4}

	b.Add(addr1)
	b.Add(addr2)
	b.Add(addr3)
	b.Add(addr4)

	bloomData, err := b.Serialize()
	require.NoError(t, err)

	b2, err := NewSerializableBFFromData(bloomData)

	require.NoError(t, err)
	require.Equal(t, b.data, b2.data)

	require.True(t, b2.Has(addr1))
	require.True(t, b2.Has(addr2))
	require.True(t, b2.Has(addr3))
	require.True(t, b2.Has(addr4))
	require.False(t, b2.Has(Address{0x5}))
}

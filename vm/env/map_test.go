package env

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewMap(t *testing.T) {
	m := NewMap([]byte{0x1}, nil, nil)
	require.Equal(t, []byte{0x1}, m.prefix)

	prefix := make([]byte, 32)
	prefix[31] = 1

	m = NewMap(prefix, nil, nil)
	require.Equal(t, prefix[:30], m.prefix)
}

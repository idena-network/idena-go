package maputil

import (
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestIsSyncMapEmpty(t *testing.T) {
	m := sync.Map{}
	require.True(t, IsSyncMapEmpty(&m))

	m.Store(1, 2)
	require.False(t, IsSyncMapEmpty(&m))

	m.Delete(1)
	require.True(t, IsSyncMapEmpty(&m))
}

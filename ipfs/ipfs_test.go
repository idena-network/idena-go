package ipfs

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIpfsProxy_Cid(t *testing.T) {

	require := require.New(t)
	var data []byte

	proxy := ipfsProxy{}
	cid, err := proxy.Cid(data)

	require.Nil(err)
	require.Equal(EmptyCid, cid)
}


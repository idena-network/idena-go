package ipfs

import (
	"github.com/stretchr/testify/require"
	"idena-go/config"
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

func TestIpfsProxy_Get(t *testing.T) {
	t.SkipNow()

	require := require.New(t)

	proxy, _ := NewIpfsProxy(&config.IpfsConfig{
		BootNodes: []string{},
		IpfsPort:  4002,
		DataDir:   ".",
	})

	data := []byte{0x1, 0x2, 0x3}

	cid, err := proxy.Add(data)

	require.NoError(err)

	localCid, err := proxy.Cid(data)

	require.NoError(err)

	require.Equal(cid, localCid)

	data2, err := proxy.Get(cid.Bytes())

	require.Equal(data, data2)
}

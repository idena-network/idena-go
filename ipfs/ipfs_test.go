package ipfs

import (
	"github.com/google/tink/go/subtle/random"
	"github.com/idena-network/idena-go/config"
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

func TestIpfsProxy_Get(t *testing.T) {
	require := require.New(t)

	proxy, _ := NewIpfsProxy(&config.IpfsConfig{
		SwarmKey:  "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530",
		BootNodes: []string{},
		IpfsPort:  4012,
		DataDir:   "./datadir-ipfs",
	})

	cases := []int{1, 100, 500, 1024, 10000, 50000, 100000, 220000, 280000, 350000, 500000, 1000000}

	for _, item := range cases {
		data := random.GetRandomBytes(uint32(item))

		cid, err := proxy.Add(data)

		require.NoError(err)

		localCid, err := proxy.Cid(data)

		require.NoError(err)

		require.Equal(cid.Bytes(), localCid.Bytes(), "n: %v", item)

		data2, err := proxy.Get(cid.Bytes())

		require.Equal(data, data2)
	}
}

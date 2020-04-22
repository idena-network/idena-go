package ipfs

import (
	"github.com/google/tink/go/subtle/random"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/stretchr/testify/require"
	"testing"
)

var proxy Proxy

func init() {
	var err error
	proxy, err = NewIpfsProxy(&config.IpfsConfig{
		SwarmKey:    "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530",
		BootNodes:   []string{},
		IpfsPort:    4012,
		DataDir:     "./datadir-ipfs",
		GracePeriod: "20s",
	}, eventbus.New())
	if err != nil {
		panic(err)
	}
}

func TestIpfsProxy_Cid(t *testing.T) {

	require := require.New(t)
	var data []byte

	proxy := ipfsProxy{}
	cid, err := proxy.Cid(data)

	require.Nil(err)
	require.Equal(EmptyCid, cid)
}

func TestIpfsProxy_Get_Cid(t *testing.T) {
	require := require.New(t)

	cid, _ := proxy.Cid([]byte{0x1})
	cid2, _ := proxy.Cid([]byte{0x1})

	require.Equal(cid.Bytes(), cid2.Bytes())
	p := proxy.(*ipfsProxy)
	require.Len(p.cidCache.Items(), 1)

	cases := []int{1, 100, 500, 1024, 10000, 50000, 100000, 220000, 280000, 350000, 500000, 1000000}

	for _, item := range cases {
		data := random.GetRandomBytes(uint32(item))

		cid, err := proxy.Add(data, false)

		require.NoError(err)

		localCid, err := proxy.Cid(data)

		require.NoError(err)

		require.Equal(cid.Bytes(), localCid.Bytes(), "n: %v", item)

		data2, err := proxy.Get(cid.Bytes(), Block)

		require.Equal(data, data2)
	}
}

func TestIpfsProxy_Get_Limit(t *testing.T) {
	require := require.New(t)

	data := random.GetRandomBytes(uint32(common.MaxFlipSize))
	cid, err := proxy.Add(data, false)
	require.NoError(err)

	_, err = proxy.Get(cid.Bytes(), Flip)
	require.NoError(err)

	_, err = proxy.Get(cid.Bytes(), Profile)
	require.NoError(err)

	_, err = proxy.Get(cid.Bytes(), Block)
	require.NoError(err)

	data = random.GetRandomBytes(uint32(common.MaxProfileSize))
	cid, err = proxy.Add(data, false)
	require.NoError(err)

	_, err = proxy.Get(cid.Bytes(), Flip)
	require.Equal(TooBigErr, err)

	_, err = proxy.Get(cid.Bytes(), Profile)
	require.NoError(err)

	data = random.GetRandomBytes(uint32(common.MaxProfileSize + 1))
	cid, err = proxy.Add(data, false)
	require.NoError(err)

	_, err = proxy.Get(cid.Bytes(), Flip)
	require.Equal(TooBigErr, err)

	_, err = proxy.Get(cid.Bytes(), Profile)
	require.Equal(TooBigErr, err)
	_, err = proxy.Get(cid.Bytes(), Block)
	require.NoError(err)
}

package types

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBody_ToIpfs(t *testing.T) {
	require := require.New(t)
	body := Body{
		Transactions: Transactions{
			&Transaction{
				Epoch: uint16(1),
			},
			&Transaction{
				Epoch: uint16(2),
			},
			&Transaction{
				Epoch: uint16(3),
			},
		},
	}

	txs := body.ToIpfs()
	require.Equal(len(body.Transactions), len(txs))

	txs["test"] = []byte{1, 2, 3}
	txs["10_test"] = []byte{1, 2, 3}
	txs["1_test"] = []byte{1, 2, 3}
	body2 := Body{}
	body2.FromIpfs(txs)

	require.Equal(len(body.Transactions), len(body2.Transactions))

}

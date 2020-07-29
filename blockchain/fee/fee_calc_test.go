package fee

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestCalculateFee(t *testing.T) {
	tx := &types.Transaction{
		Type: types.SendTx,
	}
	fee1 := big.NewInt(69e+15)
	fee2 := big.NewInt(345e+15)

	// not signed
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, new(big.Int).Div(common.DnaBase, big.NewInt(1000)), tx)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(200, new(big.Int).Div(common.DnaBase, big.NewInt(200)), tx)))

	key, _ := crypto.GenerateKey()
	signed, _ := types.SignTx(tx, key)

	// signed
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, new(big.Int).Div(common.DnaBase, big.NewInt(1000)), signed)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(200, new(big.Int).Div(common.DnaBase, big.NewInt(200)), signed)))
}

func TestCalculateCost(t *testing.T) {
	tx := &types.Transaction{
		Type:   types.SendTx,
		Amount: big.NewInt(1e+18),
		Tips:   big.NewInt(1e+18),
	}
	cost := new(big.Int).Add(big.NewInt(89e+16), tx.AmountOrZero())
	cost.Add(cost, tx.TipsOrZero())
	require.Equal(t, 0, cost.Cmp(CalculateCost(100, new(big.Int).Div(common.DnaBase, big.NewInt(100)), tx)))
}

func TestCalculateCostForInvitation(t *testing.T) {
	// tx size = 17
	tx := &types.Transaction{
		Type:   types.InviteTx,
		Amount: big.NewInt(1e+18),
	}
	const networkSize = 100

	require.Equal(t, 0, tx.AmountOrZero().Cmp(CalculateCost(networkSize, new(big.Int).Div(common.DnaBase, big.NewInt(100)), tx)))
}

func Test_GetFeePerByteForNetwork(t *testing.T) {
	require := require.New(t)

	require.Zero(big.NewInt(1e+17).Cmp(GetFeePerGasForNetwork(0)))

	require.Zero(big.NewInt(1e+16).Cmp(GetFeePerGasForNetwork(10)))

	require.Zero(big.NewInt(1e+15).Cmp(GetFeePerGasForNetwork(100)))

	require.Zero(big.NewInt(2e+13).Cmp(GetFeePerGasForNetwork(5000)))

	require.Zero(big.NewInt(1e+12).Cmp(GetFeePerGasForNetwork(100000)))

	require.Zero(big.NewInt(1e+2).Cmp(GetFeePerGasForNetwork(1e+17)))

	require.Zero(big.NewInt(1e+2).Cmp(GetFeePerGasForNetwork(1e+18)))
}

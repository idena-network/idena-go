package wasm

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/vm/wasm/testdata"
	"github.com/idena-network/idena-wasm-binding/lib"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func createHeader(height uint64, time int64) *types.Header {
	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(height))
	return &types.Header{
		ProposedHeader: &types.ProposedHeader{
			BlockSeed: seed,
			Height:    height,
			Time:      time,
		},
	}
}

func TestNewWasmEnv(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())
	appState.Initialize(0)

	contractAddr := common.Address{0x1}

	code, _ := testdata.Testdata1()

	appState.State.DeployWasmContract(contractAddr, code)

	appState.Commit(nil)

	callAttach := attachments.CreateCallContractAttachment("inc", []byte{0x7})
	payload, _ := callAttach.ToBytes()

	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.CallContractTx,
		Payload:      payload,
		Amount:       big.NewInt(0),
	}
	tx, _ = types.SignTx(tx, key)

	ctx := NewContractContext(tx)

	env := NewWasmEnv(appState, ctx, createHeader(1, 1), "")
	lib.Execute(lib.NewGoAPI(env, &lib.GasMeter{}), code, "inc", [][]byte{[]byte{0x7}}, ctx.ContractAddr(), 100000000)
}

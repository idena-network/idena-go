package appstate

import (
	"fmt"
	"github.com/asaskevich/EventBus"
	"github.com/google/tink/go/subtle/random"
	"github.com/stretchr/testify/require"
	"idena-go/blockchain/types"
	"idena-go/common"
	"testing"
)

func getRandAddr() common.Address {
	addr := common.Address{}
	addr.SetBytes(random.GetRandomBytes(20))
	return addr
}

func TestEvidenceMap_CalculateBitmap(t *testing.T) {
	require := require.New(t)

	bus := EventBus.New()
	em := NewEvidenceMap(bus)

	const candidatesCount = 10000
	const txCandidate = 1
	var addrs []common.Address

	for i := 0; i < candidatesCount; i++ {
		addr := getRandAddr()
		addrs = append(addrs, addr)
		if i == txCandidate {
			em.newTx(&types.Transaction{
				To:      &addr,
				Payload: random.GetRandomBytes(common.HashLength),
				Type:    types.SubmitAnswersHashTx,
			})
		}
	}

	additional := []common.Address{}

	for i := 0; i < candidatesCount; i++ {
		if i%2 == 0 {
			additional = append(additional, addrs[i])
		}
	}
	bytesArray := em.CalculateBitmap(addrs, additional)

	fmt.Printf("size of bitmap for %v candidates is %v bytes\n", candidatesCount, len(bytesArray))

	rmap := common.NewBitmap(candidatesCount)
	rmap.Read(bytesArray)

	require.True(rmap.Contains(txCandidate))

	for i := 0; i < candidatesCount; i++ {
		if i%2 == 0 {
			require.True(rmap.Contains(uint32(i)))
		} else if i != txCandidate {
			require.False(rmap.Contains(uint32(i)))
		}
	}
}

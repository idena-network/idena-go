package appstate

import (
	"bytes"
	"fmt"
	"github.com/google/tink/go/subtle/random"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestEvidenceMap_CalculateBitmap(t *testing.T) {
	require := require.New(t)

	bus := eventbus.New()
	em := NewEvidenceMap(bus)
	now := time.Now().UTC().Add(-29 * time.Second)
	em.SetShortSessionTime(&now, time.Minute*1)

	const candidatesCount = 10000
	const txCandidate = 1
	const delayedKeyAuthor = 10
	var addrs []common.Address

	for i := 0; i < candidatesCount; i++ {
		addr := tests.GetRandAddr()

		if i == txCandidate {
			key, _ := crypto.GenerateKey()
			a := crypto.PubkeyToAddress(key.PublicKey)
			tx := &types.Transaction{
				Payload: random.GetRandomBytes(common.HashLength),
				Type:    types.SubmitAnswersHashTx,
			}
			signed, _ := types.SignTx(tx, key)
			em.newTx(signed)
			addrs = append(addrs, a)
		} else {
			addrs = append(addrs, addr)
		}
	}

	additional := []common.Address{}

	for i := 0; i < candidatesCount; i++ {
		if i%2 == 0 {
			additional = append(additional, addrs[i])
		}
	}

	em.NewFlipsKey(addrs[txCandidate])
	time.Sleep(2 * time.Second)
	em.NewFlipsKey(addrs[delayedKeyAuthor])

	m := em.CalculateBitmap(addrs, additional, func(a common.Address) uint8 {
		if a == addrs[delayedKeyAuthor] || a == addrs[txCandidate] {
			return 1
		}
		return 0
	})

	buf := new(bytes.Buffer)

	m.WriteTo(buf)
	bytesArray := buf.Bytes()
	fmt.Printf("size of bitmap for %v candidates is %v bytes\n", candidatesCount, len(bytesArray))

	rmap := common.NewBitmap(candidatesCount)
	rmap.Read(bytesArray)

	require.True(rmap.Contains(txCandidate))
	require.False(rmap.Contains(delayedKeyAuthor))

	for i := 0; i < candidatesCount; i++ {
		if i%2 == 0 && i != delayedKeyAuthor {
			require.True(rmap.Contains(uint32(i)))
		} else if i != txCandidate {
			require.False(rmap.Contains(uint32(i)))
		}
	}
}

func TestEvidenceMap_CalculateApprovedCandidates(t *testing.T) {
	require := require.New(t)

	bus := eventbus.New()
	em := NewEvidenceMap(bus)

	const candidatesCount = 3
	var candidates []common.Address

	for i := 0; i < candidatesCount; i++ {
		addr := tests.GetRandAddr()
		candidates = append(candidates, addr)
	}
	// first map
	var maps [][]byte
	m := common.NewBitmap(candidatesCount)
	m.Add(0)
	m.Add(1)
	m.Add(2)

	buf := new(bytes.Buffer)
	m.WriteTo(buf)
	maps = append(maps, buf.Bytes())

	// second map
	m = common.NewBitmap(candidatesCount)
	m.Add(0)
	m.Add(2)

	buf = new(bytes.Buffer)
	m.WriteTo(buf)
	maps = append(maps, buf.Bytes())

	// third map
	m = common.NewBitmap(candidatesCount)
	m.Add(0)

	buf = new(bytes.Buffer)
	m.WriteTo(buf)
	maps = append(maps, buf.Bytes())

	// act
	appproved := em.CalculateApprovedCandidates(candidates, maps)

	// assert
	require.Len(appproved, 2)

	require.Contains(appproved, candidates[0])
	require.Contains(appproved, candidates[2])

}

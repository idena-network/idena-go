package pushpull

import (
	"github.com/idena-network/idena-go/common"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestSortedPendingRequests_Add(t *testing.T) {
	sorted := newSortedPendingPushes()
	sorted.Add(pendingRequestTime{time: time.Unix(1, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(3, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(2, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(5, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(0, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(1, 0)})

	require.Equal(t, []int64{0, 1, 1, 2, 3, 5}, toInt64Array(sorted))
}

func TestSortedPendingRequests_Remove(t *testing.T) {
	sorted := newSortedPendingPushes()
	sorted.Add(pendingRequestTime{time: time.Unix(1, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(3, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(2, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(5, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(0, 0)})
	sorted.Remove(0)

	require.Equal(t, []int64{1, 2, 3, 5}, toInt64Array(sorted))

	sorted.Remove(3)

	require.Equal(t, []int64{1, 2, 3}, toInt64Array(sorted))
}

func TestSortedPendingRequests_MoveWithNewTime(t *testing.T) {
	sorted := newSortedPendingPushes()
	sorted.Add(pendingRequestTime{time: time.Unix(0, 0), req: PendingPulls{
		Hash: common.Hash128{0x1},
	}})
	sorted.Add(pendingRequestTime{time: time.Unix(3, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(2, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(5, 0)})
	sorted.Add(pendingRequestTime{time: time.Unix(1, 0)})
	sorted.MoveWithNewTime(0, time.Unix(6, 0))

	require.Equal(t, []int64{1, 2, 3, 5, 6}, toInt64Array(sorted))
	require.Equal(t, common.Hash128{0x1}, sorted.list[4].req.Hash)
}

func toInt64Array(list *sortedPendingPushes) []int64 {
	var times []int64
	for _, r := range list.list {
		times = append(times, r.time.Unix())
	}
	return times
}

func TestDefaultPushTracker_AddPendingRequest(t *testing.T) {
	tracker := NewDefaultPushTracker(time.Millisecond * 300)
	holder := NewDefaultHolder(2, tracker)

	hash1 := common.Hash128{0x1}
	tracker.RegisterPull(hash1)

	pulls := make([]PendingPulls, 2)

	go func() {
		pulls[0] = <-tracker.Requests()
		pulls[1] = <-tracker.Requests()
	}()

	tracker.AddPendingPush("1", hash1)
	tracker.AddPendingPush("2", hash1)
	time.Sleep(time.Millisecond * 350)

	require.Equal(t, peer.ID("1"), pulls[0].Id)
	require.Equal(t, peer.ID(""), pulls[1].Id)

	time.Sleep(time.Millisecond * 350)
	require.Equal(t, peer.ID("2"), pulls[1].Id)

	wg := sync.WaitGroup{}
	wg.Add(1)
	pulls = make([]PendingPulls, 3)
	go func() {
		pulls[0] = <-tracker.Requests()
		pulls[1] = <-tracker.Requests()
		select {
		case pulls[2] = <-tracker.Requests():
		case <-time.After(time.Millisecond * 400):
		}
		wg.Done()
	}()

	tracker.AddPendingPush("3", hash1)
	tracker.AddPendingPush("4", hash1)
	tracker.AddPendingPush("5", hash1)
	tracker.AddPendingPush("6", hash1)

	time.Sleep(time.Millisecond * 700)
	holder.Add(hash1, 1, common.MultiShard, false)
	wg.Wait()

	require.Equal(t, peer.ID("3"), pulls[0].Id)
	require.Equal(t, peer.ID("4"), pulls[1].Id)
	require.Equal(t, peer.ID(""), pulls[2].Id)
	require.Len(t, tracker.Requests(), 0)

	len := 0
	tracker.activePulls.Range(func(key, value interface{}) bool {
		len++
		return true
	})

	require.Equal(t, 0, len)

	require.Len(t, tracker.pendingPushes.list, 0)

	tracker.AddPendingPush("1", common.Hash128{})
	require.Len(t, tracker.pendingPushes.list, 0)
}

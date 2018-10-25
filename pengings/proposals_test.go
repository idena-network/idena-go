package pengings

import (
	"fmt"
	"sync"
	"testing"
)

func TestSyncMap(t *testing.T) {

	var m sync.Map
	key1 := [3]byte{0x1, 0x2, 0x3}
	value, _ := m.LoadOrStore(key1, &sync.Map{})

	innerMap := value.(*sync.Map)
	innerMap.Store(key1, "123")
	innerMap.Store(key1, "456")

	key2 := [3]byte{0x1, 0x2, 0x3}

	m2, ok := m.Load(key2)
	if !ok {
		t.Error("Failed loading by key2")
	}

	innerMap = m2.(*sync.Map)

	m2, ok = innerMap.Load(key2)

	if (m2 != "456") {
		t.Error(fmt.Sprintf("Inner value %v is not equal to %v", m2, "456"))
	}
}

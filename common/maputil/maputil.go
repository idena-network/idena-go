package maputil

import (
	"sync"
)

func IsSyncMapEmpty(sm *sync.Map) bool {
	isEmpty := true
	if sm == nil {
		return isEmpty
	}
	sm.Range(func(key, value interface{}) bool {
		isEmpty = false
		return false
	})
	return isEmpty
}

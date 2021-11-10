// +build !windows

package util

import (
	"testing"
)

func TestManageFdLimit(t *testing.T) {
	t.Log("Testing file descriptor count")
	if changed, newLimit, err := ManageFdLimit(); err != nil {
		t.Errorf("Cannot manage file descriptors - %v", err)
	} else if changed {
		t.Logf("Setup new fd limit - %v", newLimit)
	}
}

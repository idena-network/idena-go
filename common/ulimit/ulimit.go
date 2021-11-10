// adapted code from https://github.com/ipfs/go-ipfs

package util

import (
	"fmt"
	"syscall"
)

var (
	supportsFDManagement = false

	// getlimit returns the soft and hard limits of file descriptors counts
	getLimit func() (uint64, uint64, error)
	// set limit sets the soft and hard limits of file descriptors counts
	setLimit func(uint64, uint64) error
)

var fdLevels = []uint64{1000000, 500000, 100000, 65000, 32000, 16000, 8000, 4000, 2000}

// ManageFdLimit raise the current max file descriptor count to maxFds
func ManageFdLimit() (changed bool, newLimit uint64, err error) {
	if !supportsFDManagement {
		return false, 0, nil
	}

	soft, hard, err := getLimit()
	if err != nil {
		return false, 0, err
	}
loop:
	for _, targetLimit := range fdLevels {

		if targetLimit <= soft {
			break
		}

		// the soft limit is the value that the kernel enforces for the
		// corresponding resource
		// the hard limit acts as a ceiling for the soft limit
		// an unprivileged process may only set it's soft limit to a
		// alue in the range from 0 up to the hard limit
		err = setLimit(targetLimit, targetLimit)
		switch err {
		case nil:
			newLimit = targetLimit
			break loop
		case syscall.EPERM:
			// lower limit if necessary.
			if targetLimit > hard {
				targetLimit = hard
			}

			// the process does not have permission so we should only
			// set the soft value
			err = setLimit(targetLimit, hard)
			if err != nil {
				continue
			}
			newLimit = targetLimit
			break loop
		default:
			err = fmt.Errorf("error setting: ulimit: %s", err)
		}
	}
	return newLimit > 0, newLimit, err
}

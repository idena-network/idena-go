package consensus

import "github.com/deckarep/golang-set"

type ForkDetector interface {
	HasPotentialFork() bool
	GetForkedPeers() mapset.Set
	ClearPotentialForks()
}

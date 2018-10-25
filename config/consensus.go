package config

import "time"

type ConsensusConf struct {
	MaxSteps int
	ProposerTheshold float32
	ThesholdBa float32
	FinalCommitteeConsensusTreshold float32
	WaitBlockDelay time.Duration
	WaitSortitionProofDelay time.Duration
	EstimatedBaVariance time.Duration
	WaitForStepDelay time.Duration
}

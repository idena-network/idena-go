package config

import (
	"math/big"
	"time"
)

type ConsensusConf struct {
	MaxSteps                       uint16
	ProposerTheshold               float64
	ThesholdBa                     float64
	CommitteePercent               float64
	FinalCommitteeConsensusPercent float64
	WaitBlockDelay                 time.Duration
	WaitSortitionProofDelay        time.Duration
	EstimatedBaVariance            time.Duration
	WaitForStepDelay               time.Duration
	Automine                       bool
	BlockReward                    *big.Int
	StakeRewardRate                float32
	FinalCommitteeReward           *big.Int
	FeeBurnRate                    float32
}

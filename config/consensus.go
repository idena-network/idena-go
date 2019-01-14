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

func GetDefaultConsensusConfig(automine bool) *ConsensusConf {
	return &ConsensusConf{
		MaxSteps:                       150,
		CommitteePercent:               0.3,  // 30% of valid nodes will be committee members
		FinalCommitteeConsensusPercent: 0.7,  // 70% of valid nodes will be committee members
		ThesholdBa:                     0.65, // 65% of committee members should vote for block
		ProposerTheshold:               0.5,
		WaitBlockDelay:                 time.Minute,
		WaitSortitionProofDelay:        time.Second * 5,
		EstimatedBaVariance:            time.Second * 5,
		WaitForStepDelay:               time.Second * 20,
		Automine:                       automine,
		BlockReward:                    big.NewInt(0).Mul(big.NewInt(1e+18), big.NewInt(15)),
		StakeRewardRate:                0.2,
		FeeBurnRate:                    0.9,
		FinalCommitteeReward:           big.NewInt(6e+18),
	}
}

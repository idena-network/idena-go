package config

import (
	"math/big"
	"time"
)

type ConsensusConf struct {
	Version                     ConsensusVerson
	StartActivationDate         int64 // unix timestamp
	EndActivationDate           int64 // unix timestamp
	MigrationTimeout            time.Duration
	GenerateGenesisAfterUpgrade bool

	MaxSteps                          uint8
	AgreementThreshold                float64
	CommitteePercent                  float64
	FinalCommitteePercent             float64
	WaitBlockDelay                    time.Duration
	WaitSortitionProofDelay           time.Duration
	EstimatedBaVariance               time.Duration
	WaitForStepDelay                  time.Duration
	Automine                          bool
	BlockReward                       *big.Int
	StakeRewardRate                   float32
	StakeRewardRateForNewbie          float32
	FinalCommitteeReward              *big.Int
	FeeBurnRate                       float32
	SnapshotRange                     uint64
	OfflinePenaltyBlocksCount         int64
	OfflinePenaltyDuration            time.Duration
	SuccessfulValidationRewardPercent float32
	StakingRewardPercent              float32
	CandidateRewardPercent            float32
	FlipRewardPercent                 float32
	ValidInvitationRewardPercent      float32
	ReportsRewardPercent              float32
	FoundationPayoutsPercent          float32
	ZeroWalletPercent                 float32
	FirstInvitationRewardCoef         float32
	SecondInvitationRewardCoef        float32
	ThirdInvitationRewardCoef         float32
	SavedInviteRewardCoef             float32
	SavedInviteWinnerRewardCoef       float32
	FeeSensitivityCoef                float32
	MinBlockDistance                  time.Duration
	MaxCommitteeSize                  int
	StatusSwitchRange                 uint64
	DelegationSwitchRange             uint64
	InvitesPercent                    float32
	MinProposerThreshold              float64
	UpgradeIntervalBeforeValidation   time.Duration
	ReductionOneDelay                 time.Duration
	NewKeyWordsEpoch                  uint16
	EnableUpgrade10                   bool
}

type ConsensusVerson uint16

const (
	ConsensusV9  ConsensusVerson = 9
	ConsensusV10 ConsensusVerson = 10
)

var (
	v9, v10           ConsensusConf
	ConsensusVersions map[ConsensusVerson]*ConsensusConf
)

func init() {
	ConsensusVersions = map[ConsensusVerson]*ConsensusConf{}
	v9 = ConsensusConf{
		Version:                           ConsensusV9,
		MaxSteps:                          150,
		CommitteePercent:                  0.3,  // 30% of valid nodes will be committee members
		FinalCommitteePercent:             0.7,  // 70% of valid nodes will be committee members
		AgreementThreshold:                0.65, // 65% of committee members should vote for block
		WaitBlockDelay:                    time.Second * 40,
		WaitSortitionProofDelay:           time.Second * 5,
		EstimatedBaVariance:               time.Second * 5,
		ReductionOneDelay:                 time.Second * 40,
		WaitForStepDelay:                  time.Second * 20,
		BlockReward:                       big.NewInt(1e+18),
		StakeRewardRate:                   0.2,
		StakeRewardRateForNewbie:          0.8,
		FeeBurnRate:                       0.9,
		FinalCommitteeReward:              big.NewInt(5e+18),
		SnapshotRange:                     1000,
		OfflinePenaltyBlocksCount:         1800,
		SuccessfulValidationRewardPercent: 0.2,
		StakingRewardPercent:              0.18,
		CandidateRewardPercent:            0.02,
		FlipRewardPercent:                 0.35,
		ValidInvitationRewardPercent:      0.18,
		ReportsRewardPercent:              0.15,
		FoundationPayoutsPercent:          0.1,
		ZeroWalletPercent:                 0.02,
		FirstInvitationRewardCoef:         3.0,
		SecondInvitationRewardCoef:        9.0,
		ThirdInvitationRewardCoef:         18.0,
		SavedInviteRewardCoef:             1.0,
		SavedInviteWinnerRewardCoef:       2.0,
		FeeSensitivityCoef:                0.25,
		MinBlockDistance:                  time.Second * 20,
		MaxCommitteeSize:                  100,
		StatusSwitchRange:                 50,
		DelegationSwitchRange:             50,
		InvitesPercent:                    0.5,
		MinProposerThreshold:              0.5,
		UpgradeIntervalBeforeValidation:   time.Hour * 48,
		NewKeyWordsEpoch:                  76,
		OfflinePenaltyDuration:            time.Hour * 8,
	}
	ConsensusVersions[ConsensusV9] = &v9

	v10 = v9
	ApplyConsensusVersion(ConsensusV10, &v10)
	ConsensusVersions[ConsensusV10] = &v10
}

func ApplyConsensusVersion(ver ConsensusVerson, cfg *ConsensusConf) {
	switch ver {
	case ConsensusV10:
		cfg.EnableUpgrade10 = true
		cfg.Version = ConsensusV10
		cfg.StartActivationDate = time.Date(2022, time.December, 23, 8, 0, 0, 0, time.UTC).Unix()
		cfg.EndActivationDate = time.Date(2022, time.December, 30, 0, 0, 0, 0, time.UTC).Unix()
	}
}

func GetDefaultConsensusConfig() *ConsensusConf {
	return &v9
}

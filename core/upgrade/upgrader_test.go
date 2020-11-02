package upgrade

import (
	"github.com/idena-network/idena-go/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpgrader_RevertConfig(t *testing.T) {

	cfg := &config.Config{
		Consensus: config.ConsensusVersions[config.ConsensusV1],
	}

	upgrader := NewUpgrader(cfg, nil, nil)
	prevConfig := upgrader.UpgradeConfigTo(uint32(config.ConsensusV2))
	require.Equal(t, config.ConsensusV2, cfg.Consensus.Version)
	require.Equal(t, config.ConsensusV1, prevConfig.Version)
	require.True(t, cfg.Consensus.HumanCanFailLongSession)
	require.False(t, prevConfig.HumanCanFailLongSession)

	upgrader.RevertConfig(prevConfig)
	require.Equal(t, config.ConsensusV1, cfg.Consensus.Version)
	require.False(t, cfg.Consensus.HumanCanFailLongSession)
}

package upgrade

import (
	"github.com/idena-network/idena-go/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpgrader_RevertConfig(t *testing.T) {

	cfg := &config.Config{
		Consensus: config.ConsensusVersions[config.ConsensusV3],
	}

	upgrader := NewUpgrader(cfg, nil, nil)
	prevConfig := upgrader.UpgradeConfigTo(uint32(config.ConsensusV4))
	require.Equal(t, config.ConsensusV4, cfg.Consensus.Version)
	require.Equal(t, config.ConsensusV3, prevConfig.Version)
	require.True(t, cfg.Consensus.EnablePools)
	require.False(t, prevConfig.EnablePools)

	upgrader.RevertConfig(prevConfig)
	require.Equal(t, config.ConsensusV3, cfg.Consensus.Version)
	require.False(t, cfg.Consensus.EnablePools)
}

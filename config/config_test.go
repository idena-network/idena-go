package config

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestApplyProfileDefaultsToDhtClientRouting(t *testing.T) {
	cfg := getDefaultConfig(t.TempDir())
	ctx := cli.NewContext(cli.NewApp(), flag.NewFlagSet("test", flag.ContinueOnError), nil)

	applyProfile(ctx, cfg)

	require.Equal(t, DefaultIpfsRouting, cfg.IpfsConf.Routing)
}

func TestApplyProfilePreservesConfiguredIpfsRouting(t *testing.T) {
	cfg := getDefaultConfig(t.TempDir())
	cfg.IpfsConf.Routing = IpfsRoutingDht
	ctx := cli.NewContext(cli.NewApp(), flag.NewFlagSet("test", flag.ContinueOnError), nil)

	applyProfile(ctx, cfg)

	require.Equal(t, IpfsRoutingDht, cfg.IpfsConf.Routing)
}

func TestApplyIpfsFlagsOverridesRouting(t *testing.T) {
	cfg := getDefaultConfig(t.TempDir())
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.String(IpfsRoutingFlag.Name, "", "")
	require.NoError(t, set.Set(IpfsRoutingFlag.Name, IpfsRoutingDhtClient))
	ctx := cli.NewContext(cli.NewApp(), set, nil)

	applyIpfsFlags(ctx, cfg)

	require.Equal(t, IpfsRoutingDhtClient, cfg.IpfsConf.Routing)
}

func TestValidateIpfsRoutingAllowsSafeKuboRoutingModes(t *testing.T) {
	for _, routing := range []string{
		"",
		IpfsRoutingAutoClient,
		IpfsRoutingDelegated,
		IpfsRoutingDhtClient,
		IpfsRoutingNone,
	} {
		require.NoError(t, validateIpfsRouting(routing))
	}
}

func TestValidateIpfsRoutingRejectsServerCapableModesByDefault(t *testing.T) {
	for _, routing := range []string{
		IpfsRoutingAuto,
		IpfsRoutingCustom,
		IpfsRoutingDht,
		IpfsRoutingDhtServer,
	} {
		require.ErrorContains(t, validateIpfsRouting(routing), "unsafe or ambiguous")
	}
}

func TestValidateIpfsRoutingAllowsServerCapableModesWithOptIn(t *testing.T) {
	t.Setenv(AllowUnsafeIpfsRoutingEnv, ipfsUnsafeRoutingEnabled)

	for _, routing := range []string{
		IpfsRoutingAuto,
		IpfsRoutingCustom,
		IpfsRoutingDht,
		IpfsRoutingDhtServer,
	} {
		require.NoError(t, validateIpfsRouting(routing))
	}
}

func TestValidateIpfsRoutingRejectsUnknownModes(t *testing.T) {
	require.ErrorContains(t, validateIpfsRouting("bogus"), "invalid IPFS routing mode")
}

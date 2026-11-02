# Idena Go

## Research disclaimer

This is experimental research software. I cannot guarantee its security, correctness, or fitness for any purpose. Use it at your own risk, take responsibility for your decisions, independently verify changes, and stay vigilant.

Go implementation of the Idena network node.

[![Build Idena](https://github.com/ubiubi18/idena-go/actions/workflows/main.yml/badge.svg?branch=master)](https://github.com/ubiubi18/idena-go/actions/workflows/main.yml)

> This is a community-maintained compatibility and security fork. It has no
> published binary releases. Build from a reviewed commit and verify the
> resulting binary before using it with a valuable identity.

The coordinated candidate source, Wasm artifacts, toolchains, and immutable
chain identifiers are recorded in [`compatibility/stack-lock.json`](compatibility/stack-lock.json).
That lock remains a candidate until every listed legacy differential gate has
passed; ordinary unit tests do not by themselves authorize a release.
The evidence format and approval procedure are documented in
[`compatibility/README.md`](compatibility/README.md).

## Fork status

This branch keeps the Idena consensus rules, chain data formats, transaction
encoding, and network identifiers unchanged. It is intended to remain on the
same network as nodes built from `idena-network/idena-go`. The hardening changes
reject malformed, corrupt, or excessively large inputs earlier; they do not
introduce a new chain or consensus version.

### What was updated

- Go and CI were moved to Go `1.26.8`, with native builds tested on Linux,
  macOS, and Windows, including ARM64 where supported.
- Kubo, libp2p, cryptography, database, compression, and supporting Go modules
  were refreshed while preserving the node's existing protocol behavior.
- The Wasm binding is pinned to this fork's checksum-verified static archives.
- RPC API-key files and Unix IPC sockets are restricted to the current user;
  malformed RPC, P2P, protobuf, database, and fast-sync state is handled
  fail-closed instead of panicking or silently advancing state.
- Compressed P2P messages are size-checked before decompression allocation, and
  IPFS routing defaults to client mode unless unsafe routing is explicitly
  enabled.
- CI runs focused race-detector tests, static analysis, cross-platform builds,
  and a repository-specific vulnerability policy before releases.

### Benefits

- Fewer known vulnerable or abandoned dependencies and less native build
  surface.
- Better resistance to malformed network traffic, corrupt local metadata, and
  permissive local credential files.
- Reproducible toolchain and Wasm artifact pins across the desktop and contract
  repositories.

### Risks and tradeoffs

- Dependency upgrades can still change performance, peer discovery, IPFS
  behavior, or resource usage even when consensus serialization is unchanged.
- `GO-2024-3218` remains reachable in the reviewed
  `go-libp2p-kad-dht v0.41.0`; the default DHT client mode reduces exposure but
  does not remove the upstream peer-ID spoofing issue. See
  [`docs/security/vulnerability-policy.md`](docs/security/vulnerability-policy.md).
- Existing Badger-based IPFS repositories are not migrated automatically.
  Back up the full data directory before changing versions or storage profiles.
- Mixing a node binary, Wasm binding, or database copied from another revision
  is unsupported. Keep the exact source and artifact pins together.

## Building the source

Building `idena-go` requires Go 1.27.3 and a C compiler. `idena-go` uses Go modules as a dependency manager.
Once the dependencies are installed, run

```shell
go build
```

Run the validation and vulnerability gates before starting the node:

```shell
go test ./...
./scripts/govulncheck-filter.sh
```

## Running `idena-go`

To connect to the Idena mainnet, run the executable without parameters.
`idena-go` uses Kubo and a private IPFS network to store data.

### CLI parameters

* `--config` Use custom configuration file
* `--datadir` Node data directory (default `datadir`)
* `--rpcaddr` RPC listening address (default `localhost`)
* `--rpcport` RPC listening port (default `9009`)
* `--ipfsport` IPFS P2P port (default `40405`)
* `--ipfsportstatic` Prevent changing IPFS port (default `false`)
* `--ipfsrouting` IPFS routing mode (default `dhtclient`; unsafe or server-capable modes require `IDENA_ALLOW_UNSAFE_IPFS_ROUTING=1`)
* `--ipfsbootnode` Set custom bootstrap node
* `--fast` Use fast sync (default `true`)
* `--verbosity` Log verbosity (default `3` - `Info`)
* `--nodiscovery` Do not discover another nodes (default `false`)
* `--profile=lowpower` Reduce bandwidth usage
* `--apikey` Set RPC API key
* `--logfilesize` Set maximum log file size in KB (default `10240`)



### JSON config


Custom json configuration can be used if `--config=<config file name>` parameter is specified. Use `server` IPFS profile if you run `idena-go` on VPS to prevent local network scanning.

New IPFS repositories use Kubo's supported `flatfs` datastore. Existing repositories keep their configured datastore and are never migrated automatically. Operators with an older Badger v1 repository should plan an export/import migration before Kubo removes Badger support; back up the IPFS data directory before any manual migration.

```json
{
  "DataDir": "datadir",
  "P2P": {
    "MaxInboundPeers": 12,
    "MaxOutboundPeers": 6
  },
  "RPC": {
    "HTTPHost": "localhost",
    "HTTPPort": 9009
  },
  "IpfsConf": {
    "Profile": "server",
    "IpfsPort": 40405,
    "BlockPinThreshold": 0.3,
    "FlipPinThreshold": 0.5
  },
  "Sync": {
    "FastSync": true
  }
}
```

By default, blocks and flips are pinned in local ipfs storage with 30% and 50% probability respectively. If you want to pin (save) locally all blocks and flips, set 1 for `BlockPinThreshold` and `FlipPinThreshold`.

#### Local automine node

##### Config
For debug purposes you can run local automine node with this config.

```json
{
  "IpfsConf": {
    "BootNodes": [],
    "Profile": "server",
    "IpfsPort": 60606
  },
  "RPC": {
    "HTTPHost": "localhost",
    "HTTPPort": 9111
  },
  "GenesisConf": {
    "GodAddress": "0x0000000000000000000000000000000000000000",
    "FirstCeremonyTime": 1700000000
  },
  "Consensus": {
    "Automine": true
  },
  "Validation": {
    "ValidationInterval": 300000000000,
    "FlipLotteryDuration": 10000000000,
    "ShortSessionDuration": 40000000000,
    "LongSessionDuration": 40000000000,
    "AfterLongSessionDuration": 10000000000
  },
  "Network": 3
}
```

##### Description

* `GodAddress` - the address which refers to private key in nodekey file. So, when you are running automine node, you should see log in console `Coinbase address addr=<addr>` with this address. **This address will mine coins if network has 0 valid identities**;
* `FirstCeremonyTime` - timestamp of first validation ceremony;
* `Validation section` - duration of each validation period in nanoseconds;
* `Network` - should be different from 1 or 2, any `uint32` number
* `Ipfs bootnodes` - array of bootstrap nodes in case of running multiple local nodes

For more detailed configuration, see the [config structure](config/config.go).

## Idena Go

Golang implementation of the Idena network node

[![Build Status](https://travis-ci.com/idena-network/idena-go.svg?branch=master)](https://travis-ci.com/idena-network/idena-go)

## Building the source

Building `idena-go` required a Go (version 1.13 or later) and a C compiler. `idena-go` uses Go modules as a dependency manager. 
Once the dependencies are installed, run

```shell
go build
```

## Running `idena-go`

To connect to idena `experimental mainnet` network run executable without parameters. `idena-go` uses `go-ipfs` and private ipfs network to store data.

### CLI parameters

* `--config` Use custom configuration file
* `--datadir` Node data directory (default `datadir`)
* `--rpcaddr` RPC listening address (default `localhost`)
* `--rpcport` RPC listening port (default `9009`)
* `--ipfsport` IPFS P2P port (default `40405`)
* `--ipfsportstatic` Prevent changing IPFS port (default `false`)
* `--ipfsbootnode` Set custom bootstrap node
* `--fast` Use fast sync (default `true`)
* `--verbosity` Log verbosity (default `3` - `Info`)
* `--nodiscovery` Do not discover another nodes (default `false`)
* `--profile=lowpower` Reduce bandwidth usage
* `--apikey` Set RPC API key
* `--logfilesize` Set maximum log file size in KB (default `10240`)



### JSON config


Custom json configuration can be used if `--config=<config file name>` parameter is specified. Use `server` IPFS profile if you run `idena-go` on VPS to prevent local network scanning.
```json
{
  "DataDir": "",
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
    "BootNodes": [],
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

For more detailed configuration please see [config structure](https://github.com/idena-network/idena-go/blob/master/config/config.go#L26)

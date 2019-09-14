## Idena Go

Golang implementation of the Idena network node

[![Build Status](https://travis-ci.com/idena-network/idena-go.svg?branch=master)](https://travis-ci.com/idena-network/idena-go)

## Building the source

Building `idena-go` required a Go (version 1.12 or later) and a C compiler. `idena-go` uses Go modules as a dependency manager. 
Once the dependencies are installed, run

```shell
go build
```

## Running `idena-go`

To connect to idena network `testnet` run executable without parameters. `idena-go` uses `go-ipfs` and private ipfs network to store data.

### CLI parameters

* `--config` Use custom configuration file
* `--datadir` Node data directory (default `datadir`)
* `--port` Node tcp port (default `40404`)
* `--rpcaddr` RPC listening address (default `localhost`)
* `--rpcport` RPC listening port (default `9009`)
* `--ipfsport` IPFS port (default `40403`)
* `--bootnode` Set custom bootstrap node
* `--fast` Use fast sync (default `true`)
* `--verbosity` Log verbosity (default `3` - `Info`)
* `--nodiscovery` Do not discover another nodes (default `false`)

### JSON config

Custom json configuration can be used if `--config` parameter is specified

```json
{
  "DataDir": "",
  "P2P": {
    "ListenAddr": ":40404",
    "BootstrapNodes": [],
    "NoDiscovery": false
  },
  "RPC": {
    "HTTPHost": "localhost",
    "HTTPPort": 9009
  },
  "IpfsConf": {
    "DataDir": "",
    "IpfsPort": 40403,
    "BootNodes": []
  },
  "Sync": {
    "FastSync": true
  }
}
```
### Running `idena-go` on remote host

To connect to the `idena-go` running on the remote host use `--rpcaddr=0.0.0.0` parameter

# Lorenzo Btcstaking Submitter

The submitter program is used to submit btc staking transaction proof to Lorenzo node for stBTC minting.

## Requirements
- Go 1.21+

## Building
In order to build the program, execute the following command:

```shell
make build
```
You will get the binary file named `lrz-btcstaking-submitter` in the `build` directory.

## Run locally

- copy sample config and update with your values
- create database tables by ``` ./db/schema.sql```
- insert a row to database config table 
```
name: submitter/btc-sync-point
value: $(pick a block height start from)
```

```sh       
## replace ./sample-config.yml with your config file
./build/lrz-btcstaking-submitter --config ./sample-config.yml
```
# run blockscout refresher
```sh
 ./build/lrz-btcstaking-submitter refresh --blockscout-api $(blocksoutApiUrl) --lorenzo-app-api $(lorenzoAppApiUrl) --start-height 19999
```

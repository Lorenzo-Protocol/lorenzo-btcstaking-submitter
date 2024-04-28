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
```sh       
## replace ./sample-config.yml with your config file
./build/lrz-btcstaking-submitter -config ./sample-config.yml
```
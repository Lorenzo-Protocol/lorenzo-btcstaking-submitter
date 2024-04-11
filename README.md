# lorenzo-submit-btcstaking

lorenzo-submit-btcstaking program for submitting btc staking transaction proof to Lorenzo node for creating stBtC.

## Requirements
- Go 1.21

## Building
In order to build the lorenzo-submit-btcstaking, execute the following command:
```shell
make build
```
You will get the binary file named `lrz-submit-btcstaking` in the `build` directory.

## Run locally
```sh       
## replace ./sample-config.yml with your config file
./build/lrz-submit-btcstaking -config ./sample-config.yml
```
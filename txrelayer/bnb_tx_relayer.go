package txrelayer

import "github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/evmclient"

type BnbTxRelayer struct {
	evmClient   *evmclient.Client
	delayBlocks uint64
}

func NewBnbTxRelayer(rpcUrl string, delayBlocks uint64) (*BnbTxRelayer, error) {
	evmClient, err := evmclient.New(rpcUrl)
	if err != nil {
		return nil, err
	}

	return &BnbTxRelayer{
		evmClient:   evmClient,
		delayBlocks: delayBlocks,
	}, nil
}

func Start() {

}

func ScanLoop() {

}

func SubmitLoop() {

}

func Stop() {

}

package evmclient

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type Client struct {
	client *ethclient.Client
	// Supplement to ethclient
	rpcClient *rpc.Client
}

func New(rpcUrl string) (*Client, error) {
	client, err := ethclient.Dial(rpcUrl)
	if err != nil {
		return nil, err
	}

	rpcClient, err := rpc.DialContext(context.Background(), rpcUrl)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:    client,
		rpcClient: rpcClient,
	}, nil
}

func (c *Client) ReceiptsByBlockHash(hash common.Hash) ([]*types.Receipt, error) {
	var r []*types.Receipt
	err := c.rpcClient.CallContext(context.Background(), &r, "eth_getBlockReceipts", hash.Hex())
	if err == nil && r == nil {
		return nil, ethereum.NotFound
	}

	return r, err
}

func (c *Client) ReceiptsByBlockNumber(number uint64) ([]*types.Receipt, error) {
	var r []*types.Receipt
	err := c.rpcClient.CallContext(context.Background(), &r, "eth_getBlockReceipts", hexutil.Uint64(number).String())
	if err == nil && r == nil {
		return nil, ethereum.NotFound
	}

	return r, err
}

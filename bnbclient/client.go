package bnbclient

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/v2/bnbclient/bnbtypes"
)

const (
	BlockReceiptsCacheSize = 100
	BlockHeaderCacheSize   = 100
)

type Client struct {
	ethClient *ethclient.Client
	// Supplement to ethclient
	rpcClient *rpc.Client

	blockReceiptsCache *lru.Cache[common.Hash, []*types.Receipt]
	blockHeaderCache   *lru.Cache[common.Hash, *bnbtypes.Header]
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

	blockReceiptsCache, err := lru.New[common.Hash, []*types.Receipt](BlockReceiptsCacheSize)
	if err != nil {
		return nil, err
	}
	blockHeaderCache, err := lru.New[common.Hash, *bnbtypes.Header](BlockHeaderCacheSize)
	if err != nil {
		return nil, err
	}

	return &Client{
		ethClient:          client,
		rpcClient:          rpcClient,
		blockReceiptsCache: blockReceiptsCache,
		blockHeaderCache:   blockHeaderCache,
	}, nil
}

func (c *Client) ReceiptsByBlockHash(hash common.Hash) ([]*types.Receipt, error) {
	if receipts, ok := c.blockReceiptsCache.Get(hash); ok {
		return receipts, nil
	}

	var receipts []*types.Receipt
	err := c.rpcClient.CallContext(context.Background(), &receipts, "eth_getBlockReceipts", hash.Hex())
	if err == nil && receipts == nil {
		return nil, ethereum.NotFound
	}

	c.blockReceiptsCache.Add(hash, receipts)
	return receipts, err
}

func (c *Client) ReceiptsByBlockNumber(number uint64) ([]*types.Receipt, error) {
	var r []*types.Receipt
	err := c.rpcClient.CallContext(context.Background(), &r, "eth_getBlockReceipts", hexutil.Uint64(number).String())
	if err == nil && r == nil {
		return nil, ethereum.NotFound
	}

	return r, err
}

func (c *Client) HeaderByNumber(number uint64) (*bnbtypes.Header, error) {
	var header *bnbtypes.Header

	err := c.rpcClient.CallContext(context.Background(), &header, "eth_getBlockByNumber", hexutil.EncodeUint64(number), false)
	if err == nil && header == nil {
		err = ethereum.NotFound
	}

	return header, err
}

func (c *Client) HeaderByHash(hash common.Hash) (*bnbtypes.Header, error) {
	if header, ok := c.blockHeaderCache.Get(hash); ok {
		return header, nil
	}

	var header *bnbtypes.Header
	err := c.rpcClient.CallContext(context.Background(), &header, "eth_getBlockByHash", hash, false)
	if err == nil && header == nil {
		err = ethereum.NotFound
	}

	c.blockHeaderCache.Add(header.Hash(), header)
	return header, err
}

func (c *Client) BlockNumber() (uint64, error) {
	return c.ethClient.BlockNumber(context.Background())
}

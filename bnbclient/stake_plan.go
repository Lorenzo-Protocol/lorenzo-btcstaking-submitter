package bnbclient

import (
	"context"
	"errors"
	"math/big"
	"strings"

	bnblightclienttypes "github.com/Lorenzo-Protocol/lorenzo/v3/x/bnblightclient/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type ReceiptWithProof struct {
	Receipt *types.Receipt
	Proof   *bnblightclienttypes.Proof
}

var (
	StakeBTC2JoinStakePlanEventTopic = crypto.Keccak256Hash([]byte("StakeBTC2JoinStakePlan(uint256,uint256,address,address,uint256,uint256)"))
)

type StakeBTC2JoinStakePlanEvent struct {
	Txhash    common.Hash
	BlockHash common.Hash

	StakeIndex         *big.Int
	PlanId             *big.Int
	User               common.Address
	BtcContractAddress common.Address `abi:"btcContractAddress"`
	StakeAmount        *big.Int       `abi:"stakeAmount"`
	StBTCAmount        *big.Int       `abi:"stBTCAmount"`
}

func (c *Client) GetStakeBTC2JoinStakePlanEventByRangeBlock(planStakeHubAddress common.Address, start, end uint64) ([]*StakeBTC2JoinStakePlanEvent, error) {
	eventABI := `[
        {
            "anonymous": false,
            "inputs": [
                {"indexed": true, "name": "stakeIndex", "type": "uint256"},
                {"indexed": true, "name": "planId", "type": "uint256"},
                {"indexed": true, "name": "user", "type": "address"},
                {"indexed": false, "name": "btcContractAddress", "type": "address"},
                {"indexed": false, "name": "stakeAmount", "type": "uint256"},
                {"indexed": false, "name": "stBTCAmount", "type": "uint256"}
            ],
            "name": "StakeBTC2JoinStakePlan",
            "type": "event"
        }
    ]`
	parsedABI, err := abi.JSON(strings.NewReader(eventABI))
	if err != nil {
		return nil, err
	}

	logs, err := c.getStakeBTC2JoinStakePlanEvents(planStakeHubAddress, start, end)
	if err != nil {
		return nil, err
	}

	var events []*StakeBTC2JoinStakePlanEvent
	for _, log := range logs {
		event := &StakeBTC2JoinStakePlanEvent{}
		err := parsedABI.UnpackIntoInterface(event, "StakeBTC2JoinStakePlan", log.Data)
		if err != nil {
			return nil, err
		}
		event.StakeIndex = log.Topics[1].Big()
		event.PlanId = log.Topics[2].Big()
		event.User = common.BytesToAddress(log.Topics[3].Bytes())

		//init txhash and blockHash
		event.Txhash = log.TxHash
		event.BlockHash = log.BlockHash

		events = append(events, event)
	}

	return events, nil
}

// GetStakeBTC2JoinStakePlanReceiptsWithProof get all receipts of StakeBTC2JoinStakePlan event
func (c *Client) GetStakeBTC2JoinStakePlanReceiptsWithProof(planStakeHubAddress common.Address, start, end uint64) ([]*ReceiptWithProof, error) {
	var stakeBTC2JoinStakePlanReceiptWithProofList []*ReceiptWithProof

	stakeBTC2JoinStakePlanEvents, err := c.GetStakeBTC2JoinStakePlanEventByRangeBlock(planStakeHubAddress, start, end)
	if err != nil {
		return nil, err
	}

	txhashSet := map[common.Hash]common.Hash{}
	for _, event := range stakeBTC2JoinStakePlanEvents {
		txhashSet[event.Txhash] = event.BlockHash
	}
	for txhash, blockHash := range txhashSet {
		receipts, err := c.ReceiptsByBlockHash(blockHash)
		if err != nil {
			return nil, err
		}

		var receipt *types.Receipt
		for _, r := range receipts {
			if r.TxHash == txhash {
				receipt = r
				break
			}
		}
		if receipt == nil {
			return nil, errors.New("receipt not found in his block, it's impossible, maybe something wrong")
		}
		blockHeader, err := c.HeaderByHash(blockHash)
		if err != nil {
			return nil, err
		}

		//generate receipt proof
		proof, err := bnblightclienttypes.GenReceiptProof(uint64(receipt.TransactionIndex), blockHeader.ReceiptHash, receipts)
		if err != nil {
			return nil, err
		}

		receiptWithProof := &ReceiptWithProof{
			Receipt: receipt,
			Proof:   proof,
		}
		stakeBTC2JoinStakePlanReceiptWithProofList = append(stakeBTC2JoinStakePlanReceiptWithProofList, receiptWithProof)
	}

	return stakeBTC2JoinStakePlanReceiptWithProofList, nil
}

func (c *Client) getStakeBTC2JoinStakePlanEvents(planStakeHubAddress common.Address, start, end uint64) ([]types.Log, error) {
	query := ethereum.FilterQuery{
		BlockHash: nil,
		FromBlock: big.NewInt(0).SetUint64(start),
		ToBlock:   big.NewInt(0).SetUint64(end),
		Addresses: []common.Address{planStakeHubAddress},
		Topics:    [][]common.Hash{{StakeBTC2JoinStakePlanEventTopic}},
	}
	logs, err := c.ethClient.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, err
	}

	return logs, nil
}

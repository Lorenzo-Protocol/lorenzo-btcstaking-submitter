package evmclient

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	StakeBTC2JoinStakePlanEventTopic = crypto.Keccak256Hash([]byte("StakeBTC2JoinStakePlan(uint256,uint256,address,address,uint256,uint256)"))
)

type StakeBTC2JoinStakePlanEvent struct {
	StakeIndex         *big.Int       `abi:"stakeIndex"`
	PlanId             *big.Int       `abi:"planId"`
	User               common.Address `abi:"user"`
	BtcContractAddress common.Address `abi:"btcContractAddress"`
	StakeAmount        *big.Int       `abi:"stakeAmount"`
	StBTCAmount        *big.Int       `abi:"stBTCAmount"`
}

func (c *Client) GetStakeBTC2JoinStakePlanEventByRangeBlock(planStakeHubAddress common.Address, start, end uint64) ([]*StakeBTC2JoinStakePlanEvent, error) {
	eventABI := `[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"uint256","name":"stakeIndex","type":"uint256"},{"indexed":true,"internalType":"uint256","name":"planId","type":"uint256"},{"indexed":false,"internalType":"address","name":"user","type":"address"},{"indexed":false,"internalType":"address","name":"btcContractAddress","type":"address"},{"indexed":false,"internalType":"uint256","name":"stakeAmount","type":"uint256"},{"indexed":false,"internalType":"uint256","name":"stBTCAmount","type":"uint256"}],"name":"StakeBTC2JoinStakePlan","type":"event"}]`
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

		events = append(events, event)
	}

	return events, nil
}

// GetStakeBTC2JoinStakePlanReceipts get all receipts of StakeBTC2JoinStakePlan event
func (c *Client) GetStakeBTC2JoinStakePlanReceipts(planStakeHubAddress common.Address, start, end uint64) ([]*types.Receipt, error) {
	var stakeBTC2JoinStakePlanReceipts []*types.Receipt

	for number := start; number <= end; number++ {
		receipts, err := c.ReceiptsByBlockNumber(number)
		if err != nil {
			return nil, err
		}

		for _, receipt := range receipts {
			for _, log := range receipt.Logs {
				// if contract address is not planStakeHubAddress, skip
				if log.Address != planStakeHubAddress {
					continue
				}
				if log.Topics[0] == StakeBTC2JoinStakePlanEventTopic {
					// append receipt to stakeBTC2JoinStakePlanReceipts if it contains StakeBTC2JoinStakePlan event
					stakeBTC2JoinStakePlanReceipts = append(stakeBTC2JoinStakePlanReceipts, receipt)
					break
				}
			}
		}
	}

	return stakeBTC2JoinStakePlanReceipts, nil
}

func (c *Client) getStakeBTC2JoinStakePlanEvents(planStakeHubAddress common.Address, start, end uint64) ([]types.Log, error) {
	eventSignature := []byte("StakeBTC2JoinStakePlan(uint256,uint256,address,address,uint256,uint256)")
	eventTopic := crypto.Keccak256Hash(eventSignature)
	query := ethereum.FilterQuery{
		BlockHash: nil,
		FromBlock: big.NewInt(0).SetUint64(start),
		ToBlock:   big.NewInt(0).SetUint64(end),
		Addresses: []common.Address{planStakeHubAddress},
		Topics:    [][]common.Hash{{eventTopic}},
	}
	logs, err := c.client.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, err
	}

	return logs, nil
}

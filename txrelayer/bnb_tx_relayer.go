package txrelayer

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v2/client"
	"github.com/Lorenzo-Protocol/lorenzo/v2/x/btcstaking/types"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/bnbclient"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
)

const (
	BatchPlanStakeBlockSizeFetch = uint64(1000)
	DefaultDelayBlocks           = uint64(15)
)

var (
	LorenzoBNBDuplicateErrorMessage = "duplicated receipt"
)

type BNBTxRelayer struct {
	chainName     string
	logger        *zap.SugaredLogger
	bnbClient     *bnbclient.Client
	lorenzoClient *lrzclient.Client
	delayBlocks   uint64

	planStakeHubAddress common.Address

	quit      chan struct{}
	wg        sync.WaitGroup
	submitter string
}

func NewBnbTxRelayer(cfg config.BNBTxRelayerConfig, lorenzoClient *lrzclient.Client, logger *zap.SugaredLogger) (*BNBTxRelayer, error) {
	bnbClient, err := bnbclient.New(cfg.RpcUrl)
	if err != nil {
		return nil, err
	}

	txRelayer := &BNBTxRelayer{
		chainName:     "bnb",
		bnbClient:     bnbClient,
		lorenzoClient: lorenzoClient,
		delayBlocks:   DefaultDelayBlocks,

		planStakeHubAddress: common.HexToAddress(cfg.PlanStakeHubAddress),

		quit:      make(chan struct{}),
		submitter: lorenzoClient.MustGetAddr(),
	}
	txRelayer.logger = logger.Named(txRelayer.chainName)

	return txRelayer, nil
}

func (r *BNBTxRelayer) ChainName() string {
	return r.chainName
}

func (r *BNBTxRelayer) Start() {
	r.wg.Add(1)
	go func() {
		r.mainLoop()
	}()
}

func (r *BNBTxRelayer) mainLoop() {
	networkErrorWaitTime := time.Millisecond * 500
	blockWaitTime := time.Second

	for {
		select {
		case <-r.quit:
			return
		default:
		}

		syncPoint, err := db.GetBNBSyncPoint()
		if err != nil {
			r.logger.Warnf("failed to get sync point: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}
		bnbChainTipNumber, err := r.bnbClient.BlockNumber()
		if err != nil {
			r.logger.Warnf("failed to get BNB chain tip number: %v", err)
		}
		if syncPoint+r.delayBlocks >= bnbChainTipNumber {
			r.logger.Infof("Sync point is %d, BNB chain tip is %d, wait for %d blocks",
				syncPoint, bnbChainTipNumber, syncPoint+r.delayBlocks-bnbChainTipNumber)
			time.Sleep(blockWaitTime)
			continue
		}

		start := syncPoint + 1
		end := bnbChainTipNumber - r.delayBlocks
		if end-start+1 > BatchPlanStakeBlockSizeFetch {
			end = start + BatchPlanStakeBlockSizeFetch - 1
		}

		receiptWithProofList, err := r.bnbClient.GetStakeBTC2JoinStakePlanReceiptsWithProof(r.planStakeHubAddress, start, end)
		if err != nil {
			r.logger.Warnf("failed to get receipts with proof: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}

		// every receipt is valid, so it must be success, otherwise try again
		if err := r.submit(receiptWithProofList); err != nil {
			r.logger.Warnf("failed to submit receipts with proof: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}

		if err := db.SetBNBSyncPoint(end); err != nil {
			r.logger.Warnf("failed to set sync point: %v", err)
		}
	}
}

func (r *BNBTxRelayer) submit(receiptWithProofList []*bnbclient.ReceiptWithProof) error {
	if len(receiptWithProofList) == 0 {
		return nil
	}
	networkErrorWaitTime := time.Millisecond * 500

	i := 0
	for i < len(receiptWithProofList) {
		receiptWithProof := receiptWithProofList[i]
		msg := &types.MsgCreateBTCStakingFromBNB{
			Signer:  r.submitter,
			Receipt: nil,
			Proof:   nil,
		}

		// encode receipt and proof
		var receiptBuf bytes.Buffer
		if err := json.NewEncoder(&receiptBuf).Encode(receiptWithProof.Receipt); err != nil {
			return err
		}
		var proofBuf bytes.Buffer
		if err := json.NewEncoder(&proofBuf).Encode(receiptWithProof.Proof); err != nil {
			return err
		}
		// set receipt and proof
		msg.Receipt = receiptBuf.Bytes()
		msg.Proof = proofBuf.Bytes()

		_, err := r.lorenzoClient.BNBCreateBTCStakingWithProof(context.Background(), msg)
		if err != nil {
			switch {
			// TODO: check error message, decide to retry or not
			case strings.Contains(err.Error(), LorenzoBNBDuplicateErrorMessage):
				i++
			default:
				return err
			}

			//try handle this transaction again
			time.Sleep(networkErrorWaitTime)
		} else {
			i++
		}
	}

	return nil
}

func (r *BNBTxRelayer) Stop() {
	close(r.quit)
}

func (r *BNBTxRelayer) WaitForShutdown() {
	r.wg.Wait()
}

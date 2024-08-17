package txrelayer

import (
	"context"
	"strings"
	"sync"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v3/client"
	"github.com/Lorenzo-Protocol/lorenzo/v3/x/btcstaking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/bnbclient"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
)

const (
	BatchPlanStakeBlockSizeFetch = uint64(1000)
	DefaultDelayBlocks           = uint64(15)
)

const (
	LorenzoBNBDuplicateErrorMessage = "already exists"
)

type BNBTxRelayer struct {
	chainName     string
	logger        *zap.SugaredLogger
	bnbClient     *bnbclient.Client
	lorenzoClient *lrzclient.Client
	delayBlocks   uint64

	repository          db.IBNBRepository
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

	repository, err := db.NewBNBRepository()
	if err != nil {
		return nil, err
	}

	// check if sync point is set, if not set it to start block height
	if height, err := repository.GetSyncPoint(); err != nil {
		return nil, err
	} else if height == 0 {
		if err := repository.UpdateSyncPoint(cfg.StartBlockHeight); err != nil {
			return nil, err
		}
	}

	txRelayer := &BNBTxRelayer{
		chainName:     "BNB",
		bnbClient:     bnbClient,
		lorenzoClient: lorenzoClient,
		delayBlocks:   DefaultDelayBlocks,

		repository: repository,

		planStakeHubAddress: common.HexToAddress(cfg.PlanStakeHubAddress),

		quit:      make(chan struct{}),
		submitter: lorenzoClient.MustGetAddr(),
	}
	txRelayer.logger = logger.Named(txRelayer.chainName)

	txRelayer.logger.Infof("new Relayer on BNB Smart Chain, confirmations: %d, submitter: %s",
		txRelayer.delayBlocks+1, txRelayer.submitter)
	return txRelayer, nil
}

func (r *BNBTxRelayer) ChainName() string {
	return r.chainName
}

func (r *BNBTxRelayer) Start() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.mainLoop()
	}()
}

func (r *BNBTxRelayer) mainLoop() {
	networkErrorWaitTime := time.Millisecond * 500
	blockWaitTime := time.Second

	for {
		select {
		case <-r.quit:
			r.logger.Debug("BNB mainloop quit")
			return
		default:
		}

		syncPoint, err := r.repository.GetSyncPoint()
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

		r.logger.Debugf("start: %d, end: %d", start, end)
		receiptWithProofList, err := r.bnbClient.GetStakeBTC2JoinStakePlanReceiptsWithProof(r.planStakeHubAddress, start, end)
		if err != nil {
			r.logger.Warnf("failed to get receipts with proof: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}
		r.logger.Debugf("receiptWithProofList: %d", len(receiptWithProofList))

		// every receipt is valid, so it must be success, otherwise try again
		if err := r.submit(receiptWithProofList); err != nil {
			r.logger.Warnf("failed to submit receipts with proof: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}

		if err := r.repository.UpdateSyncPoint(end); err != nil {
			r.logger.Warnf("failed to set sync point: %v", err)
		}
		r.logger.Infof("sync point updated to %d", end)
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
		msg := &types.MsgCreateBTCBStaking{
			Signer:  r.submitter,
			Number:  receiptWithProof.Receipt.BlockNumber.Uint64(),
			Receipt: nil,
			Proof:   nil,
		}
		receiptRaw, err := rlp.EncodeToBytes(receiptWithProof.Receipt)
		if err != nil {
			return err
		}
		proofRaw, err := rlp.EncodeToBytes(receiptWithProof.Proof)
		if err != nil {
			return err
		}
		msg.Receipt = receiptRaw
		msg.Proof = proofRaw
		r.logger.Debugf("BlockNumber: %d\n", msg.Number)
		r.logger.Debugf("Receipt: %x\n", msg.Receipt)
		r.logger.Debugf("Proof: %x\n", msg.Proof)
		r.logger.Debug("=====================================")

		_, err = r.lorenzoClient.CreateBTCBStaking(context.Background(), msg)
		if err != nil {
			switch {
			case isBNBStakingDuplicate(err):
				i++
			case isBNBStakingRetryError(err):
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

func isBNBStakingRetryError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), LorenzoTimeoutErrorMessage) ||
		strings.Contains(err.Error(), PostFailedMessage) ||
		strings.Contains(err.Error(), SequenceMismatch))
}

func isBNBStakingDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), LorenzoBtcStakingDuplicateTxErrorMessage)
}

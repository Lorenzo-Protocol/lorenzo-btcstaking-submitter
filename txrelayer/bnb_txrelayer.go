package txrelayer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v3/client"
	"github.com/Lorenzo-Protocol/lorenzo/v3/x/btcstaking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	if err := cfg.Validate(); err != nil {
		return nil, errors.New("invalid BNB Tx-relayer config")
	}

	chainName := "bnb"
	bnbClient, err := bnbclient.New(cfg.RpcUrl)
	if err != nil {
		return nil, err
	}

	repository, err := db.NewBNBRepository(chainName)
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
		chainName:     chainName,
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
		r.scanLoop()
	}()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.submitLoop()
	}()
}

func (r *BNBTxRelayer) scanLoop() {
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

		txs, err := r.ReceiptWithProofList2WrappedBTCDepositTxList(receiptWithProofList)
		if err != nil {
			r.logger.Warnf("failed to convert receiptWithProofList to WrappedBTCDepositTxList: %v", err)
			continue
		}

		if err := r.repository.InsertWrappedBTCDepositTxs(txs); err != nil {
			r.logger.Errorf("failed to insert wrapped btc deposit txs: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}

		if err := r.repository.UpdateSyncPoint(end); err != nil {
			r.logger.Warnf("failed to set sync point: %v", err)
		}
		r.logger.Infof("sync point updated to %d", end)
	}
}

func (r *BNBTxRelayer) submitLoop() {
	networkErrorWaitTime := time.Millisecond * 500
	blockWaitTime := time.Second

	for {
		select {
		case <-r.quit:
			return
		default:
		}
		lorenzoBNBTip, err := r.lorenzoClient.BNBLatestHeader()
		if err != nil {
			r.logger.Warnf("failed to get latest BNB header: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}

		txs, err := r.repository.GetUnhandledWrappedBTCDepositTxs(lorenzoBNBTip.Number)
		if err != nil {
			r.logger.Warnf("failed to get unhandled wrapped btc deposit txs: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}

		if len(txs) == 0 {
			r.logger.Debugf("no unhandled wrapped btc deposit txs")
			time.Sleep(blockWaitTime)
			continue
		}

		if err := r.submit(txs); err != nil {
			r.logger.Warnf("failed to submit txs: %v", err)
			time.Sleep(networkErrorWaitTime)
			continue
		}
	}
}

func (r *BNBTxRelayer) submit(txs []*db.WrappedBTCDepositTx) error {
	if len(txs) == 0 {
		return nil
	}
	networkErrorWaitTime := time.Millisecond * 500

	i := 0
	for i < len(txs) {
		select {
		case <-r.quit:
			return nil
		default:
		}

		tx := txs[i]
		receiptRaw, err := hexutil.Decode(tx.Receipt)
		if err != nil {
			r.logger.Warnf("failed to decode receipt, txid:%s error:%v", tx.Txid, err)
			r.markDepositTxInvalid(tx.Txid)
			i++ // skip this tx
			continue
		}
		proofRaw, err := hexutil.Decode(tx.Proof)
		if err != nil {
			r.markDepositTxInvalid(tx.Txid)
			i++ // skip this tx
			return err
		}

		msg := &types.MsgCreateBTCBStaking{
			Signer:  r.submitter,
			Number:  tx.Height,
			Receipt: receiptRaw,
			Proof:   proofRaw,
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
				// duplicate tx, mark as success, not really error
				err = nil
				r.markDepositTxSuccess(tx.Txid)
				i++
			case isBNBStakingRetryError(err):
				//retry again
			default:
				r.markDepositTxInvalid(tx.Txid)
				i++
			}

			if err != nil {
				r.logger.Warnf("failed to submit tx: %v", err)
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

func (r *BNBTxRelayer) markDepositTxInvalid(txid string) {
	if err := r.repository.MarkInvalid(txid); err != nil {
		r.logger.Warnf("failed to mark invalid, txid:%s, error:%v", txid, err)
	}
}

func (r *BNBTxRelayer) markDepositTxSuccess(txid string) {
	if err := r.repository.MarkSuccess(txid); err != nil {
		r.logger.Warnf("failed to mark success, txid:%s, error:%v", txid, err)
	}
}

func (r *BNBTxRelayer) ReceiptWithProofList2WrappedBTCDepositTxList(receiptWithProofList []*bnbclient.ReceiptWithProof) ([]*db.WrappedBTCDepositTx, error) {
	wrappedBTCDepositTxList := make([]*db.WrappedBTCDepositTx, 0, len(receiptWithProofList))
	for _, receiptWithProof := range receiptWithProofList {
		receiptRaw, err := rlp.EncodeToBytes(receiptWithProof.Receipt)
		if err != nil {
			return nil, err
		}
		proofRaw, err := rlp.EncodeToBytes(receiptWithProof.Proof)
		if err != nil {
			return nil, err
		}
		wrappedBTCDepositTx := &db.WrappedBTCDepositTx{
			Chain:     r.chainName,
			Txid:      receiptWithProof.Receipt.TxHash.Hex(),
			Height:    receiptWithProof.Receipt.BlockNumber.Uint64(),
			BlockHash: receiptWithProof.Receipt.BlockHash.Hex(),
			BlockTime: time.Unix(int64(receiptWithProof.BlockTime), 0),
			Receipt:   hexutil.Encode(receiptRaw),
			Proof:     hexutil.Encode(proofRaw),
		}
		wrappedBTCDepositTxList = append(wrappedBTCDepositTxList, wrappedBTCDepositTx)
	}
	return wrappedBTCDepositTxList, nil
}

func isBNBStakingRetryError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), LorenzoTimeoutErrorMessage) ||
		strings.Contains(err.Error(), PostFailedMessage) ||
		strings.Contains(err.Error(), SequenceMismatch))
}

func isBNBStakingDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), BNBBTCBStakingDuplication)
}

package txrelayer

import (
	"context"
	"strings"
	"sync"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v2/client"
	lrztypes "github.com/Lorenzo-Protocol/lorenzo/v2/types"
	"github.com/Lorenzo-Protocol/lorenzo/v2/x/btcstaking/keeper"
	"github.com/Lorenzo-Protocol/lorenzo/v2/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/btc"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
)

var (
	LorenzoBtcStakingNotConfirmedErrorMessage = "not k-deep"
	LorenzoBtcStakingDuplicateTxErrorMessage  = "duplicate btc transaction"
	LorenzoTimeoutErrorMessage                = "context deadline exceeded"
	LorenzoBtcHeaderNotFoundErrorMessage      = "btc block header not found"
	PostFailedMessage                         = "post failed"
)

type TxRelayer struct {
	logger                       *zap.SugaredLogger
	delayBlocks                  uint64
	lorenzoBtcConfirmationsDepth uint64 // lorenzo btc staking tx confirmation depth

	btcParam  *chaincfg.Params
	submitter string

	btcQuery      *btc.BTCQuery
	lorenzoClient *lrzclient.Client
	db            db.IDB

	// btc deposit tx receiver
	receivers []*types.Receiver

	wg sync.WaitGroup
}

func NewTxRelayer(database db.IDB, logger *zap.SugaredLogger, conf *config.TxRelayerConfig, btcQuery *btc.BTCQuery, lorenzoClient *lrzclient.Client) (*TxRelayer, error) {
	// get btc staking params to get btc deposit receivers
	btcStakingParams, err := lorenzoClient.QueryBTCStakingParams()
	if err != nil {
		return nil, err
	}

	_, err = database.GetSyncPoint()
	if err != nil {
		return nil, err
	}
	btcParam := btc.GetBTCParams(conf.NetParams)

	txRelayer := &TxRelayer{
		logger:                       logger,
		delayBlocks:                  conf.ConfirmationDepth,
		lorenzoBtcConfirmationsDepth: uint64(btcStakingParams.Params.BtcConfirmationsDepth),
		btcQuery:                     btcQuery,
		lorenzoClient:                lorenzoClient,
		db:                           database,
		btcParam:                     btcParam,
		submitter:                    lorenzoClient.MustGetAddr(),

		wg: sync.WaitGroup{},
	}
	txRelayer.updateBtcReceiverList(btcStakingParams.Params.Receivers)

	logger.Infof("new txRelayer on BTC network: %s, confirmation: %d, submitter: %s",
		conf.NetParams, conf.ConfirmationDepth, txRelayer.submitter)
	return txRelayer, nil
}

func (r *TxRelayer) Start() error {
	r.wg.Add(2)
	go func() {
		defer r.wg.Done()
		r.scanBlockLoop()
	}()
	go func() {
		defer r.wg.Done()
		go r.submitLoop()
	}()

	r.wg.Wait()
	return nil
}

// TODO: update btc receiver list when Lorenzo btc staking receiver list update
func (r *TxRelayer) updateBtcReceiverList(receivers []*types.Receiver) {
	r.receivers = receivers
	r.logger.Infof("*************** btc deposit receiver list ***************")
	for _, receiver := range receivers {
		r.logger.Infof("btc deposit receiver name: %s, address: %s, ethAddress: %s",
			receiver.Name, receiver.Addr, receiver.EthAddr)
	}
	r.logger.Infof("*************** btc deposit receiver list ***************")
}

func (r *TxRelayer) scanBlockLoop() {
	connectErrWaitInterval := time.Second
	btcInterval := time.Minute
	for {
		btcTip, err := r.btcQuery.GetBTCCurrentHeight()
		if err != nil {
			r.logger.Errorf("Failed to get btc tip, error: %v", err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		syncPoint, err := r.GetSyncPoint()
		if err != nil {
			r.logger.Errorf("Failed to get sync point, error: %v", err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		nextBlockHeightToFetch := syncPoint + 1
		if btcTip < nextBlockHeightToFetch+r.delayBlocks {
			r.logger.Infof("No new block, current tip: %d, syncPoint:%d", btcTip, syncPoint)
			time.Sleep(btcInterval)
			continue
		}

		msgBlock, err := r.btcQuery.GetBlockByHeight(nextBlockHeightToFetch)
		if err != nil {
			r.logger.Errorf("Failed to get btc block: %d, err: %v", nextBlockHeightToFetch, err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		depositTxs := r.getValidDepositTxs(nextBlockHeightToFetch, msgBlock)
		if err := r.db.InsertBtcDepositTxs(depositTxs); err != nil {
			r.logger.Errorf("Failed to insert btc deposit txs,blockHeight:%d, error: %v", nextBlockHeightToFetch, err)
			continue
		}

		if err := r.updateSyncPoint(nextBlockHeightToFetch); err != nil {
			r.logger.Errorf("Failed to update sync point, point:%d, error: %v", nextBlockHeightToFetch, err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		r.logger.Infof("Handled block: %d", nextBlockHeightToFetch)
	}
}

func (r *TxRelayer) submitLoop() {
	connectErrWaitInterval := time.Second
	btcInterval := time.Minute
	for {
		lorenzoBTCTipResponse, err := r.lorenzoClient.BTCHeaderChainTip()
		if err != nil {
			r.logger.Errorf("Failed to get lorenzo btc tip, error: %v", err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		txs, err := r.db.GetUnhandledBtcDepositTxs(lorenzoBTCTipResponse.Header.Height)
		if err != nil {
			r.logger.Errorf("Failed to get unhandled btc deposit txs, error: %v", err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		if len(txs) == 0 {
			r.logger.Infof("No unhandled btc deposit txs, lorenzoBTCTip: %d", lorenzoBTCTipResponse.Header.Height)
			time.Sleep(btcInterval)
			continue
		}

		i := 0
		for i < len(txs) {
			tx := txs[i]
			txStakingRecordResp, err := r.lorenzoClient.GetBTCStakingRecord(tx.Txid)
			if err != nil {
				r.logger.Errorf("Failed to get btc staking record, txid: %s, error: %v", tx.Txid, err)
				time.Sleep(connectErrWaitInterval)
				continue
			}
			if txStakingRecordResp.Record != nil {
				if err := r.db.UpdateTxStatus(tx.Txid, db.StatusHandled); err != nil {
					r.logger.Errorf("Failed to update tx status, txid: %s, error: %v", tx.Txid, err)
				}
				i++ // skip transaction have been handled
				continue
			}

			proofRaw, err := r.btcQuery.GetTxBlockProof(tx.Txid)
			if err != nil {
				r.logger.Errorf("Failed to get btc tx proof, txid: %s, error: %v", tx.Txid, err)
				time.Sleep(connectErrWaitInterval)
				continue
			}
			txBytes, err := r.btcQuery.GetTxBytes(tx.Txid)
			if err != nil {
				r.logger.Errorf("Failed to get btc tx bytes, txid: %s, error:%v", tx.Txid, err)
				time.Sleep(connectErrWaitInterval)
				continue
			}

			msg, err := r.newMsgCreateBTCStaking(tx.ReceiverName, tx.ReceiverAddress, r.submitter, proofRaw, txBytes)
			if err != nil {
				r.logger.Errorf("Failed to create msgCreateBTCStaking: %v", err)
				r.updateDepositTxStatus(tx.Txid, db.StatusInvalid)
				i++ // skip this tx
				continue
			}

			_, err = r.lorenzoClient.CreateBTCStakingWithBTCProof(context.Background(), msg)
			if err != nil {
				r.logger.Errorf("Failed to create btc staking with btc proof, txid:%s, error: %v", tx.Txid, err)
				if !isStakingMintTryAgainError(err) {
					r.updateDepositTxStatus(tx.Txid, db.StatusInvalid)
					i++ // skip this tx
				}
				continue
			}

			if err := r.db.UpdateTxStatus(tx.Txid, db.StatusHandled); err != nil {
				r.logger.Errorf("Failed to update tx status, txid: %s, error: %v", tx.Txid, err)
			}

			r.logger.Infof("Submitted btc staking tx, txid: %s", tx.Txid)
			i++
		}
	}
}

func (r *TxRelayer) updateSyncPoint(newPoint uint64) error {
	if err := r.db.UpdateSyncPoint(newPoint); err != nil {
		return err
	}

	return nil
}

func (r *TxRelayer) GetSyncPoint() (uint64, error) {
	return r.db.GetSyncPoint()
}

func (r *TxRelayer) getValidDepositTxs(blockHeight uint64, msgBlock *wire.MsgBlock) []*db.BtcDepositTx {
	var depositTxs []*db.BtcDepositTx

MainLoop:
	for _, tx := range msgBlock.Transactions {
		for _, out := range tx.TxOut {
			pkScript, err := txscript.ParsePkScript(out.PkScript)
			if err != nil {
				continue
			}

			txid := tx.TxHash().String()
			receiverAddr, err := pkScript.Address(r.btcParam)
			if err != nil {
				continue
			}

			receiver := r.GetReceiverByAddress(receiverAddr.String())
			if receiver == nil {
				continue
			}

			var value uint64
			//pick only one valid receiver check
			if receiver.EthAddr == "" {
				value, _, err = btc.ExtractPaymentToWithOpReturnId(tx, receiverAddr)
			} else {
				value, err = btc.ExtractPaymentTo(tx, receiverAddr)
			}
			if err != nil {
				r.logger.Warnf("Invalid tx, txid:%s, error: %v, receiverBTCAddress: %s, receiverName: %s, ethAddr:%v",
					txid, err, receiver.Addr, receiver.Name, receiver.EthAddr)
				continue MainLoop
			}

			//check inputs address if no opReturn
			if receiver.EthAddr != "" {
				for {
					txDetail, err := r.btcQuery.GetTx(txid)
					if err != nil {
						r.logger.Errorf("Failed to get tx detail, txid: %s, error: %v", txid, err)
						time.Sleep(time.Second)
						continue
					}

					for _, vin := range txDetail.Vin {
						if r.IsValidDepositReceiver(vin.Prevout.ScriptPubKeyAddress) {
							//skip transaction if sender is one of receivers
							continue MainLoop
						}
					}
					break
				}
			}

			depositTx := &db.BtcDepositTx{
				ReceiverName:    receiver.Name,
				ReceiverAddress: receiver.Addr,
				Amount:          value,
				Txid:            txid,
				Height:          blockHeight,
				BlockHash:       msgBlock.BlockHash().String(),
				Status:          db.StatusPending,
				BlockTime:       msgBlock.Header.Timestamp,
			}
			depositTxs = append(depositTxs, depositTx)
			continue MainLoop
		}
	}

	return depositTxs
}

func (r *TxRelayer) GetReceiverNameByAddress(addr string) string {
	for _, receiver := range r.receivers {
		if receiver.Addr == addr {
			return receiver.Name
		}
	}

	return ""
}

func (r *TxRelayer) IsValidDepositReceiver(addr string) bool {
	for _, receiver := range r.receivers {
		if receiver.Addr == addr {
			return true
		}
	}

	return false
}

func (r *TxRelayer) GetReceiverByAddress(addr string) *types.Receiver {
	for _, receiver := range r.receivers {
		if receiver.Addr == addr {
			return &types.Receiver{
				Name:    receiver.Name,
				Addr:    receiver.Addr,
				EthAddr: receiver.EthAddr,
			}
		}
	}

	return nil
}

func (r *TxRelayer) updateDepositTxStatus(txid string, status int) {
	if err := r.db.UpdateTxStatus(txid, status); err != nil {
		r.logger.Errorf("Failed to update tx status to [%d], txid: %s, error: %v", status, txid, err)
	}
}

func (r *TxRelayer) newMsgCreateBTCStaking(receiverName string, receiverAddressHex string, submitterAddressHex string, proofRaw []byte, txBytes []byte) (*types.MsgCreateBTCStaking, error) {
	merkleBlock, err := keeper.ParseMerkleBlock(proofRaw)
	if err != nil {
		return nil, err
	}
	blockHash := merkleBlock.Header.BlockHash()

	var blockHashBytes lrztypes.BTCHeaderHashBytes
	blockHashBytes.FromChainhash(&blockHash)
	txIndex, proofBytes, err := keeper.ParseBTCProof(merkleBlock)
	if err != nil {
		return nil, err
	}

	msg := &types.MsgCreateBTCStaking{
		Signer:   submitterAddressHex,
		Receiver: receiverName,
		StakingTx: &types.TransactionInfo{
			Key: &types.TransactionKey{
				Index: txIndex,
				Hash:  &blockHashBytes,
			},
			Proof:       proofBytes,
			Transaction: txBytes,
		},
	}

	return msg, nil
}

func isStakingMintTryAgainError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), LorenzoTimeoutErrorMessage) ||
		strings.Contains(err.Error(), LorenzoBtcStakingNotConfirmedErrorMessage) ||
		strings.Contains(err.Error(), LorenzoBtcStakingDuplicateTxErrorMessage) ||
		strings.Contains(err.Error(), LorenzoBtcHeaderNotFoundErrorMessage) ||
		strings.Contains(err.Error(), PostFailedMessage))
}

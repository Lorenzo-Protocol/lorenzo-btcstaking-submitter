package main

import (
	"context"
	"sync"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	lrztypes "github.com/Lorenzo-Protocol/lorenzo/types"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/keeper"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/btc"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
)

type TxRelayer struct {
	logger            *zap.SugaredLogger
	confirmationDepth uint64

	btcParam  *chaincfg.Params
	submitter string

	btcQuery      *btc.BTCQuery
	lorenzoClient *lrzclient.Client
	db            db.IDB

	// btc deposit tx receiver
	receivers []*types.Receiver
	// current synchronize point
	syncPoint uint64

	wg sync.WaitGroup
}

func NewTxRelayer(database db.IDB, logger *zap.SugaredLogger, conf *config.TxRelayerConfig, btcQuery *btc.BTCQuery, lorenzoClient *lrzclient.Client) (*TxRelayer, error) {
	btcStakingParams, err := lorenzoClient.QueryBTCStakingParams()
	if err != nil {
		return nil, err
	}

	syncPoint, err := database.GetSyncPoint()
	if err != nil {
		return nil, err
	}
	btcParam := btc.GetBTCParams(conf.NetParams)

	receivers := make([]*types.Receiver, 0, len(btcStakingParams.Params.Receivers))
	for _, receiver := range btcStakingParams.Params.Receivers {
		receivers = append(receivers, receiver)
	}

	txRelayer := &TxRelayer{
		logger:            logger,
		confirmationDepth: conf.ConfirmationDepth,
		btcQuery:          btcQuery,
		lorenzoClient:     lorenzoClient,
		db:                database,
		btcParam:          btcParam,

		receivers: receivers,
		syncPoint: syncPoint,
		submitter: lorenzoClient.MustGetAddr(),

		wg: sync.WaitGroup{},
	}

	logger.Infof("new txRelayer on BTC network: %s, confirmation: %d", conf.NetParams, conf.ConfirmationDepth)
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
		if btcTip <= r.syncPoint+r.confirmationDepth {
			r.logger.Infof("No new block, current tip: %d, syncPoint:%d", btcTip, r.syncPoint)
			time.Sleep(btcInterval)
			continue
		}

		wantToGetBlockHeight := r.syncPoint + 1
		msgBlock, err := r.btcQuery.GetBlockByHeight(wantToGetBlockHeight)
		if err != nil {
			r.logger.Errorf("Failed to get btc block: %d, err: %v", wantToGetBlockHeight, err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		depositTxs := r.getValidDepositTxs(wantToGetBlockHeight, msgBlock)
		if err := r.db.InsertBtcDepositTxs(depositTxs); err != nil {
			r.logger.Errorf("Failed to insert btc deposit txs,blockHeight:%d, error: %v", wantToGetBlockHeight, err)
			continue
		}

		if err := r.updateSyncPoint(wantToGetBlockHeight); err != nil {
			r.logger.Errorf("Failed to update sync point, point:%d, error: %v", wantToGetBlockHeight, err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		r.logger.Infof("Handled block: %d", wantToGetBlockHeight)
	}
}

func (r *TxRelayer) submitLoop() {
	connectErrWaitInterval := time.Second
	btcInterval := time.Minute
	for {
		txs, err := r.db.GetUnhandledBtcDepositTxs()
		if err != nil {
			r.logger.Errorf("Failed to get unhandled btc deposit txs, error: %v", err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		if len(txs) == 0 {
			r.logger.Infof("No unhandled btc deposit txs")
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
				i++
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

			msg, err := r.newMsgCreateBTCStaking(tx.Receiver, tx.ReceiverAddress, r.submitter, proofRaw, txBytes)
			if err != nil {
				r.logger.Errorf("Failed to create msgCreateBTCStaking: %v", err)
				if err := r.db.UpdateTxStatus(tx.Txid, db.StatusInvalid); err != nil {
					r.logger.Errorf("Failed to update tx status, txid: %s, error: %v", tx.Txid, err)
				}
				i++ // skip this tx
				continue
			}

			_, err = r.lorenzoClient.CreateBTCStakingWithBTCProof(context.Background(), msg)
			if err != nil {
				r.logger.Errorf("Failed to create btc staking with btc proof, error: %v", err)
				time.Sleep(connectErrWaitInterval)
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

	r.syncPoint = newPoint
	return nil
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
			if !r.IsValidDepositReceiver(receiverAddr.String()) {
				continue
			}

			//pick only one valid receiver check
			value, _, err := btc.ExtractPaymentToWithOpReturnId(tx, receiverAddr)
			if err != nil {
				r.logger.Warnf("Invalid tx, txid:%s, error: %v", txid, err)
				continue MainLoop
			}

			depositTx := &db.BtcDepositTx{
				Receiver:        r.GetReceiverNameByAddress(receiverAddr.String()),
				ReceiverAddress: receiverAddr.String(),
				Amount:          value,
				Txid:            txid,
				Height:          blockHeight,
				BlockHash:       msgBlock.BlockHash().String(),
				Status:          db.StatusPending,
				Timestamp:       msgBlock.Header.Timestamp,
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

func (r *TxRelayer) newMsgCreateBTCStaking(receiver string, receiverAddressHex string, submitterAddressHex string, proofRaw []byte, txBytes []byte) (*types.MsgCreateBTCStaking, error) {
	receiverAddress, err := btcutil.DecodeAddress(receiverAddressHex, r.btcParam)
	if err != nil {
		return nil, err
	}

	stakingMsgTx, err := btc.NewBTCTxFromBytes(txBytes)
	if err != nil {
		return nil, err
	}
	_, _, err = btc.ExtractPaymentToWithOpReturnId(stakingMsgTx, receiverAddress)
	if err != nil {
		return nil, err
	}

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
		Receiver: receiver, // receiver name
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

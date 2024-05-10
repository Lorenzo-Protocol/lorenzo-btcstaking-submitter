package main

import (
	"context"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	lrztypes "github.com/Lorenzo-Protocol/lorenzo/types"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/keeper"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcutil"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/btc"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
)

type TxRelayer struct {
	logger *zap.SugaredLogger
	conf   *config.TxRelayerConfig

	btcQuery      *btc.BTCQuery
	lorenzoClient *lrzclient.Client
	db            db.IDB

	// btc receiving address's newest handled transaction
	newestHandledBtcTxid string
}

func NewTxRelayer(database db.IDB, logger *zap.SugaredLogger, conf *config.TxRelayerConfig, btcQuery *btc.BTCQuery, lorenzoClient *lrzclient.Client) *TxRelayer {
	return &TxRelayer{
		logger:        logger,
		conf:          conf,
		btcQuery:      btcQuery,
		lorenzoClient: lorenzoClient,
		db:            database,

		newestHandledBtcTxid: db.GetNewestBtcTxid(database),
	}
}

func (r *TxRelayer) Start() error {
	r.logger.Infof("staring tx-relayer on BTC network: %s", r.conf.NetParams)
	btcReceivingAddressHex := r.conf.TargetDepositAddress
	btcReceivingAddress, err := btcutil.DecodeAddress(r.conf.TargetDepositAddress, btc.GetBTCParams(r.conf.NetParams))
	if err != nil {
		r.logger.Errorf("invalid Lorenzo staking account")
		return err
	}

	btcInterval := time.Minute
	round := 1
	for {
		r.startSubmit(btcReceivingAddressHex, btcReceivingAddress)
		r.logger.Infof("finish round: %d, wait for while.", round)
		r.logger.Infof("*** %s ***", r.newestHandledBtcTxid)
		round++
		time.Sleep(btcInterval)
	}
}

func (r *TxRelayer) startSubmit(btcReceivingAddressHex string, btcReceivingAddress btcutil.Address) {
	restInterval := time.Second
	btcConfirmationDepth := r.conf.ConfirmationDepth
	preHandledTxid := ""

	firstHandledTxid := ""
	updateFirstHandledTxid := func(txid string) {
		if firstHandledTxid == "" {
			firstHandledTxid = txid
		}
	}
	defer func() {
		// make sure all after firstHandledTxid transactions is handled
		if firstHandledTxid != "" {
			r.saveNewestTxid(firstHandledTxid)
		}
	}()

	for {
		txs, err := r.btcQuery.GetTxs(btcReceivingAddressHex, preHandledTxid)
		if err != nil {
			r.logger.Errorf("failed to get txs from BTC chain, lastest txid: %s. error:%v", preHandledTxid, err)
			time.Sleep(restInterval)
			continue
		}

		if len(txs) == 0 {
			r.logger.Infof("no new txs found")
			return
		}

		for i := 0; i < len(txs); i++ {
			btcCurrentHeight, err := r.btcQuery.GetBTCCurrentHeight()
			if err != nil {
				r.logger.Warnf("failed to get BTC current height. error:%v", err)
				i--
				time.Sleep(restInterval)
				continue
			}
			tx := txs[i]
			if tx.Status.BlockHeight+btcConfirmationDepth > int(btcCurrentHeight) {
				continue
			}

			if r.isNewestTxid(tx.Txid) {
				//all after tx.Txid transactions has handled by submitter
				return
			}

			txStakingRecordResp, err := r.lorenzoClient.GetBTCStakingRecord(tx.Txid)
			if err != nil {
				r.logger.Warnf("failed to get tx staking record from lorenzo, error:%v, txid: %s", err, tx.Txid)
				i--
				time.Sleep(restInterval)
				continue
			}
			if txStakingRecordResp.Record != nil {
				// ignore the tx that has been handled
				r.logger.Infof("tx has been handled, txid: %s", tx.Txid)
				preHandledTxid = tx.Txid
				updateFirstHandledTxid(tx.Txid)
				continue
			}

			proofRaw, err := r.btcQuery.GetTxBlockProof(tx.Txid)
			if err != nil {
				r.logger.Errorf("failed to get block proof from BTC chain, txid: %s. error:%v",
					tx.Txid, err)
				i--
				time.Sleep(restInterval)
				continue
			}
			txBytes, err := r.btcQuery.GetTxBytes(tx.Txid)
			if err != nil {
				r.logger.Errorf("failed to get tx bytes from BTC chain, txid: %s. error: %v", tx.Txid, err)
				i--
				time.Sleep(restInterval)
				continue
			}

			msg, err := r.newMsgCreateBTCStaking(btcReceivingAddress, r.lorenzoClient, proofRaw, txBytes)
			if err != nil {
				r.logger.Errorf("failed to create BTC staking message. txid:%s, error:%v", tx.Txid, err)
				preHandledTxid = tx.Txid
				updateFirstHandledTxid(tx.Txid)
				continue
			}

			_, err = r.lorenzoClient.CreateBTCStakingWithBTCProof(context.Background(), msg)
			if err != nil {
				r.logger.Errorf("failed to submit BTC staking message. txid:%s, error:%v", tx.Txid, err)
				i--
				time.Sleep(restInterval)
				continue
			}

			r.logger.Infof("create btc staking with btc proof successfully, txid: %s", tx.Txid)
			preHandledTxid = tx.Txid
			updateFirstHandledTxid(tx.Txid)
			continue
		}
	}
}

func (r *TxRelayer) saveNewestTxid(txid string) {
	if err := db.SetNewestBtcTxid(r.db, txid); err != nil {
		r.logger.Warnf("save newest handled txid failed, txid: %s", txid)
	}
	r.newestHandledBtcTxid = txid
}

func (r *TxRelayer) isNewestTxid(txid string) bool {
	return r.newestHandledBtcTxid == txid
}

func (r *TxRelayer) newMsgCreateBTCStaking(btcReceivingAddr btcutil.Address, lorenzoClient *lrzclient.Client, proofRaw []byte, txBytes []byte) (*types.MsgCreateBTCStaking, error) {
	stakingMsgTx, err := btc.NewBTCTxFromBytes(txBytes)
	if err != nil {
		return nil, err
	}
	_, _, err = btc.ExtractPaymentToWithOpReturnId(stakingMsgTx, btcReceivingAddr)
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
		Signer: lorenzoClient.MustGetAddr(),
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

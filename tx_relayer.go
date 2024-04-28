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
}

func NewTxRelayer(db db.IDB, logger *zap.SugaredLogger, conf *config.TxRelayerConfig, btcQuery *btc.BTCQuery, lorenzoClient *lrzclient.Client) *TxRelayer {
	return &TxRelayer{
		logger:        logger,
		conf:          conf,
		btcQuery:      btcQuery,
		lorenzoClient: lorenzoClient,
		db:            db,
	}
}

func (r *TxRelayer) Start() error {
	r.logger.Infof("staring tx-relayer on BTC network: %s", r.conf.NetParams)
	depositTargetAccount := r.conf.TargetDepositAddress
	btcReceivingAddr, err := btcutil.DecodeAddress(r.conf.TargetDepositAddress, btc.GetBTCParams(r.conf.NetParams))
	if err != nil {
		r.logger.Errorf("invalid Lorenzo staking account")
		return err
	}

	restInterval := time.Second
	btcInterval := time.Minute
	btcConfirmationDepth := r.conf.ConfirmationDepth
	preHandledTxid := db.GetLastSeenBtcTxid(r.db)
	for {
		txs, err := r.btcQuery.GetTxs(depositTargetAccount, preHandledTxid)
		if err != nil {
			r.logger.Errorf("failed to get txs from BTC chain, lastest txid: %s. error:%v", preHandledTxid, err)
			time.Sleep(restInterval)
			continue
		}

		if len(txs) == 0 {
			r.logger.Infof("no new txs found")
			time.Sleep(btcInterval)
			continue
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
				r.logger.Debugf("tx is not finalized, txid: %s. height:%d, currentHeight:%d",
					tx.Txid, tx.Status.BlockHeight, btcCurrentHeight)
				i--
				time.Sleep(btcInterval)
				continue
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

			msg, err := r.newMsgCreateBTCStaking(btcReceivingAddr, r.lorenzoClient, proofRaw, txBytes)
			if err != nil {
				r.logger.Errorf("failed to create BTC staking message. txid:%s, error:%v", tx.Txid, err)
				preHandledTxid = tx.Txid
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
			if err := db.SetLastSeenBtcTxid(r.db, tx.Txid); err != nil {
				r.logger.Errorf("failed to save last seen btc txid:%s", tx.Txid)
			}
			continue
		}
	}
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

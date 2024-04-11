package main

import (
	"context"
	"flag"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	lrztypes "github.com/Lorenzo-Protocol/lorenzo/types"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/keeper"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/types"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "./sample-config.yml", "config file")
	flag.Parse()

	cfg, err := NewConfig(configFile)
	if err != nil {
		panic(err)
	}
	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, nil)
	if err != nil {
		panic(err)
	}
	btcClient := NewBTCClient(cfg.IsTestNet())

	parentLogger, err := cfg.CreateLogger()
	if err != nil {
		panic(err)
	}
	logger := parentLogger.With().Sugar()

	depositTargetAccount := cfg.BTC.TargetDepositAddress
	restInterval := time.Second
	btcInterval := time.Minute
	btcConfirmationDepth := cfg.BTC.ConfirmationDepth
	preHandledTxid := cfg.BTC.PreHandledTxid
	for {
		txs, err := btcClient.GetTxs(depositTargetAccount, preHandledTxid)
		if err != nil {
			logger.Warnf("failed to get txs from BTC chain, lastest txid: %s. Will try again, error:%v", preHandledTxid, err)
			time.Sleep(restInterval)
			continue
		}

		if len(txs) == 0 {
			logger.Debugf("no new txs found, will try again")
			time.Sleep(btcInterval)
			continue
		}

		for i := 0; i < len(txs); i++ {
			btcCurrentHeight, err := btcClient.GetBTCCurrentHeight()
			if err != nil {
				logger.Warnf("failed to get BTC current height. Will try again. error:%v", err)
				i--
				time.Sleep(restInterval)
				continue
			}
			tx := txs[i]
			if tx.Status.BlockHeight+btcConfirmationDepth > int(btcCurrentHeight) {
				logger.Debugf("tx is not mature enough, txid: %s. height:%d, currentHeight:%d, Will try again",
					tx.Txid, tx.Status.BlockHeight, btcCurrentHeight)
				i--
				time.Sleep(btcInterval)
				continue
			}

			txStakingRecordResp, err := lorenzoClient.GetBTCStakingRecord(tx.Txid)
			if err != nil {
				logger.Warnf("failed to get tx staking record from lorenzo, error:%v, txid: %s. Will try again", err, tx.Txid)
				i--
				time.Sleep(restInterval)
				continue
			}
			if txStakingRecordResp.Record != nil {
				// ignore the tx that has been handled
				preHandledTxid = tx.Txid
				continue
			}

			proofRaw, err := btcClient.GetTxBlockProof(tx.Txid)
			if err != nil {
				logger.Warnf("failed to get block proof from BTC chain, txid: %s. error:%v, Will try again",
					tx.Txid, err)
				i--
				time.Sleep(restInterval)
				continue
			}
			txBytes, err := btcClient.GetTxBytes(tx.Txid)
			if err != nil {
				logger.Warnf("failed to get tx bytes from BTC chain, txid: %s. error: %v, Will try again", tx.Txid, err)
				i--
				time.Sleep(restInterval)
				continue
			}

			if err := submitDepositTxProof(lorenzoClient, proofRaw, txBytes); err != nil {
				logger.Warnf("failed to submit deposit tx proof to lorenzo, txid: %s. error: %v, Will try again", tx.Txid, err)
				i--
				time.Sleep(restInterval)
				continue
			}

			logger.Infof("create btc staking with btc proof successfully, txid: %s", tx.Txid)
			preHandledTxid = tx.Txid
			continue
		}
	}
}

func submitDepositTxProof(lorenzoClient *lrzclient.Client, proofRaw []byte, txBytes []byte) error {
	merkleBlock, err := keeper.ParseMerkleBlock(proofRaw)
	if err != nil {
		return err
	}
	blockHash := merkleBlock.Header.BlockHash()

	var blockHashBytes lrztypes.BTCHeaderHashBytes
	blockHashBytes.FromChainhash(&blockHash)
	txIndex, proofBytes, err := keeper.ParseBTCProof(merkleBlock)
	if err != nil {
		return err
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

	_, err = lorenzoClient.CreateBTCStakingWithBTCProof(context.Background(), msg)
	if err != nil {
		return err
	}
	return nil
}

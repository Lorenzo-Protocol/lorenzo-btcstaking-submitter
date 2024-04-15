package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	lrztypes "github.com/Lorenzo-Protocol/lorenzo/types"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/keeper"
	"github.com/Lorenzo-Protocol/lorenzo/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
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
	btcReceivingAddr, err := btcutil.DecodeAddress(cfg.BTC.TargetDepositAddress, getBTCParams(cfg.BTC.NetParams))
	if err != nil {
		panic(err)
	}

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
			logger.Infof("no new txs found, will try again")
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
				logger.Infof("tx has been handled, txid: %s", tx.Txid)
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

			if err := submitDepositTxProof(btcReceivingAddr, lorenzoClient, proofRaw, txBytes); err != nil {
				logger.Warnf("failed to submit deposit tx proof to lorenzo, txid: %s. error: %v", tx.Txid, err)
				preHandledTxid = tx.Txid
				continue
			}

			logger.Infof("create btc staking with btc proof successfully, txid: %s", tx.Txid)
			preHandledTxid = tx.Txid
			continue
		}
	}
}

func submitDepositTxProof(btcReceivingAddr btcutil.Address, lorenzoClient *lrzclient.Client, proofRaw []byte, txBytes []byte) error {
	stakingMsgTx, err := newBTCTxFromBytes(txBytes)
	if err != nil {
		return err
	}
	_, _, err = extractPaymentToWithOpReturnId(stakingMsgTx, btcReceivingAddr)
	if err != nil {
		return err
	}

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

func extractPaymentToWithOpReturnId(tx *wire.MsgTx, addr btcutil.Address) (uint64, []byte, error) {
	const maxOpReturnPkScriptSize = 83
	payToAddrScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid address")
	}
	var amt uint64 = 0
	foundOpReturnId := false
	var opReturnId []byte
	for _, out := range tx.TxOut {
		if bytes.Equal(out.PkScript, payToAddrScript) {
			amt += uint64(out.Value)
		} else {
			pkScript := out.PkScript
			pkScriptLen := len(pkScript)
			// valid op return script will have at least 2 bytes
			// - fisrt byte should be OP_RETURN marker
			// - second byte should indicate how many bytes there are in opreturn script
			if pkScriptLen > 1 &&
				pkScriptLen <= maxOpReturnPkScriptSize &&
				pkScript[0] == txscript.OP_RETURN {

				// if this is OP_PUSHDATA1, we need to drop first 3 bytes as those are related
				// to script iteslf i.e OP_RETURN + OP_PUSHDATA1 + len of bytes
				if pkScript[1] == txscript.OP_PUSHDATA1 {
					opReturnId = pkScript[3:]
				} else {
					// this should be one of OP_DATAXX opcodes we drop first 2 bytes
					opReturnId = pkScript[2:]
				}
				foundOpReturnId = true
			}
		}
	}
	if !foundOpReturnId {
		return 0, nil, fmt.Errorf("expected op_return_id not found")
	}
	return amt, opReturnId, nil
}

func newBTCTxFromBytes(txBytes []byte) (*wire.MsgTx, error) {
	var msgTx wire.MsgTx
	rbuf := bytes.NewReader(txBytes)
	if err := msgTx.Deserialize(rbuf); err != nil {
		return nil, err
	}

	return &msgTx, nil
}

func getBTCParams(net string) *chaincfg.Params {
	switch net {
	case "mainnet":
		return &chaincfg.MainNetParams
	case "testnet":
		return &chaincfg.TestNet3Params
	}

	err := fmt.Errorf(
		"BTC network with name %s does not exist. should be one of {testnet,mainnet}",
		net,
	)
	panic(err)
}

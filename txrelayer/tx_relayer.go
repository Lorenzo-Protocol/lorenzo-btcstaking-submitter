package txrelayer

import (
	"context"
	"strings"
	"sync"
	"time"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v3/client"
	lrztypes "github.com/Lorenzo-Protocol/lorenzo/v3/types"
	agenttypes "github.com/Lorenzo-Protocol/lorenzo/v3/x/agent/types"
	"github.com/Lorenzo-Protocol/lorenzo/v3/x/btcstaking/keeper"
	"github.com/Lorenzo-Protocol/lorenzo/v3/x/btcstaking/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types/query"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/v2/btc"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/v2/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/v2/db"
)

type TxRelayer struct {
	chainName   string
	logger      *zap.SugaredLogger
	delayBlocks uint64

	btcParam  *chaincfg.Params
	submitter string

	btcQuery      *btc.BTCQuery
	lorenzoClient *lrzclient.Client
	repository    db.IBTCRepository

	agents []agenttypes.Agent

	wg   sync.WaitGroup
	quit chan struct{}
}

func NewTxRelayer(logger *zap.SugaredLogger, conf *config.TxRelayerConfig, lorenzoClient *lrzclient.Client) (*TxRelayer, error) {
	btcQuery := btc.NewBTCQuery(conf.BtcApiEndpoint)
	logger = logger.Named("btc")

	repository, err := db.NewBTCRepository()
	if err != nil {
		return nil, err
	}
	// check if sync point is set, if not set it to start block height
	if height, err := repository.GetSyncPoint(); err != nil {
		return nil, err
	} else if height == 0 {
		if err := repository.UpdateSyncPoint(conf.StartBlockHeight); err != nil {
			return nil, err
		}
	}

	btcParam := btc.GetBTCParams(conf.NetParams)
	txRelayer := &TxRelayer{
		chainName:     "BTC",
		logger:        logger,
		delayBlocks:   conf.ConfirmationDepth,
		btcQuery:      btcQuery,
		lorenzoClient: lorenzoClient,
		repository:    repository,
		btcParam:      btcParam,
		submitter:     lorenzoClient.MustGetAddr(),

		wg:   sync.WaitGroup{},
		quit: make(chan struct{}),
	}
	if err := txRelayer.updateAgentsList(); err != nil {
		return nil, err
	}

	logger.Infof("new txRelayer on BTC network: %s, confirmations: %d, submitter: %s",
		conf.NetParams, conf.ConfirmationDepth+1, txRelayer.submitter)
	return txRelayer, nil
}

func (r *TxRelayer) Start() {
	r.wg.Add(2)
	go func() {
		defer r.wg.Done()
		r.scanBlockLoop()
	}()
	go func() {
		defer r.wg.Done()
		go r.submitLoop()
	}()
}

func (r *TxRelayer) Stop() {
	close(r.quit)
}

func (r *TxRelayer) WaitForShutdown() {
	r.wg.Wait()
}

func (r *TxRelayer) ChainName() string {
	return r.chainName
}

func (r *TxRelayer) scanBlockLoop() {
	connectErrWaitInterval := time.Second
	btcInterval := time.Minute
	for {
		select {
		case <-r.quit:
			return
		default:
		}

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
		if err := r.repository.InsertBtcDepositTxs(depositTxs); err != nil {
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
		select {
		case <-r.quit:
			return
		default:
		}

		lorenzoBTCTipResponse, err := r.lorenzoClient.BTCHeaderChainTip()
		if err != nil {
			r.logger.Errorf("Failed to get lorenzo btc tip, error: %v", err)
			time.Sleep(connectErrWaitInterval)
			continue
		}

		txs, err := r.repository.GetUnhandledBtcDepositTxs(lorenzoBTCTipResponse.Header.Height)
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
				if err := r.repository.UpdateTxStatus(tx.Txid, db.StatusSuccess); err != nil {
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

			if tx.AgentId == 0 {
				agent := r.GetAgentByAddress(tx.ReceiverAddress)
				if agent == nil {
					r.logger.Warnf("Agent not found for btc deposit tx, txid: %s, receiverAddress: %s", tx.Txid, tx.ReceiverAddress)
					r.updateDepositTxStatus(tx.Txid, db.StatusReceiverIsNotBelongToAgent)
					i++
					continue
				}

				tx.AgentId = agent.Id
			}

			msg, err := r.newMsgCreateBTCStaking(tx.AgentId, r.submitter, proofRaw, txBytes)
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

			if err := r.repository.UpdateTxStatus(tx.Txid, db.StatusSuccess); err != nil {
				r.logger.Errorf("Failed to update tx status, txid: %s, error: %v", tx.Txid, err)
			}

			r.logger.Infof("Submitted btc staking tx, txid: %s", tx.Txid)
			i++
		}
	}
}

func (r *TxRelayer) updateSyncPoint(newPoint uint64) error {
	if err := r.repository.UpdateSyncPoint(newPoint); err != nil {
		return err
	}

	return nil
}

func (r *TxRelayer) GetSyncPoint() (uint64, error) {
	return r.repository.GetSyncPoint()
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

			agent := r.GetAgentByAddress(receiverAddr.String())
			if agent == nil {
				continue
			}

			var value uint64
			//pick only one valid agent check
			if agent.EthAddr == "" {
				value, _, err = btc.ExtractPaymentToWithOpReturnId(tx, receiverAddr)
			} else {
				value, err = btc.ExtractPaymentTo(tx, receiverAddr)
			}
			if err != nil {
				r.logger.Warnf("Invalid tx, txid:%s, error: %v, receiverBTCAddress: %s, receiverName: %s, ethAddr:%v",
					txid, err, agent.BtcReceivingAddress, agent.Name, agent.EthAddr)
				continue MainLoop
			}

			//check inputs address if no opReturn
			if agent.EthAddr != "" {
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
				AgentId:         agent.Id,
				ReceiverName:    agent.Name,
				ReceiverAddress: agent.BtcReceivingAddress,
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

func (r *TxRelayer) IsValidDepositReceiver(addr string) bool {
	for _, agent := range r.agents {
		if agent.BtcReceivingAddress == addr {
			return true
		}
	}

	return false
}

func (r *TxRelayer) updateDepositTxStatus(txid string, status int) {
	if err := r.repository.UpdateTxStatus(txid, status); err != nil {
		r.logger.Errorf("Failed to update tx status to [%d], txid: %s, error: %v", status, txid, err)
	}
}

func (r *TxRelayer) newMsgCreateBTCStaking(agentId uint64, submitterAddressHex string, proofRaw []byte, txBytes []byte) (*types.MsgCreateBTCStaking, error) {
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
		Signer:  submitterAddressHex,
		AgentId: agentId,
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

func (r *TxRelayer) updateAgentsList() error {
	var agents []agenttypes.Agent
	var nextKey []byte
	for {
		agentsResponse, err := r.lorenzoClient.Agents(&query.PageRequest{
			Key:        nextKey,
			CountTotal: false,
			Reverse:    false,
		})
		if err != nil {
			return err
		}
		agents = append(agents, agentsResponse.Agents...)
		if agentsResponse.Pagination.NextKey == nil {
			break
		}

		nextKey = agentsResponse.Pagination.NextKey
	}

	r.agents = agents
	r.logger.Info("*************** agents ***************")
	for _, agent := range agents {
		r.logger.Infof("agent id: %d, name: %s, btcReceivingAddress: %s, ethAddr: %s, description: %s, url: %s",
			agent.Id, agent.Name, agent.BtcReceivingAddress, agent.EthAddr, agent.Description, agent.Url)
	}
	r.logger.Infof("*************** btc deposit receiver list ***************")
	r.logger.Info("*************** agents ***************")
	return nil
}

func (r *TxRelayer) GetAgentByAddress(addr string) *agenttypes.Agent {
	for _, agent := range r.agents {
		if agent.BtcReceivingAddress == addr {
			return &agenttypes.Agent{
				Id:                  agent.Id,
				Name:                agent.Name,
				BtcReceivingAddress: agent.BtcReceivingAddress,
				EthAddr:             agent.EthAddr,
				Description:         agent.Description,
				Url:                 agent.Url,
			}
		}
	}

	return nil
}

func isStakingMintTryAgainError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), LorenzoTimeoutErrorMessage) ||
		strings.Contains(err.Error(), LorenzoBtcStakingNotConfirmedErrorMessage) ||
		strings.Contains(err.Error(), LorenzoBtcStakingDuplicateTxErrorMessage) ||
		strings.Contains(err.Error(), LorenzoBtcHeaderNotFoundErrorMessage) ||
		strings.Contains(err.Error(), PostFailedMessage) ||
		strings.Contains(err.Error(), SequenceMismatch))
}

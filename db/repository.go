package db

import (
	"errors"

	"gorm.io/gorm"
)

const submitterBtcSyncPointKey = "submitter/btc-sync-point"

const (
	StatusPending                    = 0
	StatusHandled                    = 1
	StatusInvalid                    = 2
	StatusReceiverIsNotBelongToAgent = 3
)

const (
	BatchHandleBtcDepositTxsNum = 50
)

type BtcRepository struct {
	db *gorm.DB

	syncPointKey string
}

func NewBTCRepository() (IBTCRepository, error) {
	if DB == nil {
		return nil, errors.New("DB is not initialized yet")
	}

	mysqlDb := &BtcRepository{
		db:           DB,
		syncPointKey: submitterBtcSyncPointKey,
	}

	return mysqlDb, nil
}

func (r *BtcRepository) UpdateSyncPoint(height uint64) error {
	return SetUint64(r.db, r.syncPointKey, height)
}

func (r *BtcRepository) GetSyncPoint() (uint64, error) {
	return GetUint64(r.db, r.syncPointKey)
}

func (r *BtcRepository) InsertBtcDepositTxs(txs []*BtcDepositTx) (err error) {
	if len(txs) == 0 {
		return nil
	}

	return r.db.Transaction(func(dbtx *gorm.DB) error {
		for _, tx := range txs {
			//check tx is already exist
			if ok, err := r.hasDepositTxByTxid(dbtx, tx.Txid); err != nil {
				return err
			} else if ok {
				continue
			}

			err := dbtx.Create(tx).Error
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *BtcRepository) GetUnhandledBtcDepositTxs(lorenzoBTCTip uint64) ([]*BtcDepositTx, error) {
	var txs []*BtcDepositTx
	// BTC block timestamp is not strictly increasing.
	err := r.db.Model(&BtcDepositTx{}).
		Where("status=? AND (amount<? OR (amount<? AND height<=?) OR (amount<? AND height<=?) OR (amount<? AND height<=?) OR (amount>=? AND height<=?))",
			StatusPending, Dep0Amount, Dep1Amount, lorenzoBTCTip-1, Dep2Amount, lorenzoBTCTip-2, Dep3Amount, lorenzoBTCTip-3, Dep3Amount, lorenzoBTCTip-4).
		Order("height ASC").Limit(BatchHandleBtcDepositTxsNum).Find(&txs).Error
	if err != nil {
		return nil, err
	}

	return txs, nil
}

func (r *BtcRepository) UpdateTxStatus(txid string, status int) error {
	result := r.db.Model(&BtcDepositTx{}).Where("txid = ?", txid).Update("status", status)
	return result.Error
}

func (r *BtcRepository) hasDepositTxByTxid(dbtx *gorm.DB, txid string) (bool, error) {
	var count int64
	result := dbtx.Model(&BtcDepositTx{}).Where("txid = ?", txid).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}

	return count > 0, nil
}

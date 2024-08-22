package db

import (
	"errors"
	"gorm.io/gorm"
)

// BNBRepository BNB Smart Chain transaction relayer repository
type BNBRepository struct {
	db           *gorm.DB
	chainName    string
	syncPointKey string
}

func (r *BNBRepository) MarkSuccess(txid string) error {
	return r.db.Model(&WrappedBTCDepositTx{}).Where("chain = ? AND txid = ?", r.chainName, txid).
		Update("status", StatusSuccess).Error
}

func (r *BNBRepository) MarkInvalid(txid string) error {
	return r.db.Model(&WrappedBTCDepositTx{}).Where("chain = ? AND txid = ?", r.chainName, txid).
		Update("status", StatusInvalid).Error
}

func (r *BNBRepository) InsertWrappedBTCDepositTxs(txs []*WrappedBTCDepositTx) error {
	return r.db.Transaction(func(dbtx *gorm.DB) error {
		for _, tx := range txs {
			if ok, err := r.hasWrappedBTCDepositTxByTxid(dbtx, tx.Chain, tx.Txid); err != nil {
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

func (r *BNBRepository) GetUnhandledWrappedBTCDepositTxs(lorenzoBTCTip uint64) ([]*WrappedBTCDepositTx, error) {
	var txs []*WrappedBTCDepositTx
	result := r.db.Model(&WrappedBTCDepositTx{}).Where("status = ? AND height <= ?", StatusPending, lorenzoBTCTip).
		Order("height").Find(&txs)
	if result.Error != nil {
		return nil, result.Error
	}

	return txs, nil
}

func (r *BNBRepository) UpdateSyncPoint(height uint64) error {
	return SetUint64(r.db, r.syncPointKey, height)
}

func (r *BNBRepository) GetSyncPoint() (uint64, error) {
	return GetUint64(r.db, r.syncPointKey)
}

func (r *BNBRepository) hasWrappedBTCDepositTxByTxid(dbtx *gorm.DB, chain string, txid string) (bool, error) {
	var count int64
	result := dbtx.Model(&WrappedBTCDepositTx{}).Where("chain=? AND txid = ?", chain, txid).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}

	return count > 0, nil
}

func NewBNBRepository(chainName string) (IBNBRepository, error) {
	if DB == nil {
		return nil, errors.New("DB is not initialized yet")
	}

	mysqlDb := &BNBRepository{
		db:           DB,
		chainName:    chainName,
		syncPointKey: submitterBnbSyncPointKey,
	}

	return mysqlDb, nil
}

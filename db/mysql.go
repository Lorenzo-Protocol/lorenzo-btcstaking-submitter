package db

import (
	"fmt"
	"math/big"
	"strconv"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
)

const submitterBtcSyncPointKey = "submitter/btc-sync-point"

const (
	StatusPending = 0
	StatusHandled = 1
	StatusInvalid = 2
)

type MysqlDB struct {
	db *gorm.DB

	syncPointKey string
}

func NewMysqlDB(cfg config.Database) (*MysqlDB, error) {
	dns := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
	db, err := gorm.Open(mysql.Open(dns), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	mysqlDb := &MysqlDB{
		db:           db,
		syncPointKey: submitterBtcSyncPointKey,
	}

	return mysqlDb, nil
}

func (db *MysqlDB) UpdateSyncPoint(height uint64) error {
	var cfg ConfigTable
	cfg.Name = db.syncPointKey
	cfg.Value = big.NewInt(0).SetUint64(height).String()

	return db.db.Model(&ConfigTable{}).Where("name = ? and value<?", cfg.Name, cfg.Value).Updates(cfg).Error
}

func (db *MysqlDB) GetSyncPoint() (uint64, error) {
	var cfg ConfigTable
	result := db.db.Model(&ConfigTable{}).Where("name = ?", db.syncPointKey).First(&cfg)
	if result.Error != nil {
		return 0, nil
	}

	syncPoint, err := strconv.ParseUint(cfg.Value, 10, 64)
	if err != nil {
		return 0, err
	}

	return syncPoint, nil
}

func (db *MysqlDB) InsertBtcDepositTxs(txs []*BtcDepositTx) (err error) {
	dbtx := db.db.Begin()

	defer func() {
		if err != nil {
			dbtx.Rollback()
			return
		}

		dbtx.Commit()
	}()

	for _, tx := range txs {
		//check tx is already exist
		if ok, err := db.hasDepositTxByTxid(dbtx, tx.Txid); err != nil {
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
}

func (db *MysqlDB) GetUnhandledBtcDepositTxs() ([]*BtcDepositTx, error) {
	var txs []*BtcDepositTx
	err := db.db.Model(&BtcDepositTx{}).Where("status = ?", StatusPending).
		Order("block_time ASC").Limit(50).Find(&txs).Error
	if err != nil {
		return nil, err
	}

	return txs, nil
}

func (db *MysqlDB) UpdateTxStatus(txid string, status int) error {
	result := db.db.Model(&BtcDepositTx{}).Where("txid = ?", txid).Update("status", status)
	return result.Error
}

func (db *MysqlDB) hasDepositTxByTxid(dbtx *gorm.DB, txid string) (bool, error) {
	var count int64
	result := dbtx.Model(&BtcDepositTx{}).Where("txid = ?", txid).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}

	return count > 0, nil
}

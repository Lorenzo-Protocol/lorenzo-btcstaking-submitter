package db

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
)

const submitterBtcSyncPointKey = "submitter-btc-sync-point"

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
	db, err := gorm.Open(mysql.Open(dns), &gorm.Config{})
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
		result := dbtx.Create(tx)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return err
		}
	}
	return nil
}

func (db *MysqlDB) GetUnhandledBtcDepositTxs() ([]*BtcDepositTx, error) {
	var txs []*BtcDepositTx
	err := db.db.Model(&BtcDepositTx{}).Where("status = ?", StatusPending).Find(&txs).Error
	if err != nil {
		return nil, err
	}

	return txs, nil
}

func (db *MysqlDB) UpdateTxStatus(txid string, status int) error {
	result := db.db.Model(&BtcDepositTx{}).Where("txid = ?", txid).Update("status", status)
	return result.Error
}

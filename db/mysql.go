package db

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"math/big"
	"strconv"
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

func NewMysqlDB(host string, port int, user string, password string, dbname string) (*MysqlDB, error) {
	dns := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, port, dbname)
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

func (db *MysqlDB) InsertBtcDepositTxs(txs []*BtcDepositTx) error {
	return nil
}

func (db *MysqlDB) GetUnhandledBtcDepositTxs() ([]*BtcDepositTx, error) {
	return []*BtcDepositTx{}, nil
}

func (db *MysqlDB) UpdateTxStatus(txid string, status int) error {
	return nil
}

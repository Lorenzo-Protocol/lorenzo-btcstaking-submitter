package db

import "time"

type BaseTable struct {
	Id          int
	UpdatedTime time.Time `gorm:"autoUpdateTime"`
	CreatedTime time.Time `gorm:"autoCreateTime"`
}

type ConfigTable struct {
	Name  string
	Value string

	BaseTable
}

func (ConfigTable) TableName() string {
	return "config"
}

type BtcDepositTx struct {
	Receiver        string
	ReceiverAddress string
	Value           uint64
	Txid            string
	Height          uint64
	BlockHash       string
	Status          int

	BaseTable
}

func (BtcDepositTx) TableName() string {
	return "btc_deposit_tx"
}

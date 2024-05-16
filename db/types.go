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
	Receiver        string `gorm:"size:256"`
	ReceiverAddress string `gorm:"size:256"`
	Value           uint64
	Txid            string `gorm:"size:256, uniqueIndex"`
	Height          uint64
	BlockHash       string `gorm:"size:256"`
	Status          int

	BaseTable
}

func (BtcDepositTx) TableName() string {
	return "btc_deposit_tx"
}

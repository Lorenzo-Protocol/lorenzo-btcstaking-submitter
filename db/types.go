package db

import "time"

const (
	Dep0Amount = 4e5
	Dep1Amount = 2e6
	Dep2Amount = 1e7
	Dep3Amount = 5e7
)

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
	AgentId         uint64
	ReceiverName    string `gorm:"size:256"`
	ReceiverAddress string `gorm:"size:256"`
	Amount          uint64
	Txid            string `gorm:"size:256,uniqueIndex"`
	Height          uint64
	BlockHash       string `gorm:"size:256"`
	BlockTime       time.Time
	Status          int

	BaseTable
}

func (BtcDepositTx) TableName() string {
	return "btc_deposit_tx"
}

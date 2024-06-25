package blockscout

import (
	"time"
)

type LorenzoBurnAndMintEvent struct {
	Type               int       `json:"type"`
	Status             int       `json:"status"`
	Amount             string    `json:"amount"`
	LorenzoBlockHeight uint64    `json:"lorenzo_block_height"`
	LorenzoAddr        string    `json:"lorenzo_addr"`
	LorenzoTxHash      string    `json:"lorenzo_tx_hash"`
	LorenzoTxTime      time.Time `json:"lorenzo_tx_time"`
}

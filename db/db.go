package db

type ISyncPointRepository interface {
	UpdateSyncPoint(height uint64) error
	GetSyncPoint() (uint64, error)
}

type IBTCRepository interface {
	ISyncPointRepository
	InsertBtcDepositTxs(txs []*BtcDepositTx) error
	GetUnhandledBtcDepositTxs(lorenzoBTCTip uint64) ([]*BtcDepositTx, error)
	UpdateTxStatus(txid string, status int) error
}

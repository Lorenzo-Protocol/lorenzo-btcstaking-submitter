package db

type ISyncPointRepository interface {
	UpdateSyncPoint(height uint64) error
	GetSyncPoint() (uint64, error)
}

type IWrappedBTCDepositTxRepository interface {
	InsertWrappedBTCDepositTxs(txs []*WrappedBTCDepositTx) error
	GetUnhandledWrappedBTCDepositTxs(lorenzoBTCTip uint64) ([]*WrappedBTCDepositTx, error)
	MarkSuccess(txid string) error
	MarkInvalid(txid string) error
}

type IBTCRepository interface {
	ISyncPointRepository
	InsertBtcDepositTxs(txs []*BtcDepositTx) error
	GetUnhandledBtcDepositTxs(lorenzoBTCTip uint64) ([]*BtcDepositTx, error)
	UpdateTxStatus(txid string, status int) error
}

type IBNBRepository interface {
	ISyncPointRepository
	IWrappedBTCDepositTxRepository
}

package db

var newestBtcTxidKey = []byte("newest-btc-txid")

func SetNewestBtcTxid(db IDB, txid string) error {
	return db.Put(newestBtcTxidKey, []byte(txid))
}

func GetNewestBtcTxid(db IDB) string {
	val, _ := db.Get(newestBtcTxidKey)

	return string(val)
}

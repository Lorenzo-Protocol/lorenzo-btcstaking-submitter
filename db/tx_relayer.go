package db

var lastSeenBtcTxidKey = []byte("last-seen-btc-txid")

func SetLastSeenBtcTxid(db IDB, txid string) error {
	return db.Put(lastSeenBtcTxidKey, []byte(txid))
}

func GetLastSeenBtcTxid(db IDB) string {
	val, _ := db.Get(lastSeenBtcTxidKey)

	return string(val)
}

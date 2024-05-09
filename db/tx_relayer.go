package db

var lastSeenBtcTxidKey = []byte("last-seen-btc-txid")
var prefixTxid = []byte("txid-")

func SetLastSeenBtcTxid(db IDB, txid string) error {
	return db.Put(lastSeenBtcTxidKey, []byte(txid))
}

func GetLastSeenBtcTxid(db IDB) string {
	val, _ := db.Get(lastSeenBtcTxidKey)

	return string(val)
}

func SetTxid(db IDB, txid string) error {
	return db.Put(txidKey(txid), []byte(txid))
}

func HasTxid(db IDB, txid string) bool {
	val, _ := db.Get(txidKey(txid))
	return val != nil
}

func txidKey(txid string) []byte {
	return append(prefixTxid, []byte(txid)...)
}

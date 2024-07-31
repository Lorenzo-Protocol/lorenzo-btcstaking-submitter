package db

//import (
//	"fmt"
//	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
//	"testing"
//	"time"
//)
//
//func TestMysqlDB_GetSyncPoint(t *testing.T) {
//	cfg, err := config.NewConfig("../.testnet/sample-config.yml")
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	mysqlDB, err := NewBTCRepository(cfg.Database)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	syncPoint, err := mysqlDB.GetSyncPoint()
//	if err != nil {
//		t.Fatal(err)
//	}
//	fmt.Println(syncPoint)
//
//	if err := mysqlDB.UpdateSyncPoint(2811751); err != nil {
//		t.Fatal(err)
//	}
//
//	if _, err := mysqlDB.GetUnhandledBtcDepositTxs(); err != nil {
//		t.Fatal(err)
//	}
//
//	depositTx := &BtcDepositTx{
//		Txid:      "0x12ab",
//		BlockTime: time.Now(),
//	}
//	if err := mysqlDB.InsertBtcDepositTxs([]*BtcDepositTx{depositTx}); err != nil {
//		t.Fatal(err)
//	}
//	// Duplicate insert
//	if err := mysqlDB.InsertBtcDepositTxs([]*BtcDepositTx{depositTx}); err != nil {
//		t.Fatal(err)
//	}
//}

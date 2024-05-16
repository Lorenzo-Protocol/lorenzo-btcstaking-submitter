package db

import (
	"fmt"
	"testing"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
)

func TestMysqlDB_GetSyncPoint(t *testing.T) {
	cfg, err := config.NewConfig("../.testnet/sample-config.yml")
	if err != nil {
		t.Fatal(err)
	}

	mysqlDB, err := NewMysqlDB(cfg.Database)
	if err != nil {
		t.Fatal(err)
	}

	syncPoint, err := mysqlDB.GetSyncPoint()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(syncPoint)

	if err := mysqlDB.UpdateSyncPoint(2811751); err != nil {
		t.Fatal(err)
	}

	if _, err := mysqlDB.GetUnhandledBtcDepositTxs(); err != nil {
		t.Fatal(err)
	}
}

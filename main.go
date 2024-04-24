package main

import (
	"flag"
	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	"github.com/Lorenzo-Protocol/lorenzo-submit-btcstaking/btc"
	"github.com/Lorenzo-Protocol/lorenzo-submit-btcstaking/config"
	db2 "github.com/Lorenzo-Protocol/lorenzo-submit-btcstaking/db"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "./sample-config.yml", "config file")
	flag.Parse()

	cfg, err := config.NewConfig(configFile)
	if err != nil {
		panic(err)
	}
	db, err := db2.NewLevelDB(cfg.DBDir)
	if err != nil {
		panic(err)
	}

	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, nil)
	if err != nil {
		panic(err)
	}

	btcQuery := btc.NewBTCQuery(cfg.BtcApiEndpoint)

	parentLogger, err := cfg.CreateLogger()
	if err != nil {
		panic(err)
	}
	logger := parentLogger.With().Sugar()

	txRelayer := NewTxRelayer(db, logger, &cfg.TxRelayer, btcQuery, lorenzoClient)
	if err := txRelayer.Start(); err != nil {
		panic(err)
	}
}

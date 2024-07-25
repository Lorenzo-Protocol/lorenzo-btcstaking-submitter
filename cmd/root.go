package cmd

import (
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/btc"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/txrelayer"
	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v2/client"
	"github.com/spf13/cobra"
)

func RootAction(c *cobra.Command, _ []string) {
	configFile, err := c.Flags().GetString("config")
	if err != nil {
		panic(err)
	}

	cfg, err := config.NewConfig(configFile)
	if err != nil {
		panic(err)
	}
	db.Init(cfg.Database)

	database, err := db.NewMysqlDB(db.DB)
	if err != nil {
		panic(err)
	}

	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, nil)
	if err != nil {
		panic(err)
	}

	btcQuery := btc.NewBTCQuery(cfg.BtcApiEndpoint)

	rootLogger, err := cfg.CreateLogger()
	if err != nil {
		panic(err)
	}
	logger := rootLogger.With().Sugar()

	txRelayer, err := txrelayer.NewTxRelayer(database, logger, &cfg.TxRelayer, btcQuery, lorenzoClient)
	if err != nil {
		panic(err)
	}
	txRelayer.Start()

	addInterruptHandler(func() {
		rootLogger.Info("Stopping BTC Tx-relayer...")
		txRelayer.Stop()
		txRelayer.WaitForShutdown()
		rootLogger.Info("BTC Tx-relayer shutdown")
	})

	<-interruptHandlersDone
	rootLogger.Info("Shutdown complete")
}

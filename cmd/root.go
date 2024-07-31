package cmd

import (
	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v2/client"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/txrelayer"
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

	database, err := db.NewMysqlDB(cfg.Database)
	if err != nil {
		panic(err)
	}

	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, nil)
	if err != nil {
		panic(err)
	}

	enableDebug, err := c.Flags().GetBool("debug")
	if err != nil {
		panic(err)
	}
	parentLogger, err := cfg.CreateLogger(enableDebug)
	if err != nil {
		panic(err)
	}
	logger := parentLogger.With().Sugar()

	txRelayer, err := txrelayer.NewTxRelayer(database, logger, &cfg.TxRelayer, lorenzoClient)
	if err != nil {
		panic(err)
	}
	txRelayer.Start()

	addInterruptHandler(func() {
		parentLogger.Sugar().Infof("Stopping %s Tx-relayer...", txRelayer.ChainName())
		txRelayer.Stop()
		txRelayer.WaitForShutdown()
		parentLogger.Sugar().Infof("%s Tx-relayer shutdown", txRelayer.ChainName())
	})
	<-interruptHandlersDone
	parentLogger.Info("Shutdown complete")
}

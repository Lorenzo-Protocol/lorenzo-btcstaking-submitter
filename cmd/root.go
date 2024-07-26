package cmd

import (
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/txrelayer"
	"github.com/spf13/cobra"
)

func RootAction(c *cobra.Command, _ []string) {
	configFile, err := c.Flags().GetString("config")
	if err != nil {
		panic(err)
	}

	debugMode, err := c.Flags().GetBool("debug")
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

	rootLogger, err := cfg.CreateLogger(debugMode)
	if err != nil {
		panic(err)
	}
	logger := rootLogger.With().Sugar()

	var txRelayerList []txrelayer.ITxRelayer
	txRelayer, err := txrelayer.NewTxRelayer(database, logger, &cfg.TxRelayer)
	if err != nil {
		panic(err)
	}
	txRelayerList = append(txRelayerList, txRelayer)

	//bnbTxRelayer, err := txrelayer.NewBnbTxRelayer(cfg.BNBTxRelayer, logger)
	//if err != nil {
	//	panic(err)
	//}
	//txRelayerList = append(txRelayerList, bnbTxRelayer)

	for _, txRelayer := range txRelayerList {
		txRelayer.Start()
		addInterruptHandler(func() {
			rootLogger.Sugar().Infof("Stopping %s Tx-relayer...", txRelayer.ChainName())
			txRelayer.Stop()
			txRelayer.WaitForShutdown()
			rootLogger.Sugar().Infof("%s Tx-relayer shutdown", txRelayer.ChainName())
		})
	}

	<-interruptHandlersDone
	rootLogger.Info("Shutdown complete")
}

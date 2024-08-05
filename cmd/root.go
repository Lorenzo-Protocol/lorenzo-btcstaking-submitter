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
	if err := db.Init(cfg.Database); err != nil {
		panic(err)
	}

	database, err := db.NewBTCRepository()
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
	rootLogger, err := cfg.CreateLogger(enableDebug)
	if err != nil {
		panic(err)
	}
	logger := rootLogger.With().Sugar()

	var txRelayerList []txrelayer.ITxRelayer
	btcTxRelayer, err := txrelayer.NewTxRelayer(database, logger, &cfg.TxRelayer, lorenzoClient)
	if err != nil {
		panic(err)
	}
	txRelayerList = append(txRelayerList, btcTxRelayer)

	if cfg.BNBTxRelayer.RpcUrl != "" {
		bnbTxRelayer, err := txrelayer.NewBnbTxRelayer(cfg.BNBTxRelayer, &cfg.Lorenzo, logger)
		if err != nil {
			panic(err)
		}
		txRelayerList = append(txRelayerList, bnbTxRelayer)
	}

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

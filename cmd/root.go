package cmd

import (
	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v3/client"
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

	enableDebug, err := c.Flags().GetBool("debug")
	if err != nil {
		panic(err)
	}
	parentLogger, err := cfg.CreateLogger(enableDebug)
	if err != nil {
		panic(err)
	}
	logger := parentLogger.With().Sugar()

	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, parentLogger)
	if err != nil {
		panic(err)
	}
	lorenzoClient.SetRetryAttempts(3)

	var txRelayerList []txrelayer.ITxRelayer
	btcTxRelayer, err := txrelayer.NewTxRelayer(logger, &cfg.TxRelayer, lorenzoClient)
	if err != nil {
		panic(err)
	}
	txRelayerList = append(txRelayerList, btcTxRelayer)

	bnbTxRelayer, err := txrelayer.NewBnbTxRelayer(cfg.BNBTxRelayer, lorenzoClient, logger)
	if err != nil {
		logger.Errorf("Failed to create BNB Tx-relayer: %s", err)
	} else {
		txRelayerList = append(txRelayerList, bnbTxRelayer)
	}

	for _, txRelayer := range txRelayerList {
		txRelayer.Start()
		addInterruptHandler(func() {
			parentLogger.Sugar().Infof("Stopping %s Tx-relayer...", txRelayer.ChainName())
			txRelayer.Stop()
			txRelayer.WaitForShutdown()
			parentLogger.Sugar().Infof("%s Tx-relayer shutdown", txRelayer.ChainName())
		})
	}

	<-interruptHandlersDone
	parentLogger.Info("Shutdown complete")
}

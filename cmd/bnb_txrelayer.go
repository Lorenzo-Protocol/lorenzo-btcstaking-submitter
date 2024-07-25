package cmd

import (
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/txrelayer"
	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v2/client"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
)

func BNBTxRelayerCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "bnb",
		Short: "Lorenzo BTC Staking Submitter for submitting BNB Smart Chain transactions",
		Run:   BNBTxRelayerAction,
	}
	command.Flags().StringP("config", "c", "./.testnet/sample-config.yml", "config file")

	return command
}

func BNBTxRelayerAction(c *cobra.Command, _ []string) {
	configFile, err := c.Flags().GetString("config")
	if err != nil {
		panic(err)
	}

	cfg, err := config.NewConfig(configFile)
	if err != nil {
		panic(err)
	}
	db.Init(cfg.Database)

	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, nil)
	if err != nil {
		panic(err)
	}

	rootLogger, err := cfg.CreateLogger()
	if err != nil {
		panic(err)
	}
	logger := rootLogger.With().Sugar()

	txRelayer, err := txrelayer.NewBnbTxRelayer(cfg.BNBTxRelayer, lorenzoClient, logger)
	if err != nil {
		panic(err)
	}
	txRelayer.Start()

	addInterruptHandler(func() {
		rootLogger.Info("Stopping BNB Tx-relayer...")
		txRelayer.Stop()
		txRelayer.WaitForShutdown()
		rootLogger.Info("BNB Tx-relayer shutdown")
	})

	<-interruptHandlersDone
	rootLogger.Info("Shutdown complete")
}

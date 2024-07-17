package main

import (
	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/btc"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/cmd"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/db"
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/txrelayer"
)

func Main(c *cobra.Command, _ []string) {
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

	btcQuery := btc.NewBTCQuery(cfg.BtcApiEndpoint)

	parentLogger, err := cfg.CreateLogger()
	if err != nil {
		panic(err)
	}
	logger := parentLogger.With().Sugar()

	txRelayer, err := txrelayer.NewTxRelayer(database, logger, &cfg.TxRelayer, btcQuery, lorenzoClient)
	if err != nil {
		panic(err)
	}
	if err := txRelayer.Start(); err != nil {
		panic(err)
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "lrz-btcstaking-submitter",
		Short: "Lorenzo BTC Staking Submitter",
		Run:   Main,
	}

	rootCmd.Flags().StringP("config", "c", "./.testnet/sample-config.yml", "config file")
	rootCmd.AddCommand(cmd.BlockscoutRefreshCmd())
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

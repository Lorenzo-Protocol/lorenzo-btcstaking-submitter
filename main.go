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

const DefaultConfigFile = "./.testnet/sample-config.yml"

var configFile string
var rootCmd = &cobra.Command{
	Use:   "lrz-btcstaking-submitter",
	Short: "Lorenzo BTC Staking Submitter",
	Run:   Main,
}

func init() {
	rootCmd.Flags().StringVarP(&configFile, "config", "c", DefaultConfigFile, "config file")
}

func Main(c *cobra.Command, _ []string) {
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
	rootCmd.AddCommand(cmd.BlockscoutRefreshCmd())
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

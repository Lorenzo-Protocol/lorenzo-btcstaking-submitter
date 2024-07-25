package main

import (
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/cmd"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "lrz-btcstaking-submitter",
		Short: "Lorenzo BTC Staking Submitter",
		Run:   cmd.RootAction,
	}
	rootCmd.Flags().StringP("config", "c", "./.testnet/sample-config.yml", "config file")

	rootCmd.AddCommand(cmd.BlockscoutRefreshCmd())
	rootCmd.AddCommand(cmd.BNBTxRelayerCmd())
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

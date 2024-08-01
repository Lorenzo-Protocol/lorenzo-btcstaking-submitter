package main

import (
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/cmd"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "lrz-btcstaking-submitter",
		Short: "Lorenzo BTC Staking Submitter",
		Run:   cmd.RootAction,
	}
	rootCmd.Flags().StringP("config", "c", "./.testnet/sample-config.yml", "config file")
	rootCmd.Flags().Bool("debug", false, "enable debug mode")

	rootCmd.AddCommand(cmd.BlockscoutRefreshCmd())
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

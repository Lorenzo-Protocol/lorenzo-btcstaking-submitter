package main

import "github.com/spf13/cobra"

func main() {
	rootCmd := &cobra.Command{
		Use:   "lrz-btcstaking-submitter",
		Short: "Lorenzo BTC Staking Submitter",
	}

	rootCmd.AddCommand(BlockscoutRefreshCmd())
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

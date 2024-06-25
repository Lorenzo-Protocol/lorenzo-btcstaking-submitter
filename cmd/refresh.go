package main

import (
	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/blockscout"
	"github.com/spf13/cobra"
)

func BlockscoutRefreshCmd() *cobra.Command {
	var blockscoutApiUrl string
	var lorenzoAppApiUrl string
	var startHeight uint64

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the blockscout balance when mint/burn or other events happen",
		Run: func(_ *cobra.Command, _ []string) {
			refresher, err := blockscout.NewRefresher(startHeight, blockscoutApiUrl, lorenzoAppApiUrl)
			if err != nil {
				panic(err)
			}

			if err := refresher.Start(); err != nil {
				panic(err)
			}
		},
	}

	cmd.Flags().StringVar(&blockscoutApiUrl, "blockscout-api",
		"https://scan-testnet.lorenzo-protocol.xyz/api", "Blockscout API URL")
	cmd.Flags().StringVar(&lorenzoAppApiUrl, "lorenzo-app-api",
		"https://app-testnet.lorenzo-protocol.xyz/api", "Lorenzo App api URL")
	cmd.Flags().Uint64Var(&startHeight, "start-height", 0, "Start height to refresh")
	return cmd
}

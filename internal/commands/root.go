package commands

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "teleslurp",
	Short: "Teleslurp is a tool for analyzing Telegram users and groups",
	Long: `Teleslurp allows you to search and analyze Telegram users and their group participation,
utilizing TGScan API for data gathering and providing detailed historical information.`,
}

func Execute() error {
	return rootCmd.Execute()
}

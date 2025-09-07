package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "webhook",
	Short: "a server program for processing and weaving together the zinc system",
	Run: func(cmd *cobra.Command, args []string) {
		if version, _ := cmd.Flags().GetBool("version"); version {
			fmt.Println("1.5")
			return
		} else {
			cmd.HelpFunc()(cmd, args)
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("version", "v", false, "prints version of the application")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "c", "config file (default is $XDG_CONFIG_DIR/config.yaml)")
}

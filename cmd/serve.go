package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zinc-sig/webhook/pkg/app"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "serves the webhook server",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("==> Starting ZINC webhook service")
		debug, err := cmd.Flags().GetBool("dev")
		if err != nil {
			panic(fmt.Errorf("failed to get dev flag: %v", err))
		}
		app.New(app.Options{
			ConfigPath: configPath,
			Debug:      debug,
		}).Run()
	},
}

func init() {
	serveCmd.Flags().BoolP("dev", "", false, "Run in development mode")
	rootCmd.AddCommand(serveCmd)
}

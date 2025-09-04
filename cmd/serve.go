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
		fmt.Println("==> Starting ZINC webhook")
		app.New().Run()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

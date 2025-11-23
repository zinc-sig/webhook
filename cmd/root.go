package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "webhook",
	Short: "a server program for processing and weaving together the zinc system",
	Run: func(cmd *cobra.Command, args []string) {
		if version, _ := cmd.Flags().GetBool("version"); version {
			platform := fmt.Sprintf("%v %v", runtime.GOOS, runtime.GOARCH)
			buildDate := func() string {
				if info, ok := debug.ReadBuildInfo(); ok {
					for _, setting := range info.Settings {
						if setting.Key == "vcs.time" {
							d, err := time.Parse(time.RFC3339, setting.Value)
							if err != nil {
								return "unknown"
							}
							return fmt.Sprintf("v%s", d.Format("06.01"))
						}
					}
				}

				return "n/a"
			}()
			commitHash := func() string {
				if info, ok := debug.ReadBuildInfo(); ok {
					for _, setting := range info.Settings {
						if setting.Key == "vcs.revision" {
							return setting.Value[0:7]
						}
					}
				}

				return "n/a"
			}()
			fmt.Printf("%s %s (%s)\n%s", cmd.Use, buildDate, commitHash, platform)
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

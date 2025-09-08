package main

import (
	"embed"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/spf13/cobra"
)

//go:embed assets/*
var assets embed.FS

func ApplyMigrations(dsn string) error {
	dir, err := iofs.New(assets, "assets")
	if err != nil {
		return fmt.Errorf("failed to create iofs instance: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", dir, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "run database schema migrations against database",
	Run: func(cmd *cobra.Command, args []string) {
		dsn := cmd.Flag("dsn").Value.String()
		if dsn == "" {
			slog.Error("a valid DSN is required to perform migrations")
			os.Exit(1)
		}
		if err := ApplyMigrations(dsn); err != nil {
			panic(err)
		}
		slog.Info("SQL migrations has been successfully applied")
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().String("dsn", os.Getenv("DSN"), "Database connection string")
}

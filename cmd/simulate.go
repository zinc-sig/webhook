package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/narwhl/mockestra"
	"github.com/narwhl/mockestra/postgres"
	"github.com/narwhl/mockestra/redis"
	"github.com/spf13/cobra"
	"github.com/testcontainers/testcontainers-go"
	"github.com/zinc-sig/webhook/pkg/mock"
	"go.uber.org/fx"
)

func NewSimulation() *fx.App {
	defaultModules := []fx.Option{
		fx.NopLogger,
		fx.Supply(
			fx.Annotate(
				fmt.Sprintf("%x", time.Now().Unix()),
				fx.ResultTags(`name:"prefix"`),
			),
		),
		postgres.Module(
			postgres.WithPassword("zinc"),
			postgres.WithUsername("zinc"),
			postgres.WithDatabase("zinc"),
			postgres.WithMigration(func(dsn string) error {
				return ApplyMigrations(dsn)
			}),
		),
		redis.Module(),
		mock.HasuraModule(
			mock.WithAdminSecret("zincsecret"),
		),
		fx.Invoke(func(p struct {
			fx.In
			Lifecycle  fx.Lifecycle
			Containers []testcontainers.Container `group:"containers"`
		}) {
			p.Lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					slog.Info("ZINC Simulation Started")
					return nil
				},
				OnStop: func(ctx context.Context) error {
					slog.Info("Stopping ZINC Simulation...")
					return nil
				},
			})
		}),
	}

	defaultModules = append(defaultModules, mockestra.Versions(map[string]string{
		"postgres": "17-alpine",
		"redis":    "8-alpine",
		"hasura":   "v2.48.0",
	})...)

	return fx.New(defaultModules...)
}

var simulateCmd = &cobra.Command{
	Use:   "simulate",
	Short: "simulates the stack",
	Run: func(cmd *cobra.Command, args []string) {
		app := NewSimulation()
		app.Run()
	},
}

func init() {
	rootCmd.AddCommand(simulateCmd)
}

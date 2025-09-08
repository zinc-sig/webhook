package main_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/narwhl/mockestra/postgres"
	"github.com/testcontainers/testcontainers-go"
	main "github.com/zinc-sig/webhook/cmd"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestApplyMigrations(t *testing.T) {
	pgUserPassword := "testpassword"
	app := fxtest.New(
		t,
		fx.NopLogger,
		fx.Supply(
			fx.Annotate(
				"latest",
				fx.ResultTags(`name:"postgres_version"`),
			),
			fx.Annotate(
				fmt.Sprintf("migration-test-%x", time.Now().Unix()),
				fx.ResultTags(`name:"prefix"`),
			),
		),
		postgres.Module(
			postgres.WithUsername("zinc"),
			postgres.WithDatabase("zinc"),
			postgres.WithPassword(pgUserPassword),
		),
		fx.Invoke(func(params struct {
			fx.In
			Container testcontainers.Container `name:"postgres"`
		}) {
			endpoint, err := params.Container.PortEndpoint(t.Context(), postgres.Port, "")
			if err != nil {
				t.Fatalf("failed to get postgres endpoint: %v", err)
			}
			dsn := fmt.Sprintf(
				"postgres://zinc:%s@%s/%s?sslmode=disable",
				pgUserPassword, endpoint, "zinc",
			)
			t.Run("ApplyMigrations", func(t *testing.T) {
				err = main.ApplyMigrations(dsn)
				if err != nil {
					t.Errorf("failed to apply migrations: %v", err)
				}
			})
			t.Run("CheckSchemaAndTables", func(t *testing.T) {
				conn, err := pgx.Connect(t.Context(), dsn)
				if err != nil {
					t.Fatalf("failed to connect to postgres: %v", err)
				}
				query := `
				SELECT table_schema, table_name
				FROM information_schema.tables
				WHERE table_type = 'BASE TABLE'
				ORDER BY table_schema, table_name;
				`

				rows, err := conn.Query(t.Context(), query)
				if err != nil {
					t.Errorf("Query failed: %v\n", err)
				}
				defer rows.Close()

				schemaTables := make(map[string][]string)
				for rows.Next() {
					var schema, table string
					err := rows.Scan(&schema, &table)
					if err != nil {
						t.Errorf("Row scan failed: %v\n", err)
					}
					if slices.Contains([]string{"public"}, schema) {
						schemaTables[schema] = append(schemaTables[schema], table)
					}
				}

				for schema, tables := range schemaTables {
					t.Logf("Schema: %s\n", schema)
					for _, table := range tables {
						t.Logf("  Table: %s\n", table)
					}
				}
				t.Cleanup(func() {
					conn.Close(t.Context())
				})
			})
		}),
	)
	app.RequireStart()
	t.Cleanup(app.RequireStop)
}

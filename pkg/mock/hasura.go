package mock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/narwhl/mockestra"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zinc-sig/webhook/pkg/api"
	"go.uber.org/fx"
)

const (
	Port  = "8080/tcp"
	Tag   = "hasura"
	Image = "hasura/graphql-engine"
)

func WithPostgres(dsn string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		req.Env["HASURA_GRAPHQL_DATABASE_URL"] = dsn
		req.Env["HASURA_GRAPHQL_METADATA_DATABASE_URL"] = dsn
		req.Env["PG_DATABASE_URL"] = dsn
		return nil
	}
}

func WithAdminSecret(secret string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		req.Env["HASURA_GRAPHQL_ADMIN_SECRET"] = secret
		return nil
	}
}

type RequestParams struct {
	fx.In
	Prefix  string                               `name:"prefix"`
	Version string                               `name:"hasura_version"`
	Opts    []testcontainers.ContainerCustomizer `group:"hasura"`
}

func New(p RequestParams) (*testcontainers.GenericContainerRequest, error) {

	r := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Name:            fmt.Sprintf("mock-%s-%s", p.Prefix, Tag),
			Image:           fmt.Sprintf("%s:%s", Image, p.Version),
			ExposedPorts:    []string{Port},
			HostAccessPorts: []int{api.Port},
			WaitingFor:      wait.ForHTTP("/healthz").WithPort(Port),
			Env: map[string]string{
				"HASURA_GRAPHQL_VERSION":        "3",
				"HASURA_GRAPHQL_ENABLE_CONSOLE": "true",
				"HASURA_GRAPHQL_AUTH_HOOK":      fmt.Sprintf("http://%s:%d/identity", testcontainers.HostInternal, api.Port),
				"HASURA_GRAPHQL_AUTH_HOOK_MODE": "POST",
			},
		},
		Started: true,
	}

	for _, opt := range p.Opts {
		if err := opt.Customize(&r); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

type ContainerParams struct {
	fx.In
	Lifecycle                fx.Lifecycle
	Request                  *testcontainers.GenericContainerRequest `name:"hasura"`
	PostgresContainerRequest *testcontainers.GenericContainerRequest `name:"postgres"`
	PostgresContainer        testcontainers.Container                `name:"postgres"`
}

type Result struct {
	fx.Out
	Container      testcontainers.Container `name:"hasura"`
	ContainerGroup testcontainers.Container `group:"containers"`
}

func Actualize(p ContainerParams) (Result, error) {
	postgresIP, err := p.PostgresContainer.ContainerIP(context.Background())
	if err != nil {
		return Result{}, fmt.Errorf("failed to get Postgres container IP: %w", err)
	}
	WithPostgres(
		fmt.Sprintf(
			"postgres://%s:%s@%s/%s",
			p.PostgresContainerRequest.Env["POSTGRES_USER"],
			p.PostgresContainerRequest.Env["POSTGRES_PASSWORD"],
			postgresIP,
			p.PostgresContainerRequest.Env["POSTGRES_DB"],
		),
	).Customize(p.Request)
	c, err := testcontainers.GenericContainer(context.Background(), *p.Request)
	if err != nil {
		return Result{}, fmt.Errorf("an error occurred while instantiating Hasura container: %w", err)
	}

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			hasuraPort, err := c.MappedPort(ctx, Port)
			if err != nil {
				return fmt.Errorf("unable to get %s port: %w", Tag, err)
			}

			slog.Info(
				fmt.Sprintf("%s container is running", Tag),
				"addr", fmt.Sprintf("localhost:%s", hasuraPort.Port()),
			)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			err := c.Terminate(ctx)
			if err != nil {
				slog.Warn(fmt.Sprintf("an error occurred while terminating %s container", Tag), "error", err)
			} else {
				slog.Info(fmt.Sprintf("%s container is terminated", Tag))
			}
			return err
		},
	})
	return Result{
		Container:      c,
		ContainerGroup: c,
	}, nil
}

var HasuraModule = mockestra.BuildContainerModule(
	Tag,
	fx.Provide(
		fx.Annotate(
			New,
			fx.ResultTags(`name:"hasura"`),
		),
		Actualize,
	),
)

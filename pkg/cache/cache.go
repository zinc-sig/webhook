package cache

import (
	"context"
	"encoding/json"
	"log/slog"

	"go.uber.org/fx"
)

type Config struct {
	DSN string `mapstructure:"dsn" yaml:"dsn"`
}

const (
	QueueKey = "grader:queues"
)

var Module = fx.Module(
	"cache",
	fx.Provide(
		NewService,
	),
	fx.Invoke(func(lifecycle fx.Lifecycle, cache Service) {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				cache.RegisterHandler("doneGrading", func(ctx context.Context, jobType, queue string, payload json.RawMessage) error {
					var payloadJson string
					if err := json.Unmarshal(payload, &payloadJson); err != nil {
						slog.Warn("Failed to unmarshal payload", "error", err)
						return err
					}
					var data DoneGradingPayload
					if err := json.Unmarshal([]byte(payloadJson), &data); err != nil {
						slog.Warn("Failed to unmarshal payload", "error", err)
						return err
					}
					slog.Info("Processing job", "type", jobType, "queue", queue, "payload", data, "is_batch", len(data.Reports) > 1)
					
					// Log successful processing
					var reportIDs []int
					for _, report := range data.Reports {
						reportIDs = append(reportIDs, report.ID)
					}
					slog.Info("doneGrading processed successfully", "reportIDs", reportIDs, "reportCount", len(data.Reports), "queue", queue)
					return nil
				})
				go cache.Subscribe(context.Background())
				return nil
			},
		})
	}),
)

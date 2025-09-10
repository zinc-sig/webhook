package cache

import (
	"context"
	"encoding/json"

	"go.uber.org/fx"
	"go.uber.org/zap"
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
	fx.Invoke(func(lifecycle fx.Lifecycle, cache Service, logger *zap.SugaredLogger) {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				cache.RegisterHandler("doneGrading", func(ctx context.Context, jobType, queue string, payload json.RawMessage) error {
					var payloadJson string
					if err := json.Unmarshal(payload, &payloadJson); err != nil {
						logger.Warnw("Failed to unmarshal payload", "context", ctx, "error", err)
						return err
					}
					var data DoneGradingPayload
					if err := json.Unmarshal([]byte(payloadJson), &data); err != nil {
						logger.Warnw("Failed to unmarshal payload", "context", ctx, "error", err)
						return err
					}
					logger.Infow("Processing job", "context", ctx, "type", jobType, "queue", queue, "payload", data, "is_batch", len(data.Reports) > 1)

					// Log successful processing
					var reportIDs []int
					for _, report := range data.Reports {
						reportIDs = append(reportIDs, report.ID)
					}
					logger.Infow("doneGrading processed successfully", "context", ctx, "reportIDs", reportIDs, "reportCount", len(data.Reports), "queue", queue)
					return nil
				})
				go cache.Subscribe(context.Background())
				return nil
			},
		})
	}),
)

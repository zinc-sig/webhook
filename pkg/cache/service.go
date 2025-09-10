package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type MessageHandler func(ctx context.Context, jobType string, queue string, payload json.RawMessage) error

type JobMessage struct {
	Job     string          `json:"job"`
	Payload json.RawMessage `json:"payload"`
}

type DoneGradingPayload struct {
	Reports []struct {
		ID           int `json:"id"`
		SubmissionID int `json:"submission_id"`
	} `json:"reports"`
}

func (s *service) processMessage(ctx context.Context, queue, rawMessage string) {
	ctx, span := s.tracer.Start(ctx, "cache.processMessage",
		trace.WithAttributes(
			attribute.String("queue", queue),
			attribute.Int("message_size", len(rawMessage)),
		))
	defer span.End()

	s.logger.Infow("Received message", "context", ctx, "msg", rawMessage, "queue", queue)

	var msg JobMessage
	if err := json.Unmarshal([]byte(rawMessage), &msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal message")
		s.logger.Warnw("Failed to unmarshal message", "context", ctx, "error", err)
		return
	}

	span.SetAttributes(attribute.String("job_type", msg.Job))

	// Look up the handler for this job type
	handler, exists := s.handlers[msg.Job]
	if !exists {
		span.SetStatus(codes.Error, "no handler registered")
		s.logger.Warnw("No handler registered", "context", ctx, "job_type", msg.Job)
		return
	}

	// Execute the callback
	if err := handler(ctx, msg.Job, queue, msg.Payload); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "handler error")
		s.logger.Warnw("Handler error", "context", ctx, "job", msg.Job, "error", err)
	} else {
		span.SetAttributes(attribute.String("status", "success"))
	}
}

type Service interface {
	Put(ctx context.Context, key string, value []byte, expiry time.Duration) error
	Read(ctx context.Context, key string) ([]byte, error)
	Remove(ctx context.Context, key string) error
	Publish(ctx context.Context, channel string, message []byte) error
	LoadBalancePublish(ctx context.Context, channels []string, message []byte) error
	Llen(ctx context.Context, channel string) (int64, error)
	Subscribe(ctx context.Context) error
	RegisterHandler(jobType string, handler MessageHandler)
}

type service struct {
	client   *redis.Client
	locker   sync.Mutex
	handlers map[string]MessageHandler
	tracer   trace.Tracer
	logger   *zap.SugaredLogger
}

type ServiceParams struct {
	fx.In
	Config *Config
	Logger *zap.SugaredLogger
}

func NewService(p ServiceParams) Service {
	client := redis.NewClient(&redis.Options{
		Addr:     p.Config.DSN, // e.g., "localhost:6379"
		Password: "",           // no password set
		DB:       0,            // use default DB
	})
	return &service{
		client:   client,
		handlers: make(map[string]MessageHandler),
		tracer:   otel.Tracer("webhook.cache"),
		logger:   p.Logger,
	}
}

func (s *service) Put(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	ctx, span := s.tracer.Start(ctx, "cache.Put",
		trace.WithAttributes(
			attribute.String("key", key),
			attribute.Int("value_size", len(value)),
			attribute.Float64("expiry_seconds", expiry.Seconds()),
		))
	defer span.End()

	if err := s.client.Set(ctx, key, value, expiry).Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set key")
		return err
	}
	return nil
}

func (s *service) Read(ctx context.Context, key string) ([]byte, error) {
	ctx, span := s.tracer.Start(ctx, "cache.Read",
		trace.WithAttributes(
			attribute.String("key", key),
		))
	defer span.End()

	result, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		span.SetAttributes(attribute.Bool("key_exists", false))
		s.logger.Warnw("Key does not exist", "context", ctx, "key", key)
		return nil, nil // Key does not exist
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "error reading key")
		s.logger.Warnw("Error reading key", "context", ctx, "key", key, "error", err)
		return nil, err
	}
	span.SetAttributes(
		attribute.Bool("key_exists", true),
		attribute.Int("value_size", len(result)),
	)
	return result, nil
}

func (s *service) Remove(ctx context.Context, key string) error {
	ctx, span := s.tracer.Start(ctx, "cache.Remove",
		trace.WithAttributes(
			attribute.String("key", key),
		))
	defer span.End()

	if err := s.client.Del(ctx, key).Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete key")
		return err
	}
	return nil
}

func (s *service) Llen(ctx context.Context, channel string) (int64, error) {
	ctx, span := s.tracer.Start(ctx, "cache.Llen",
		trace.WithAttributes(
			attribute.String("channel", channel),
		))
	defer span.End()

	result, err := s.client.LLen(ctx, channel).Result()
	if err == redis.Nil {
		span.SetAttributes(attribute.Bool("channel_exists", false))
		s.logger.Warnw("Channel does not exist", "context", ctx, "channel", channel)
		return 0, nil // Channel does not exist
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "error reading channel")
		s.logger.Warnw("Error reading channel", "context", ctx, "channel", channel, "error", err)
		return 0, err
	}
	span.SetAttributes(
		attribute.Bool("channel_exists", true),
		attribute.Int64("queue_length", result),
	)
	return result, nil
}

func (s *service) Publish(ctx context.Context, channel string, message []byte) error {
	ctx, span := s.tracer.Start(ctx, "cache.Publish",
		trace.WithAttributes(
			attribute.String("channel", channel),
			attribute.Int("message_size", len(message)),
		))
	defer span.End()

	if err := s.client.RPush(ctx, channel, message).Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to publish message")
		return err
	}
	return nil
}

func (s *service) LoadBalancePublish(ctx context.Context, channels []string, message []byte) error {
	ctx, span := s.tracer.Start(ctx, "cache.LoadBalancePublish",
		trace.WithAttributes(
			attribute.StringSlice("channels", channels),
			attribute.Int("message_size", len(message)),
		))
	defer span.End()

	s.locker.Lock()
	defer s.locker.Unlock()

	// Find the channel with the least number of messages
	var targetChannel string
	minLen := int64(-1)
	for _, channel := range channels {
		length, err := s.Llen(ctx, channel)
		if err != nil {
			s.logger.Warnw("Failed to get length of channel", "context", ctx, "channel", channel, "error", err)
			continue
		}
		if minLen == -1 || length < minLen {
			minLen = length
			targetChannel = channel
		}
	}

	if minLen < 0 {
		span.SetStatus(codes.Error, "failed to determine target channel")
		return fmt.Errorf("failed to determine target channel for load balancing")
	}
	span.SetAttributes(
		attribute.String("target_channel", targetChannel),
		attribute.Int64("queue_length", minLen),
	)
	s.logger.Infow("Publishing to channel", "context", ctx, "channel", targetChannel, "length", minLen)
	err := s.client.RPush(ctx, targetChannel, message).Err()
	if err == nil {
		s.logger.Infow("successfully published to queue", "context", ctx, "targetChannel", targetChannel, "queueLength", minLen+1, "messageSize", len(message))
		span.SetAttributes(attribute.String("status", "success"))
	} else {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to publish to queue")
	}
	return err
}

func (s *service) Subscribe(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "cache.Subscribe")
	defer span.End()

	for {
		select {
		case <-ctx.Done():
			s.logger.Infow("Context cancelled, stopping consumer", "context", ctx)
			span.SetAttributes(attribute.String("stop_reason", "context_cancelled"))
			return ctx.Err()
		default:
			// Block for up to 5 seconds waiting for a message
			data, err := s.Read(ctx, QueueKey)
			if err != nil {
				s.logger.Warnw("Failed to read grader queues from cache", "context", ctx, "error", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if data == nil {
				s.logger.Infow("No grader queues configured, waiting...", "context", ctx)
				time.Sleep(5 * time.Second)
				continue
			}
			var queues []string
			for _, queue := range strings.Split(string(data), ",") {
				queues = append(queues, fmt.Sprintf("%s:api", queue))
			}

			for _, queue := range queues {
				result, err := s.client.BRPop(ctx, 5*time.Second, queue).Result()
				if err != nil {
					if err == redis.Nil {
						// No message available, continue polling
						continue
					}
					s.logger.Warnw("Error reading from queue", "context", ctx, "error", err)
					time.Sleep(1 * time.Second)
					continue
				}

				// result[0] is the queue name, result[1] is the message
				if len(result) != 2 {
					continue
				}

				// Process the message in a goroutine for non-blocking processing
				go s.processMessage(ctx, queue, result[1])
			}
		}
	}
}

func (s *service) RegisterHandler(jobType string, handler MessageHandler) {
	// Note: This doesn't need a span as it's just a registration operation during startup
	s.handlers[jobType] = handler
}

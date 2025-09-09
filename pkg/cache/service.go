package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/fx"
)

type MessageHandler func(ctx context.Context, jobType string, payload json.RawMessage) error

type JobMessage struct {
	Job     string          `json:"job"`
	Payload json.RawMessage `json:"payload"`
}

func (s *service) processMessage(ctx context.Context, queue, rawMessage string) {
	slog.Info("Received message: %s", "msg", rawMessage, "queue", queue)

	var msg JobMessage
	if err := json.Unmarshal([]byte(rawMessage), &msg); err != nil {
		slog.Warn("Failed to unmarshal message", "error", err)
		return
	}

	// Look up the handler for this job type
	handler, exists := s.handlers[msg.Job]
	if !exists {
		slog.Warn("No handler registered for: %s", "job type", msg.Job)
		return
	}

	// Execute the callback
	if err := handler(ctx, msg.Job, msg.Payload); err != nil {
		slog.Warn("Handler error for job %s", "job", msg.Job, "error", err)
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
}

type ServiceParams struct {
	fx.In
	Config *Config
}

func NewService(p ServiceParams) Service {
	client := redis.NewClient(&redis.Options{
		Addr:     p.Config.DSN, // e.g., "localhost:6379"
		Password: "",           // no password set
		DB:       0,            // use default DB
	})
	return &service{
		client: client,
	}
}

func (s *service) Put(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	return s.client.Set(ctx, key, value, expiry).Err()
}

func (s *service) Read(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		slog.Warn("Key does not exist", "key", key)
		return nil, nil // Key does not exist
	}
	if err != nil {
		slog.Warn("Error reading key", "key", key, "error", err)
		return nil, err
	}
	return result, nil
}

func (s *service) Remove(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func (s *service) Llen(ctx context.Context, channel string) (int64, error) {
	result, err := s.client.LLen(ctx, channel).Result()
	if err == redis.Nil {
		slog.Warn("Channel does not exist", "channel", channel)
		return 0, nil // Channel does not exist
	}
	if err != nil {
		slog.Warn("Error reading channel", "channel", channel, "error", err)
		return 0, err
	}
	return result, nil
}

func (s *service) Publish(ctx context.Context, channel string, message []byte) error {
	return s.client.RPush(ctx, channel, message).Err()
}

func (s *service) LoadBalancePublish(ctx context.Context, channels []string, message []byte) error {

	s.locker.Lock()
	defer s.locker.Unlock()

	// Find the channel with the least number of messages
	var targetChannel string
	minLen := int64(-1)
	for _, channel := range channels {
		length, err := s.Llen(ctx, channel)
		if err != nil {
			slog.Warn("Failed to get length of channel", "channel", channel, "error", err)
			continue
		}
		if minLen == -1 || length < minLen {
			minLen = length
			targetChannel = channel
		}
	}

	if minLen < 0 {
		return fmt.Errorf("failed to determine target channel for load balancing")
	}
	slog.Info("Publishing to channel: %s with %d messages", "channel", targetChannel, "length", minLen)
	return s.client.RPush(ctx, targetChannel, message).Err()
}

func (s *service) Subscribe(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("Context cancelled, stopping consumer")
			return ctx.Err()
		default:
			// Block for up to 5 seconds waiting for a message
			data, err := s.Read(ctx, QueueKey)
			if err != nil {
				slog.Warn("Failed to read grader queues from cache", "error", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if data == nil {
				slog.Info("No grader queues configured, waiting...")
				time.Sleep(5 * time.Second)
				continue
			}
			var queues []string
			for _, queue := range strings.Split(string(data), ",") {
				queues = append(queues, fmt.Sprintf("%s:grader", queue))
			}

			for _, queue := range queues {
				result, err := s.client.BRPop(ctx, 5*time.Second, queue).Result()
				if err != nil {
					if err == redis.Nil {
						// No message available, continue polling
						continue
					}
					slog.Warn("Error reading from queue: %v", "error", err)
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
	s.handlers[jobType] = handler
}

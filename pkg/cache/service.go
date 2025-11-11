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
	"github.com/google/uuid"
	"go.uber.org/fx"
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

type ConfigValidationRequest struct {
	ID         string `json:"id"`
	ConfigYAML string `json:"config_yaml"`
}

type ConfigValidationResponse struct {
	ID          string  `json:"id"`
	ConfigError *string `json:"configError"`
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
	if err := handler(ctx, msg.Job, queue, msg.Payload); err != nil {
		slog.Warn("Handler error for job %s", "job", msg.Job, "error", err)
	}
}

type Service interface {
	Put(ctx context.Context, key string, value []byte, expiry time.Duration) error
	Read(ctx context.Context, key string) ([]byte, error)
	Remove(ctx context.Context, key string) error
	Publish(ctx context.Context, channel string, message []byte) error
	Llen(ctx context.Context, channel string) (int64, error)
	LoadBalanceGraderPublish(ctx context.Context, message []byte, njobs int) error
	LoadBalanceGraderDequeue(ctx context.Context, channel string, njobs int) error
	ValidateConfig(ctx context.Context, configYAML string) (*ConfigValidationResponse, error)
	Subscribe(ctx context.Context) error
	RegisterHandler(jobType string, handler MessageHandler)
}

type service struct {
	client   *redis.Client
	debug    bool
	locker   sync.Mutex
	handlers map[string]MessageHandler
}

type ServiceParams struct {
	fx.In
	Config *Config
	Debug  bool `name:"debug"`
}

func NewService(p ServiceParams) Service {
	client := redis.NewClient(&redis.Options{
		Addr:     p.Config.DSN, // e.g., "localhost:6379"
		Password: "",           // no password set
		DB:       0,            // use default DB
	})
	return &service{
		client:   client,
		debug:    p.Debug,
		handlers: make(map[string]MessageHandler),
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

func (s *service) LoadBalanceGraderPublish(ctx context.Context, message []byte, njobs int) error {

	s.locker.Lock()
	defer s.locker.Unlock()

	data, err := s.Read(ctx, QueueKey)
	if err != nil {
		slog.Warn("Failed to read grader queues", "error", err)
		return fmt.Errorf("failed to read grader queues: %s", err.Error())
	}
	if data == nil {
		return fmt.Errorf("no grader queues configured")
	}

	channels := strings.Split(string(data), ",")

	// Transaction for atomicity on getting lengths
	trans := s.client.TxPipeline()
	for _, channel := range channels {
		trans.Get(ctx, fmt.Sprintf("%s:length", channel))
	}
	cmds, err := trans.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get lengths of channels: %s", err.Error())
	}

	// Find the channel with the least number of messages
	var targetChannel string
	minLen := -1

	for idx, channel := range channels {
		length, err := cmds[idx].(*redis.StringCmd).Int()
		if err == redis.Nil {
			length = 0 // Channel does not exist, treat as empty
		} else if err != nil {
			slog.Warn("Failed to get value of of key", "key", fmt.Sprintf("%s:length", channel), "error", err)
			continue
		}
		if minLen == -1 || length < minLen {
			minLen = length
			targetChannel = channel
		}
	}

	targetGraderChannel := fmt.Sprintf("%s:grader", targetChannel)
	if minLen < 0 {
		return fmt.Errorf("failed to determine target channel for load balancing")
	}
	slog.Info("Publishing to channel: %s with %d messages", "channel", targetGraderChannel, "length", minLen)

	trans = s.client.TxPipeline()
	trans.RPush(ctx, targetGraderChannel, message)
	trans.IncrBy(ctx, fmt.Sprintf("%s:length", targetChannel), int64(njobs))

	_, err = trans.Exec(ctx)
	if err == nil {
		slog.Info("successfully published to queue", "queue", targetGraderChannel, "queueLength", minLen+njobs, "messageSize", len(message))
	}
	return err
}

func (s *service) LoadBalanceGraderDequeue(ctx context.Context, channel string, njobs int) error {
	s.locker.Lock()
	defer s.locker.Unlock()

	err := s.client.DecrBy(ctx, fmt.Sprintf("%s:length", channel), int64(njobs)).Err()
	if err != nil {
		return err
	}

	curr_len, err := s.client.Get(ctx, fmt.Sprintf("%s:length", channel)).Int()
	if err != nil {
		return err
	}
	if curr_len < 0 {
		// Reset to zero if it goes negative
		slog.Warn("Channel length went negative, resetting to zero", "channel", channel)
		return s.client.Set(ctx, fmt.Sprintf("%s:length", channel), 0, 0).Err()
	}
	return nil
}

func (s *service) Subscribe(ctx context.Context) error {
	slog.Info("Starting cache subscriber...")
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
			queues := strings.Split(string(data), ",")

			for _, queue := range queues {
				result, err := s.client.BRPop(ctx, 5*time.Second, fmt.Sprintf("%s:api", queue)).Result()
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

func (s *service) ValidateConfig(ctx context.Context, configYAML string) (*ConfigValidationResponse, error) {
	id := uuid.New()

	// Create the validation request
	validationReq := ConfigValidationRequest{
		ID:         id.String(),
		ConfigYAML: configYAML,
	}

	reqBytes, err := json.Marshal(validationReq)
	if err != nil {
		slog.Warn("Failed to marshal validation request", "error", err)
		return nil, fmt.Errorf("failed to marshal validation request: %w", err)
	}

	// For debug mode, skip actual publishing and return a mock response
	if s.debug {
		slog.Info("Publishing config validation request", "request", validationReq)
		return &ConfigValidationResponse{
			ID:          id.String(),
			ConfigError: nil,
		}, nil
	}

	// Publish the validation request to the validateConfig channel
	if err := s.client.Publish(ctx, "validateConfig", reqBytes).Err(); err != nil {
		return nil, err
	}
	subscription := s.client.Subscribe(ctx, "configValidated")
	defer subscription.Close()

	ch := subscription.Channel()
	var result ConfigValidationResponse
	for msg := range ch {
		var payload ConfigValidationResponse

		if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
			slog.Warn("Error parsing message:", "err", err)
			continue // Skip malformed messages
		}

		if payload.ID == id.String() {
			if err := subscription.Unsubscribe(ctx, "configValidated"); err != nil {
				slog.Warn("Error unsubscribing from topic:", "err", err)
				return nil, err
			}
			result = payload
			break
		}
	}
	return &result, nil
}

func (s *service) RegisterHandler(jobType string, handler MessageHandler) {
	s.handlers[jobType] = handler
}

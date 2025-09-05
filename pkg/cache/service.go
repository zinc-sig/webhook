package cache

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
)

type MessageHandler func(ctx context.Context, jobType string, payload json.RawMessage) error

type JobMessage struct {
	Job     string          `json:"job"`
	Payload json.RawMessage `json:"payload"`
}

func (s *service) processMessage(ctx context.Context, queue, rawMessage string) {
	log.Printf("Received message: %s", rawMessage)

	var msg JobMessage
	if err := json.Unmarshal([]byte(rawMessage), &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	// Look up the handler for this job type
	handler, exists := s.handlers[msg.Job]
	if !exists {
		log.Printf("No handler registered for job type: %s", msg.Job)
		return
	}

	// Execute the callback
	if err := handler(ctx, msg.Job, msg.Payload); err != nil {
		log.Printf("Handler error for job %s: %v", msg.Job, err)
	}
}

type Service interface {
	Put(ctx context.Context, key string, value []byte, expiry time.Duration) error
	Read(ctx context.Context, key string) ([]byte, error)
	Remove(ctx context.Context, key string) error
	Publish(ctx context.Context, channel string, message []byte) error
	Subscribe(ctx context.Context) error
	RegisterHandler(jobType string, handler MessageHandler)
}

type service struct {
	client   *redis.Client
	queues   []string
	handlers map[string]MessageHandler
}

func NewService() Service {
	client := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: "", // no password set
		DB:       0,  // use default DB
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
		return nil, nil // Key does not exist
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *service) Remove(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func (s *service) Publish(ctx context.Context, channel string, message []byte) error {
	return s.client.RPush(ctx, channel, message).Err()
}

func (s *service) Subscribe(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("Context cancelled, stopping consumer")
			return ctx.Err()
		default:
			// Block for up to 5 seconds waiting for a message
			for _, queue := range s.queues {
				result, err := s.client.BRPop(ctx, 5*time.Second, queue).Result()
				if err != nil {
					if err == redis.Nil {
						// No message available, continue polling
						continue
					}
					log.Printf("Error reading from queue: %v", err)
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

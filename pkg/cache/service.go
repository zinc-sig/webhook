package cache

import (
	"context"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
)

type Service interface {
	Put(ctx context.Context, key string, value []byte, expiry time.Duration) error
	Read(ctx context.Context, key string) ([]byte, error)
	Remove(ctx context.Context, key string) error
}

type service struct {
	client *redis.Client
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

package cache_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/mock"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestLlen(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		app := fxtest.New(
			t,
			mock.ProvideMockCacheService(t),
			fx.Invoke(
				func(cache cache.Service) {
					err := cache.Publish(t.Context(), "test", []byte("hello1"))
					assert.NoError(t, err, "failed to publish message")

					err = cache.Publish(t.Context(), "test", []byte("hello2"))
					assert.NoError(t, err, "failed to publish message")

					length, err := cache.Llen(t.Context(), "test")
					assert.NoError(t, err, "failed to get list length")

					assert.Equal(t, int64(2), length)
				},
			),
		)

		app.RequireStart()
		t.Cleanup(app.RequireStop)
	})
}

func TestLoadBalancePublish(t *testing.T) {
	t.Run("logic success", func(t *testing.T) {
		app := fxtest.New(
			t,
			mock.ProvideMockCacheService(t),
			fx.Invoke(
				func(cache cache.Service, client *redis.Client) {
					jobs := []int{1, 3, 5, 1, 10, 1, 3}
					channels := []string{"channel1", "channel2", "channel3"}

					for idx, njobs := range jobs {
						err := cache.LoadBalancePublish(t.Context(), channels, []byte(fmt.Sprintf("test%d", idx)), njobs)
						assert.NoError(t, err, "failed to load balance publish")
					}

					expected := map[string][]string{
						"channel1": {"test0", "test3", "test4"}, // 1+1+10 jobs
						"channel2": {"test1", "test5", "test6"}, // 3+1+3 jobs
						"channel3": {"test2"},                   // 5 jobs
					}

					for _, channel := range channels {
						content, err := client.LRange(t.Context(), channel, 0, -1).Result()
						assert.NoError(t, err, "failed to read channel content")
						assert.ElementsMatchf(t, content, expected[channel], "channel content does not match for channel %s", channel)
					}
				},
			),
		)

		app.RequireStart()
		t.Cleanup(app.RequireStop)
	})

	t.Run("race condition success", func(t *testing.T) {
		app := fxtest.New(
			t,
			mock.ProvideMockCacheService(t),
			fx.Invoke(
				func(cache cache.Service, client *redis.Client) {
					var wg sync.WaitGroup

					channels := []string{"channel1", "channel2", "channel3"}
					for range 99 {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := cache.LoadBalancePublish(t.Context(), channels, []byte("hello"), 3)
							assert.NoError(t, err, "failed to load balance publish")
						}()
					}
					wg.Wait()

					// Check that messages are distributed across channels
					length1, err := client.LLen(t.Context(), "channel1").Result()
					assert.NoError(t, err, "failed to get list length for channel1")

					length2, err := client.LLen(t.Context(), "channel2").Result()
					assert.NoError(t, err, "failed to get list length for channel2")

					length3, err := client.LLen(t.Context(), "channel3").Result()
					assert.NoError(t, err, "failed to get list length for channel3")

					assert.Equal(t, int64(33), length1)
					assert.Equal(t, int64(33), length2)
					assert.Equal(t, int64(33), length3)

				},
			),
		)

		app.RequireStart()
		t.Cleanup(app.RequireStop)
	})
}

func TestLoadBalanceDequeue(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		app := fxtest.New(
			t,
			mock.ProvideMockCacheService(t),
			fx.Invoke(
				func(cache cache.Service, client *redis.Client) {
					channels := []string{"channel1", "channel2", "channel3"}

					// Pre-fill channels with lengths
					for i, channel := range channels {
						err := client.Set(t.Context(), fmt.Sprintf("%s:length", channel), (i+1)*10, 0).Err()
						assert.NoError(t, err, "failed to set initial length")
					}

					// Dequeue jobs
					err := cache.LoadBalanceDequeue(t.Context(), "channel1", 5)
					assert.NoError(t, err, "failed to load balance dequeue")

					err = cache.LoadBalanceDequeue(t.Context(), "channel2", 15)
					assert.NoError(t, err, "failed to load balance dequeue")

					err = cache.LoadBalanceDequeue(t.Context(), "channel3", 8)
					assert.NoError(t, err, "failed to load balance dequeue")

					// Check final lengths
					length1, err := client.Get(t.Context(), "channel1:length").Int()
					assert.NoError(t, err, "failed to get length for channel1")
					assert.Equal(t, 5, length1)

					length2, err := client.Get(t.Context(), "channel2:length").Int()
					assert.NoError(t, err, "failed to get length for channel2")
					assert.Equal(t, 5, length2)

					length3, err := client.Get(t.Context(), "channel3:length").Int()
					assert.NoError(t, err, "failed to get length for channel3")
					assert.Equal(t, 22, length3)
				},
			),
		)

		app.RequireStart()
		t.Cleanup(app.RequireStop)
	})
}

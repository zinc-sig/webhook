package cache_test

import (
	"sync"
	"testing"

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
				func(cache cache.Service) {
					channels := []string{"channel1", "channel2", "channel3"}
					err := cache.LoadBalancePublish(t.Context(), channels, []byte("hello1"))
					assert.NoError(t, err, "failed to load balance publish")

					err = cache.LoadBalancePublish(t.Context(), channels, []byte("hello2"))
					assert.NoError(t, err, "failed to load balance publish")

					err = cache.LoadBalancePublish(t.Context(), channels, []byte("hello3"))
					assert.NoError(t, err, "failed to load balance publish")

					// Check that messages are distributed across channels
					length1, err := cache.Llen(t.Context(), "channel1")
					assert.NoError(t, err, "failed to get list length for channel1")

					length2, err := cache.Llen(t.Context(), "channel2")
					assert.NoError(t, err, "failed to get list length for channel2")

					length3, err := cache.Llen(t.Context(), "channel3")
					assert.NoError(t, err, "failed to get list length for channel3")

					assert.Equal(t, int64(1), length1)
					assert.Equal(t, int64(1), length2)
					assert.Equal(t, int64(1), length3)
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
				func(cache cache.Service) {
					var wg sync.WaitGroup

					channels := []string{"channel1", "channel2", "channel3"}
					for range 99 {
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := cache.LoadBalancePublish(t.Context(), channels, []byte("hello"))
							assert.NoError(t, err, "failed to load balance publish")
						}()
					}
					wg.Wait()

					// Check that messages are distributed across channels
					length1, err := cache.Llen(t.Context(), "channel1")
					assert.NoError(t, err, "failed to get list length for channel1")

					length2, err := cache.Llen(t.Context(), "channel2")
					assert.NoError(t, err, "failed to get list length for channel2")

					length3, err := cache.Llen(t.Context(), "channel3")
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

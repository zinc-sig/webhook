package cache_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestLoadBalanceIntegration(t *testing.T) {
	t.Run("publish and dequeue", func(t *testing.T) {
		app := fxtest.New(
			t,
			mock.ProvideMockCacheService(t),
			fx.Invoke(
				func(s cache.Service, client *redis.Client) {
					channels := []string{"channel1", "channel2", "channel3"}
					s.Put(t.Context(), cache.QueueKey, []byte(strings.Join(channels, ",")), 0)

					s.RegisterHandler("doneGrading", func(ctx context.Context, jobType, queue string, payload json.RawMessage) error {
						var payloadJson string
						if err := json.Unmarshal(payload, &payloadJson); err != nil {
							slog.Warn("Failed to unmarshal payload", "error", err)
							return err
						}
						var data cache.DoneGradingPayload
						if err := json.Unmarshal([]byte(payloadJson), &data); err != nil {
							slog.Warn("Failed to unmarshal payload", "error", err)
							return err
						}
						slog.Info("Processing job", "type", jobType, "queue", queue, "payload", data, "is_batch", len(data.Reports) > 1)

						if err := s.LoadBalanceDequeue(ctx, queue, len(data.Reports)); err != nil {
							slog.Warn("Failed to load balance dequeue", "error", err)
							return err
						}

						// Log successful processing
						var reportIDs []int
						for _, report := range data.Reports {
							reportIDs = append(reportIDs, report.ID)
						}
						slog.Info("doneGrading processed successfully", "reportIDs", reportIDs, "reportCount", len(data.Reports), "queue", queue)

						return nil
					})

					go s.Subscribe(t.Context())

					// Publish jobs
					for i := range 30 {
						err := s.LoadBalancePublish(t.Context(), channels, []byte(fmt.Sprintf("job%d", i)), 1)
						assert.NoError(t, err, "failed to load balance publish")
					}

					donePayloadJsons := make([]cache.DoneGradingPayload, 3)
					for i := 1; i <= 30; i++ {
						donePayloadJsons[i%3].Reports = append(donePayloadJsons[i%3].Reports,
							struct {
								ID           int `json:"id"`
								SubmissionID int `json:"submission_id"`
							}{ID: i, SubmissionID: i})
					}

					for idx, channel := range channels {
						donePayload, err := json.Marshal(donePayloadJsons[idx])
						assert.NoError(t, err, "failed to marshal done grading payload")

						donePayloadStr, err := json.Marshal(string(donePayload))
						assert.NoError(t, err, "failed to marshal done grading payload string")

						msg := cache.JobMessage{
							Job:     "doneGrading",
							Payload: donePayloadStr,
						}
						msgPayload, err := json.Marshal(msg)
						assert.NoError(t, err, "failed to marshal done message")
						err = client.LPush(t.Context(), fmt.Sprintf("%s:api", channel), msgPayload).Err()
						assert.NoError(t, err, "failed to push done message to channel %s", channel)
					}

					// Allow some time for processing
					t.Log("Waiting for jobs to be processed... (19s)")
					time.Sleep(19 * time.Second)
					t.Log("Finished waiting")

					// Check that all jobs have been processed and queues are empty
					for _, channel := range channels {
						length, err := client.Get(t.Context(), fmt.Sprintf("%s:length", channel)).Int()
						assert.NoError(t, err, "failed to get length for %s", channel)
						assert.Equal(t, 0, length)
					}

					// cancelCtx()
				},
			),
		)

		app.RequireStart()
		t.Cleanup(app.RequireStop)
	})
}

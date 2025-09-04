package main

import (
	"net/http"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/graphql"
	"github.com/zinc-sig/webhook/handlers"
)

func main() {
	graphqlClient := graphql.NewGraphQLClient()
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	h := &handlers.Handler{GraphQLClient: graphqlClient, RedisClient: rdb}

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.POST("/identity", h.Identity)
	e.POST("/trigger/syncEnrollment", func(c echo.Context) error {
		return h.SyncEnrollment(c, os.Getenv("ISO_API_URL"))
	})
	e.POST("/trigger/decompression", h.DecompressSubmission)
	e.POST("/trigger/postGradingProcessing", h.PostGradingProcessing)
	e.POST("/trigger/scheduleGrading", func(c echo.Context) error {
		return h.ScheduleGrading(c, os.Getenv("HASURA_URL"))
	})
	e.POST("/trigger/manualGradingTask/:assignmentConfigId", h.ManualGradingTask)
	e.POST("/trigger/gradingTask", h.GradingTask)

	e.Logger.Fatal(e.Start(":1323"))
}

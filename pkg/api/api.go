package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/zinc-sig/webhook/handlers"
	"go.uber.org/fx"
)

const port = 4000

func RegisterRoutes(e *echo.Echo, h *handlers.Handler) {
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
}

func NewRouter(h *handlers.Handler) http.Handler {
	router := echo.New()
	router.HideBanner = true
	router.HidePort = true
	router.Pre(middleware.RemoveTrailingSlash())
	router.Use(middleware.Recover())

	RegisterRoutes(router, h)

	return router
}

func NewHttp(router http.Handler, lifecycle fx.Lifecycle) *http.Server {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	return server
}

var Module = fx.Options(
	fx.Provide(
		NewRouter,
		NewHttp,
	),
	fx.Invoke(func(lifecycle fx.Lifecycle, httpServer *http.Server) {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				slog.Info("Starting webhook server at :", "port", port)
				go func() {
					if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						slog.Warn("Failed to start http server", "error", err)
					}
				}()
				return httpServer.ListenAndServe()
			},
			OnStop: func(ctx context.Context) error {
				slog.Info("Stopping webhook server")
				return httpServer.Shutdown(context.Background())
			},
		})
	}),
)

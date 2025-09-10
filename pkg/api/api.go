package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Handler interface {
	RegisterRoutes(e *echo.Echo)
}

const Port = 4000

type ApiParams struct {
	fx.In
	Handlers []Handler `group:"handlers"`
	Logger   *zap.SugaredLogger
}

type Response struct {
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func NewRouter(p ApiParams) http.Handler {
	router := echo.New()
	router.HideBanner = true
	router.HidePort = true
	router.Use(otelecho.Middleware("hk.ust.cse.zinc.webhook"))
	router.Pre(middleware.RemoveTrailingSlash())
	router.Use(middleware.Recover())
	router.Use(middleware.RequestID())

	router.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	for _, handler := range p.Handlers {
		handler.RegisterRoutes(router)
	}
	router.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			p.Logger.Infow("request",
				"URI", v.URI,
				"method", v.Method,
				"status", v.Status,
				"requestID", v.RequestID,
			)
			return nil
		},
	}))

	return router
}

func NewHttp(router http.Handler, lifecycle fx.Lifecycle) *http.Server {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", Port),
		Handler: router,
	}

	return server
}

func AsHandler(h any) any {
	return fx.Annotate(
		h,
		fx.As(new(Handler)),
		fx.ResultTags(`group:"handlers"`),
	)
}

var Module = fx.Options(
	fx.Provide(
		NewRouter,
		NewHttp,
	),
	fx.Invoke(func(lifecycle fx.Lifecycle, httpServer *http.Server, logger *zap.SugaredLogger) {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				logger.Infow("Starting webhook server", "context", ctx, "port", Port)
				go func() {
					if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						logger.Warnw("Failed to start http server", "error", err)
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				logger.Infow("Stopping webhook server", "context", ctx)
				return httpServer.Shutdown(context.Background())
			},
		})
	}),
)

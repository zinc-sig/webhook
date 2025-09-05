package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"
)

type Handler interface {
	RegisterRoutes(e *echo.Echo)
}

const port = 4000

type ApiParams struct {
	fx.In
	Handlers []Handler `group:"handlers"`
}

func NewRouter(p ApiParams) http.Handler {
	router := echo.New()
	router.HideBanner = true
	router.HidePort = true
	router.Pre(middleware.RemoveTrailingSlash())
	router.Use(middleware.Recover())
	router.Use(middleware.RequestID())

	router.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	for _, handler := range p.Handlers {
		handler.RegisterRoutes(router)
	}

	return router
}

func NewHttp(router http.Handler, lifecycle fx.Lifecycle) *http.Server {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
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

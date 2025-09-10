package telemetry

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Config struct {
	CollectorEndpoint string  `mapstructure:"collector_endpoint" yaml:"collector_endpoint"`
	SamplingRate      float64 `mapstructure:"sampling_rate" yaml:"sampling_rate"` // 0.0 to 1.0, defaults to 0.1 (10%)
	Environment       string  `mapstructure:"environment" yaml:"environment"`     // production, staging, development
}

func NewMeterProvider(config *Config, res *resource.Resource, lifecycle fx.Lifecycle) (*metric.MeterProvider, error) {
	metricExporter, err := otlpmetrichttp.New(
		context.Background(),
		otlpmetrichttp.WithEndpointURL(config.CollectorEndpoint),
	)
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			metric.WithInterval(time.Minute))),
	)

	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			otel.SetMeterProvider(meterProvider)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return meterProvider.Shutdown(ctx)
		},
	})
	return meterProvider, nil
}

func NewPropagator(config *Config, lifecycle fx.Lifecycle) propagation.TextMapPropagator {

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			otel.SetTextMapPropagator(propagator)
			return nil
		},
	})

	return propagator
}

func NewTraceProvider(config *Config, res *resource.Resource, lifecycle fx.Lifecycle) (*trace.TracerProvider, error) {
	traceExporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpointURL(config.CollectorEndpoint),
	)
	if err != nil {
		return nil, err
	}

	// Set sampling rate based on config, default to 10% if not specified
	samplingRate := config.SamplingRate
	if samplingRate == 0 {
		samplingRate = 0.1 // Default 10% sampling
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithSampler(trace.TraceIDRatioBased(samplingRate)),
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(time.Second*5)),
	)
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			otel.SetTracerProvider(traceProvider)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return traceProvider.Shutdown(ctx)
		},
	})

	return traceProvider, nil
}

func NewLoggerProvider(config *Config, resource *resource.Resource) (*log.LoggerProvider, error) {
	logExporter, err := otlploghttp.New(
		context.Background(),
		otlploghttp.WithEndpointURL(config.CollectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	processor := log.NewBatchProcessor(logExporter)
	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(processor),
		log.WithResource(resource),
	)
	return loggerProvider, nil
}

func NewResource(param struct {
	fx.In
	Config  *Config
	DevMode bool `name:"devModeEnabled"`
}) (*resource.Resource, error) {
	serviceName := "zinc-webhook"
	version := "dev"

	// Determine environment
	environment := param.Config.Environment
	if environment == "" {
		if param.DevMode {
			environment = "development"
		} else {
			environment = "production"
		}
	}

	// Get instance ID (hostname or container ID)
	instanceID, err := os.Hostname()
	if err != nil {
		instanceID = "unknown"
	}

	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(version),
		semconv.DeploymentEnvironmentKey.String(environment),
		semconv.ServiceInstanceIDKey.String(instanceID),
	), nil
}

type LifecycleEventParams struct {
	fx.In
	Lifecycle      fx.Lifecycle
	Logger         *zap.SugaredLogger
	MeterProvider  *metric.MeterProvider
	Propagator     propagation.TextMapPropagator
	TracerProvider *trace.TracerProvider
}

func AttachLifecycleEvents(params LifecycleEventParams) {
	params.Lifecycle.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if params.Logger != nil {
				params.Logger.Infow("Stopping telemetry components")
			}
			return nil
		},
	})
}

var Module = fx.Module(
	"telemetry",
	fx.Provide(
		NewResource,
		NewMeterProvider,
		NewPropagator,
		NewTraceProvider,
		NewLoggerProvider,
	),
	fx.Invoke(AttachLifecycleEvents),
)

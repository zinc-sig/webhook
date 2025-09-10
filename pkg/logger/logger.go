package logger

import (
	"os"

	"github.com/fatih/color"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/sdk/log"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var FxEventLog = "false"

func Module(debug bool) fx.Option {
	if debug {
		return fx.Options(
			fx.Provide(New),
			fx.WithLogger(func(logger *zap.SugaredLogger) fxevent.Logger {
				return &fxevent.ZapLogger{Logger: logger.Desugar()}
			}),
		)
	} else {
		return fx.Options(
			fx.Provide(New),
			fx.NopLogger,
		)
	}
}

type LoggerParams struct {
	fx.In
	LoggerProvider *log.LoggerProvider `optional:"true"`
}

func New(p LoggerParams) (*zap.SugaredLogger, error) {
	// setting the log level for console log output
	consoleLogLevel := zapcore.InfoLevel

	// ignore context fields used to bind tracing context to logs
	filteredFields := []string{"context"}

	// setup the encoders
	consoleEncoderConfig := zap.NewProductionEncoderConfig()
	colorMap := map[zapcore.Level]*color.Color{
		zapcore.DebugLevel:  color.New(color.FgMagenta),
		zapcore.InfoLevel:   color.New(color.FgBlue),
		zapcore.WarnLevel:   color.New(color.FgYellow),
		zapcore.ErrorLevel:  color.New(color.FgRed),
		zapcore.DPanicLevel: color.New(color.FgRed, color.Bold),
		zapcore.FatalLevel:  color.New(color.FgRed, color.Bold),
		zapcore.PanicLevel:  color.New(color.FgRed, color.Bold),
	}
	consoleEncoderConfig.EncodeLevel = func(l zapcore.Level, pae zapcore.PrimitiveArrayEncoder) {
		// custom encoding of level string as [INFO] style
		pae.AppendString(colorMap[l].Sprintf("[%s]", l.CapitalString()))
	}
	consoleEncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	consoleEncoderConfig.EncodeCaller = func(ec zapcore.EntryCaller, pae zapcore.PrimitiveArrayEncoder) {
		// custom encoding of the caller, now is set to the trimmed file path
		pae.AppendString(ec.TrimmedPath())
	}
	consoleEncoder := NewFilteredEncoder(zapcore.NewConsoleEncoder(consoleEncoderConfig), filteredFields)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stderr), consoleLogLevel),
		otelzap.NewCore("github.com/zinc-sig/webhook/pkg/logger", otelzap.WithLoggerProvider(p.LoggerProvider)),
	)

	return zap.New(core, zap.AddCaller()).Sugar(), nil
}

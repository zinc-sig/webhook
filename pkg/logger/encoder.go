package logger

import (
	"slices"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

type FilteredEncoder struct {
	zapcore.Encoder
	IgnoredFields []string
}

func NewFilteredEncoder(encoder zapcore.Encoder, ignoredFields []string) zapcore.Encoder {
	return &FilteredEncoder{
		Encoder:       encoder,
		IgnoredFields: ignoredFields,
	}
}

func (e *FilteredEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	var filteredFields []zapcore.Field
	for _, field := range fields {
		if !slices.Contains(e.IgnoredFields, field.Key) {
			filteredFields = append(filteredFields, field)
		}
	}
	return e.Encoder.EncodeEntry(entry, filteredFields)
}

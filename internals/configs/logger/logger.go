package logger

import (
	"log"

	"go.uber.org/zap"
)

// Logger interface that holds methods required method when using any logging package
type Logger interface {
	LogWithFields(level string, message string, fields ...interface{})
	Log(level string, message string)
}

// SugarLogger is a wrapper around zap.SugaredLogger
type SugarLogger struct {
	*zap.SugaredLogger
}

// NewSugarLogger creates a new Logger interface instance
func NewSugarLogger(isProduction bool) Logger {
	// Create Zap logger
	var (
		logger *zap.Logger
		err    error
	)
	if isProduction {
		logger, err = zap.NewProduction()
		if err != nil {
			log.Panic(err.Error())
		}
	} else {
		logger, err = zap.NewDevelopment()
		if err != nil {
			log.Panic(err.Error())
		}
	}

	return &SugarLogger{logger.Sugar()}
}

// LogWithFields creates a new logger instance with additional fields
func (s *SugarLogger) LogWithFields(level string, message string, fields ...interface{}) {
	switch level {
	case "debug":
		s.SugaredLogger.With(fields...).Debug(message)
	case "info":
		s.SugaredLogger.With(fields...).Info(message)
	case "warn":
		s.SugaredLogger.With(fields...).Warn(message)
	case "error":
		s.SugaredLogger.With(fields...).Error(message)
	case "dPanic":
		s.SugaredLogger.With(fields...).DPanic(message)
	case "panic":
		s.SugaredLogger.With(fields...).Panic(message)
	case "fatal":
	default:
		s.SugaredLogger.With(fields...).Fatal(message)
	}
}

// Log creates a new logger instance without additional fields
func (s *SugarLogger) Log(level string, message string) {
	switch level {
	case "debug":
		s.SugaredLogger.Debug(message)
	case "info":
		s.SugaredLogger.Info(message)
	case "warn":
		s.SugaredLogger.Warn(message)
	case "error":
		s.SugaredLogger.Error(message)
	case "dPanic":
		s.SugaredLogger.DPanic(message)
	case "panic":
		s.SugaredLogger.Panic(message)
	case "fatal":
	default:
		s.SugaredLogger.Fatal(message)
	}
}

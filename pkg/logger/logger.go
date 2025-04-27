package logger

import (
	"os"
	"sync"

	"gitlab.ozon.dev/gojhw1/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})
}

// globalLogger хранит экземпляр глобального логгера
var (
	globalLogger Logger
	once         sync.Once
)

// InitGlobalLogger инициализирует глобальный логгер
func InitGlobalLogger(cfg *config.Config) error {
	var err error
	once.Do(func() {
		globalLogger, err = NewSugaredLogger(cfg)
	})
	return err
}

// Global возвращает экземпляр глобального логгера
func Global() Logger {
	if globalLogger == nil {
		// Возвращаем стандартный логгер, если глобальный не инициализирован
		// Это позволит избежать паники при использовании логгера до инициализации
		return zap.NewNop().Sugar()
	}
	return globalLogger
}

func Debug(args ...interface{})                   { Global().Debug(args...) }
func Info(args ...interface{})                    { Global().Info(args...) }
func Warn(args ...interface{})                    { Global().Warn(args...) }
func Error(args ...interface{})                   { Global().Error(args...) }
func Fatal(args ...interface{})                   { Global().Fatal(args...) }
func Debugf(template string, args ...interface{}) { Global().Debugf(template, args...) }
func Infof(template string, args ...interface{})  { Global().Infof(template, args...) }
func Warnf(template string, args ...interface{})  { Global().Warnf(template, args...) }
func Errorf(template string, args ...interface{}) { Global().Errorf(template, args...) }
func Fatalf(template string, args ...interface{}) { Global().Fatalf(template, args...) }

func NewSugaredLogger(cfg *config.Config) (Logger, error) {
	level := getLogLevel(cfg.Logger.Level)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if cfg.Logger.Encoding == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var writeSyncer zapcore.WriteSyncer
	if cfg.Logger.OutputPath == "" && cfg.Logger.OutputPath != "stdout" {
		file, err := os.OpenFile(cfg.Logger.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		writeSyncer = zapcore.AddSync(file)
	} else {
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	core := zapcore.NewCore(
		encoder,
		writeSyncer,
		level,
	)

	var logger *zap.Logger
	if cfg.Logger.DevMode {
		logger = zap.New(core, zap.Development(), zap.AddStacktrace(zapcore.ErrorLevel))
	} else {
		logger = zap.New(core, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	return logger.Sugar(), nil
}

func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

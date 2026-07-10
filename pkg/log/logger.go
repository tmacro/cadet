package log

import (
	"os"
	"sync"
	
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type API interface {
	Debug(v ...any)
	Debugf(format string, v ...any)
	Info(v ...any)
	Infof(format string, v ...any)
	Error(v ...any)
	Errorf(format string, v ...any)
	Warn(v ...any)
	Warnf(format string, v ...any)
	Fatal(v ...any)
	Fatalf(format string, v ...any)
}

var (
	logger API
	mu     sync.RWMutex
)

func init() {
	mu.Lock()
	logger = zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
		zapcore.Lock(os.Stdout),
		zapcore.InfoLevel,
	)).Sugar()
	mu.Unlock()
}

func SetLogger(l API) {
	mu.Lock()
	defer mu.Unlock()
	logger = l
}

func Info(v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Info(v...)
}

func Infof(format string, v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Infof(format, v...)
}

func Error(v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Error(v...)
}

func Errorf(format string, v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Errorf(format, v...)
}

func Warn(v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Warn(v...)
}

func Warnf(format string, v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Warnf(format, v...)
}

func Fatal(v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Fatal(v...)
}

func Fatalf(format string, v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Fatalf(format, v...)
}

func Debug(v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Debug(v...)
}

func Debugf(format string, v ...any) {
	mu.RLock()
	defer mu.RUnlock()
	logger.Debugf(format, v...)
}

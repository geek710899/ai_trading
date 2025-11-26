package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	infoFile    *os.File
	errorFile   *os.File
	metricsFile *os.File
	pnlFile     *os.File
	tradesFile  *os.File
	mu          sync.Mutex
	baseDir     string
	day         string
}

func New(baseDir string) *Logger {
	l := &Logger{baseDir: baseDir}
	l.rotateIfNeeded()
	return l
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.infoFile != nil {
		_ = l.infoFile.Close()
	}
	if l.errorFile != nil {
		_ = l.errorFile.Close()
	}
}

func (l *Logger) rotateIfNeeded() {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := time.Now().Format("2006-01-02")
	if day == l.day && l.infoFile != nil && l.errorFile != nil && l.metricsFile != nil && l.pnlFile != nil && l.tradesFile != nil {
		return
	}
	l.day = day
	infoDir := filepath.Join(l.baseDir, "info")
	errDir := filepath.Join(l.baseDir, "error")
	metricsDir := filepath.Join(l.baseDir, "metrics")
	pnlDir := filepath.Join(l.baseDir, "pnl")
	tradesDir := filepath.Join(l.baseDir, "trades")
	_ = os.MkdirAll(infoDir, 0o755)
	_ = os.MkdirAll(errDir, 0o755)
	_ = os.MkdirAll(metricsDir, 0o755)
	_ = os.MkdirAll(pnlDir, 0o755)
	_ = os.MkdirAll(tradesDir, 0o755)
	infoPath := filepath.Join(infoDir, day+".log")
	errPath := filepath.Join(errDir, day+".log")
	metricsPath := filepath.Join(metricsDir, day+".log")
	pnlPath := filepath.Join(pnlDir, day+".log")
	tradesPath := filepath.Join(tradesDir, day+".log")
	if l.infoFile != nil {
		_ = l.infoFile.Close()
	}
	if l.errorFile != nil {
		_ = l.errorFile.Close()
	}
	if l.metricsFile != nil {
		_ = l.metricsFile.Close()
	}
	if l.pnlFile != nil {
		_ = l.pnlFile.Close()
	}
	if l.tradesFile != nil {
		_ = l.tradesFile.Close()
	}
	l.infoFile, _ = os.OpenFile(infoPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	l.errorFile, _ = os.OpenFile(errPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	l.metricsFile, _ = os.OpenFile(metricsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	l.pnlFile, _ = os.OpenFile(pnlPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	l.tradesFile, _ = os.OpenFile(tradesPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func (l *Logger) Info(tag string, kv ...string) {
	l.rotateIfNeeded()
	l.write(l.infoFile, "INFO", tag, kv...)
}

func (l *Logger) Error(tag string, kv ...string) {
	l.rotateIfNeeded()
	l.write(l.errorFile, "ERROR", tag, kv...)
}

func (l *Logger) Metrics(tag string, kv ...string) {
	l.rotateIfNeeded()
	l.write(l.metricsFile, "METRICS", tag, kv...)
}

func (l *Logger) PnL(tag string, kv ...string) {
	l.rotateIfNeeded()
	l.write(l.pnlFile, "PNL", tag, kv...)
}

func (l *Logger) Trade(tag string, kv ...string) {
	l.rotateIfNeeded()
	l.write(l.tradesFile, "TRADE", tag, kv...)
}

func (l *Logger) write(f *os.File, level, tag string, kv ...string) {
	if f == nil {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	line := fmt.Sprintf("%s %s %s", ts, level, tag)
	for i := 0; i+1 < len(kv); i += 2 {
		line += fmt.Sprintf(" %s=%s", kv[i], kv[i+1])
	}
	line += "\n"
	_, _ = f.WriteString(line)
}

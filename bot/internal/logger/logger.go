package logger

import (
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
)

type Logger struct {
    infoFile  *os.File
    errorFile *os.File
    mu        sync.Mutex
    baseDir   string
    day       string
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
    if day == l.day && l.infoFile != nil && l.errorFile != nil {
        return
    }
    l.day = day
    infoDir := filepath.Join(l.baseDir, "info")
    errDir := filepath.Join(l.baseDir, "error")
    _ = os.MkdirAll(infoDir, 0o755)
    _ = os.MkdirAll(errDir, 0o755)
    infoPath := filepath.Join(infoDir, day+".log")
    errPath := filepath.Join(errDir, day+".log")
    if l.infoFile != nil {
        _ = l.infoFile.Close()
    }
    if l.errorFile != nil {
        _ = l.errorFile.Close()
    }
    l.infoFile, _ = os.OpenFile(infoPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    l.errorFile, _ = os.OpenFile(errPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func (l *Logger) Info(tag string, kv ...string) {
    l.rotateIfNeeded()
    l.write(l.infoFile, "INFO", tag, kv...)
}

func (l *Logger) Error(tag string, kv ...string) {
    l.rotateIfNeeded()
    l.write(l.errorFile, "ERROR", tag, kv...)
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


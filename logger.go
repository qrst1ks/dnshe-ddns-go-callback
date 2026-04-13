package main

import (
	"fmt"
	"sync"
	"time"
)

// LogEntry 单条日志
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

var (
	logBuf  []LogEntry
	logLock sync.RWMutex
	maxLogs = 200
)

// addLog 追加一条日志到内存缓冲区
func addLog(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	now := time.Now().Format("15:04:05")

	logLock.Lock()
	logBuf = append(logBuf, LogEntry{Time: now, Level: level, Message: msg})
	if len(logBuf) > maxLogs {
		logBuf = logBuf[len(logBuf)-maxLogs:]
	}
	logLock.Unlock()

	// 同时输出到终端
	fmt.Printf("[%s] [%s] %s\n", now, level, msg)
}

// getLogs 返回最近的日志
func getLogs() []LogEntry {
	logLock.RLock()
	defer logLock.RUnlock()
	result := make([]LogEntry, len(logBuf))
	copy(result, logBuf)
	return result
}

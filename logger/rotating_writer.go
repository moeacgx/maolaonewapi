package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogMaxSizeMB     = 100
	defaultLogRetentionDays = 0
)

// rotatingFileWriter 按文件大小和自然日轮转日志，避免高流量访问日志持续写入单个文件。
type rotatingFileWriter struct {
	mu            sync.Mutex
	dir           string
	maxBytes      int64
	retentionDays int
	file          *os.File
	path          string
	day           string
	size          int64
}

func newRotatingFileWriter(dir string, maxSizeMB, retentionDays int) (*rotatingFileWriter, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	writer := &rotatingFileWriter{
		dir:           dir,
		maxBytes:      int64(maxSizeMB) * 1024 * 1024,
		retentionDays: retentionDays,
	}
	if err := writer.rotateLocked(time.Now()); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	if w.file == nil || w.day != now.Format("20060102") ||
		(w.maxBytes > 0 && w.size > 0 && w.size+int64(len(p)) > w.maxBytes) {
		if err := w.rotateLocked(now); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFileWriter) rotateLocked(now time.Time) error {
	timestamp := now.Format("20060102150405")
	var (
		fd   *os.File
		path string
		err  error
	)
	for attempt := 0; ; attempt++ {
		name := fmt.Sprintf("oneapi-%s.log", timestamp)
		if attempt > 0 {
			name = fmt.Sprintf("oneapi-%s-%02d.log", timestamp, attempt)
		}
		path = filepath.Join(w.dir, name)
		fd, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if !os.IsExist(err) {
			break
		}
	}
	if err != nil {
		return err
	}

	oldFile := w.file
	w.file = fd
	w.path = path
	w.day = now.Format("20060102")
	w.size = 0
	currentLogPathMu.Lock()
	currentLogPath = path
	currentLogPathMu.Unlock()
	if oldFile != nil {
		_ = oldFile.Close()
	}
	w.cleanupLocked(now)
	return nil
}

func (w *rotatingFileWriter) cleanupLocked(now time.Time) {
	if w.retentionDays <= 0 {
		return
	}
	cutoff := now.AddDate(0, 0, -w.retentionDays)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "oneapi-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		path := filepath.Join(w.dir, entry.Name())
		if path == w.path {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

func (w *rotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

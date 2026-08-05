package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotatingFileWriterRotatesBySize(t *testing.T) {
	dir := t.TempDir()
	writer, err := newRotatingFileWriter(dir, 1, 0)
	if err != nil {
		t.Fatalf("创建日志写入器失败: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	payload := make([]byte, 700*1024)
	if _, err = writer.Write(payload); err != nil {
		t.Fatalf("第一次写入失败: %v", err)
	}
	if _, err = writer.Write(payload); err != nil {
		t.Fatalf("第二次写入失败: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "oneapi-*.log"))
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("期望轮转后有 2 个日志文件，实际为 %d", len(files))
	}
}

func TestRotatingFileWriterRemovesExpiredFilesWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "oneapi-20200101000000.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatalf("创建旧日志失败: %v", err)
	}
	oldTime := time.Now().AddDate(0, 0, -2)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("修改旧日志时间失败: %v", err)
	}

	writer, err := newRotatingFileWriter(dir, 1, 1)
	if err != nil {
		t.Fatalf("创建日志写入器失败: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("过期日志未按保留策略清理: %v", err)
	}
	if _, err := os.Stat(writer.path); err != nil {
		t.Fatalf("当前日志文件不应被清理: %v", err)
	}
}

func TestRotatingFileWriterRotatesOnNewDay(t *testing.T) {
	dir := t.TempDir()
	writer, err := newRotatingFileWriter(dir, 0, 0)
	if err != nil {
		t.Fatalf("创建日志写入器失败: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	writer.mu.Lock()
	writer.day = time.Now().AddDate(0, 0, -1).Format("20060102")
	writer.mu.Unlock()
	if _, err = writer.Write([]byte("next day")); err != nil {
		t.Fatalf("跨自然日写入失败: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "oneapi-*.log"))
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("期望跨自然日后有 2 个日志文件，实际为 %d", len(files))
	}
}

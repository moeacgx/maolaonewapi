package logger

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestLogDebugOnlyWritesWhenEnabled(t *testing.T) {
	oldDebugEnabled := common.DebugEnabled
	oldErrorWriter := gin.DefaultErrorWriter
	t.Cleanup(func() {
		common.DebugEnabled = oldDebugEnabled
		gin.DefaultErrorWriter = oldErrorWriter
	})

	var output bytes.Buffer
	gin.DefaultErrorWriter = &output
	common.DebugEnabled = false
	LogDebug(nil, "consume params: %s", "hidden")
	if output.Len() != 0 {
		t.Fatalf("非调试模式不应输出调试日志: %s", output.String())
	}

	common.DebugEnabled = true
	LogDebug(nil, "consume params: %s", "visible")
	if !strings.Contains(output.String(), "consume params: visible") {
		t.Fatalf("调试模式应输出调试日志")
	}
}

func TestRuntimeLogEnabledConfig(t *testing.T) {
	t.Setenv("RUNTIME_LOG_ENABLED", "false")
	if runtimeLogEnabled() {
		t.Fatalf("RUNTIME_LOG_ENABLED=false 时不应启用运行日志")
	}
	t.Setenv("RUNTIME_LOG_ENABLED", "true")
	if !runtimeLogEnabled() {
		t.Fatalf("RUNTIME_LOG_ENABLED=true 时应启用运行日志")
	}
}

func TestSetupLoggerDisabledPreservesErrorOutput(t *testing.T) {
	oldDefaultWriter := gin.DefaultWriter
	oldDefaultErrorWriter := gin.DefaultErrorWriter
	oldLogWriter := log.Writer()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldDefaultWriter
		gin.DefaultErrorWriter = oldDefaultErrorWriter
		common.LogWriterMu.Unlock()
		log.SetOutput(oldLogWriter)
	})

	t.Setenv("RUNTIME_LOG_ENABLED", "false")
	SetupLogger()

	if gin.DefaultWriter != io.Discard {
		t.Fatalf("关闭常规运行日志时应丢弃访问和信息日志")
	}
	if gin.DefaultErrorWriter != os.Stderr {
		t.Fatalf("关闭常规运行日志时仍应将错误和致命信息写入 stderr")
	}
	if log.Writer() != os.Stderr {
		t.Fatalf("关闭常规运行日志时 Go 标准错误日志仍应写入 stderr")
	}
}

package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	loggerINFO  = "INFO"
	loggerWarn  = "WARN"
	loggerError = "ERR"
	loggerDebug = "DEBUG"
)

var currentLogPath string
var currentLogPathMu sync.RWMutex
var currentLogWriter *rotatingFileWriter
var currentLogWriterMu sync.Mutex

func GetCurrentLogPath() string {
	currentLogPathMu.RLock()
	defer currentLogPathMu.RUnlock()
	return currentLogPath
}

func SetupLogger() {
	if !runtimeLogEnabled() {
		currentLogWriterMu.Lock()
		oldWriter := currentLogWriter
		currentLogWriter = nil
		currentLogWriterMu.Unlock()
		currentLogPathMu.Lock()
		currentLogPath = ""
		currentLogPathMu.Unlock()
		common.LogWriterMu.Lock()
		gin.DefaultWriter = io.Discard
		gin.DefaultErrorWriter = os.Stderr
		common.LogWriterMu.Unlock()
		log.SetOutput(os.Stderr)
		if oldWriter != nil {
			_ = oldWriter.Close()
		}
		return
	}
	if *common.LogDir == "" {
		return
	}
	log.SetOutput(os.Stderr)
	maxSizeMB := common.GetEnvOrDefault("LOG_MAX_SIZE_MB", defaultLogMaxSizeMB)
	if maxSizeMB < 0 {
		maxSizeMB = 0
	}
	retentionDays := common.GetEnvOrDefault("LOG_RETENTION_DAYS", defaultLogRetentionDays)
	if retentionDays < 0 {
		retentionDays = 0
	}
	writer, err := newRotatingFileWriter(*common.LogDir, maxSizeMB, retentionDays)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}

	currentLogWriterMu.Lock()
	oldWriter := currentLogWriter
	currentLogWriter = writer
	currentLogWriterMu.Unlock()
	common.LogWriterMu.Lock()
	gin.DefaultWriter = io.MultiWriter(os.Stdout, writer)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, writer)
	common.LogWriterMu.Unlock()
	if oldWriter != nil {
		_ = oldWriter.Close()
	}
}

func runtimeLogEnabled() bool {
	return common.GetEnvOrDefaultBool("RUNTIME_LOG_ENABLED", true)
}

func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

func LogWarn(ctx context.Context, msg string) {
	logHelper(ctx, loggerWarn, msg)
}

func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

func LogDebug(ctx context.Context, msg string, args ...any) {
	if common.DebugEnabled {
		if len(args) > 0 {
			msg = fmt.Sprintf(msg, args...)
		}
		logHelper(ctx, loggerDebug, msg)
	}
}

func logHelper(ctx context.Context, level string, msg string) {
	var id any = "SYSTEM"
	if ctx != nil {
		if requestID := ctx.Value(common.RequestIdKey); requestID != nil {
			id = requestID
		}
	}
	now := time.Now()
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	if level == loggerINFO {
		writer = gin.DefaultWriter
	}
	_, _ = fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
	common.LogWriterMu.RUnlock()
}

func LogQuota(quota int) string {
	// 新逻辑：根据额度展示类型输出
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f 额度", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f 额度", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d 点额度", quota)
	default: // USD
		return fmt.Sprintf("＄%.6f 额度", q/common.QuotaPerUnit)
	}
}

func FormatQuota(quota int) string {
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d", quota)
	default:
		return fmt.Sprintf("＄%.6f", q/common.QuotaPerUnit)
	}
}

// LogJson 仅供测试使用 only for test
func LogJson(ctx context.Context, msg string, obj any) {
	if !common.DebugEnabled {
		return
	}
	jsonStr, err := common.Marshal(obj)
	if err != nil {
		LogError(ctx, fmt.Sprintf("json marshal failed: %s", err.Error()))
		return
	}
	LogDebug(ctx, "%s | %s", msg, jsonStr)
}

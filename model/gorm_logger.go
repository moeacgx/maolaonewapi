package model

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultSlowThresholdMs = 200
	maxSlowThresholdMs     = 60 * 60 * 1000
)

func newGormConfig(prepareStmt bool) *gorm.Config {
	return &gorm.Config{
		PrepareStmt: prepareStmt,
		Logger:      newGormLogger(os.Stdout),
	}
}

func newGormLogger(w io.Writer) logger.Interface {
	slowThresholdMs := common.GetEnvOrDefault("SQL_SLOW_THRESHOLD_MS", defaultSlowThresholdMs)
	if slowThresholdMs < 0 || slowThresholdMs > maxSlowThresholdMs {
		common.SysError(fmt.Sprintf(
			"invalid SQL_SLOW_THRESHOLD_MS %d (allowed 0-%d, 0 disables slow query log), using default %d",
			slowThresholdMs,
			maxSlowThresholdMs,
			defaultSlowThresholdMs,
		))
		slowThresholdMs = defaultSlowThresholdMs
	}
	// 在 Writer 层脱敏可保留 GORM 对业务调用点的正确归因，也不会破坏 ParamsFilter。
	return logger.New(&sanitizedLogWriter{delegate: log.New(w, "\r\n", log.LstdFlags)}, logger.Config{
		SlowThreshold:             time.Duration(slowThresholdMs) * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      !common.DebugEnabled,
		Colorful:                  true,
	})
}

// ParameterizedQueries 只过滤 SQL 参数；驱动错误消息也可能内联敏感数据，
// 因此非调试模式下将已知数据库驱动错误收敛为错误码。
type sanitizedLogWriter struct {
	delegate *log.Logger
}

func (s *sanitizedLogWriter) Printf(format string, args ...interface{}) {
	if !common.DebugEnabled {
		for i, arg := range args {
			if err, ok := arg.(error); ok {
				args[i] = sanitizeDBError(err)
			}
		}
	}
	s.delegate.Printf(format, args...)
}

// 仅收敛可能包含查询数据的数据库服务端错误；网络、上下文等错误原样保留，
// 以免损失必要的连接与超时排障信息。
func sanitizeDBError(err error) error {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return fmt.Errorf("mysql error %d", mysqlErr.Number)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("postgres error SQLSTATE %s", pgErr.Code)
	}

	var sqliteErr *sqlitedriver.Error
	if errors.As(err, &sqliteErr) {
		return fmt.Errorf("sqlite error %d", sqliteErr.Code())
	}

	return err
}

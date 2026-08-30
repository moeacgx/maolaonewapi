package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordConsumeLogPreservesFailureType(t *testing.T) {
	db := setupOperationAuditLogTestDB(t)
	previousLogConsume := common.LogConsumeEnabled
	previousDataExport := common.DataExportEnabled
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = previousLogConsume
		common.DataExportEnabled = previousDataExport
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("username", "audit-admin")
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ModelName: "gpt-test",
		LogType:   LogTypeError,
	})

	var log Log
	require.NoError(t, db.Order("id DESC").First(&log).Error)
	assert.Equal(t, LogTypeError, log.Type)
}

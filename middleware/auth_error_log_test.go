package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordAuthErrorLogPersistsGroupAccessFailure(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldEnabled := constant.ErrorLogEnabled
	oldRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	constant.ErrorLogEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		model.InitDBColumns()
		constant.ErrorLogEnabled = oldEnabled
		common.RedisEnabled = oldRedis
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.User{Id: 77, Username: "auth-log-user"}).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set("id", 77)
	ctx.Set("username", "auth-log-user")
	ctx.Set("token_id", 88)
	ctx.Set("token_name", "grok")
	ctx.Set(common.RequestIdKey, "auth-log-request")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "grok-4.6")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "Grok-Super")

	recordAuthErrorLog(ctx, http.StatusForbidden, "无权访问 Grok-Super 分组", types.ErrorCodeAccessDenied, "381")

	var logRow model.Log
	require.NoError(t, db.Where("request_id = ?", "auth-log-request").First(&logRow).Error)
	assert.Equal(t, model.LogTypeError, logRow.Type)
	assert.Equal(t, 0, logRow.ChannelId)
	assert.Equal(t, "grok-4.6", logRow.ModelName)
	assert.Equal(t, "381", logRow.Group)
	assert.Contains(t, logRow.Content, "无权访问 Grok-Super 分组")
	assert.Contains(t, logRow.Other, `"error_stage":"authentication"`)
}

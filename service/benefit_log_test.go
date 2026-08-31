package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBenefitBillingInfoIncludesBreakdownAndLinksLedgerLogID(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	db, err := gorm.Open(sqlite.Open("file:benefit_log_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.BenefitVoucherLedger{}))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		model.InitDBColumns()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.BenefitVoucherLedger{RequestId: "log-request", Type: model.BenefitLedgerTypePreConsume, VoucherId: 8, ActivityId: 9, UserId: 10, QuotaDelta: -30, BalanceAfter: 70}).Error)
	info := &relaycommon.RelayInfo{
		BillingSource:    "benefit_voucher",
		BillingBreakdown: &relaycommon.BillingBreakdown{VoucherQuota: 30, SubscriptionQuota: 40, WalletQuota: 30, ActivityID: 9, VoucherID: 8},
		RequestId:        "log-request",
		RelayFormat:      types.RelayFormatOpenAI,
		StartTime:        time.Now(), FirstResponseTime: time.Now(),
	}
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	other := make(map[string]interface{})
	appendBillingInfo(info, other)
	breakdown, ok := other["billing_breakdown"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, int64(30), breakdown["voucher_quota"])
	assert.Equal(t, int64(40), breakdown["subscription_quota"])
	assert.Equal(t, int64(30), breakdown["wallet_quota"])

	logID := model.RecordConsumeLog(ctx, 10, model.RecordConsumeLogParams{Quota: 100, ModelName: "gpt-test", Group: "benefit", Other: other})
	assert.Positive(t, logID)
	require.NoError(t, model.LinkBenefitLedgerLogID("log-request", logID))
	var ledger model.BenefitVoucherLedger
	require.NoError(t, db.Where("request_id = ? AND type = ?", "log-request", model.BenefitLedgerTypePreConsume).First(&ledger).Error)
	assert.Equal(t, logID, ledger.LogId)
}

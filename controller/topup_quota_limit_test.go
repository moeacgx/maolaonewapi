package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTopUpQuotaValidation(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	testCases := []struct {
		name        string
		displayType string
		amount      int64
		wantQuota   int64
		wantErr     bool
	}{
		{
			name:        "currency amount below limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      4294,
			wantQuota:   2_147_000_000,
		},
		{
			name:        "currency amount above the old int32 limit",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      10_000,
			wantQuota:   5_000_000_000,
		},
		{
			name:        "currency amount of fifty thousand dollars",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      50_000,
			wantQuota:   25_000_000_000,
		},
		{
			name:        "token amount preserves settlement truncation",
			displayType: operation_setting.QuotaDisplayTypeTokens,
			amount:      5_000_000_123,
			wantQuota:   5_000_000_000,
		},
		{
			name:        "currency amount beyond int64 storage range",
			displayType: operation_setting.QuotaDisplayTypeUSD,
			amount:      18_446_744_073_710,
			wantErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.displayType
			quota, err := getTopUpQuota(tc.amount)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantQuota, quota)
		})
	}
}

func TestValidateTopUpQuotaAllowsLargeAmountAndKeepsInt64Boundary(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
	})

	maxAmount := decimal.NewFromInt(common.MaxWalletQuota).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Floor().IntPart()

	_, err := validateTopUpQuota(maxAmount)
	require.NoError(t, err)
	_, err = validateTopUpQuota(maxAmount + 1)
	require.Error(t, err)
}

func TestRequestAmountAllowsLargeTopUp(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldDB := model.DB
	oldMainDatabase := common.MainDatabaseType()
	oldLogDatabase := common.LogDatabaseType()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		model.DB = oldDB
		common.SetDatabaseTypes(oldMainDatabase, oldLogDatabase)
		model.InitDBColumns()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	require.NoError(t, model.DB.Create(&model.User{
		Id: 41, Username: "large_topup_user", Group: "default", AuthVersion: 1,
		Quota: 1_000_000, Status: common.UserStatusEnabled,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/amount",
		strings.NewReader(`{"amount":50000}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 41)

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
}

func TestRequestAmountRejectsTopUpThatWouldExceedInt64Wallet(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldDB := model.DB
	oldMainDatabase := common.MainDatabaseType()
	oldLogDatabase := common.LogDatabaseType()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		model.DB = oldDB
		common.SetDatabaseTypes(oldMainDatabase, oldLogDatabase)
		model.InitDBColumns()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id: 42, Username: "topup_capacity_user", Group: "default", AuthVersion: 1,
		Quota: common.MaxWalletQuota - 1_000, Status: common.UserStatusEnabled,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 42)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user/amount",
		strings.NewReader(`{"amount":10000}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `top-up quota limit exceeded`)
}

func TestValidateCreditedQuotaAllowsLargeWalletQuota(t *testing.T) {
	quota, err := validateCreditedQuota(decimal.NewFromInt(25_000_000_000))
	require.NoError(t, err)
	assert.Equal(t, int64(25_000_000_000), quota)
	_, err = validateCreditedQuota(decimal.Zero)
	require.EqualError(t, err, "充值额度必须大于 0")
	_, err = validateCreditedQuota(decimal.NewFromInt(common.MaxWalletQuota).Add(decimal.NewFromInt(1)))
	require.EqualError(
		t,
		err,
		"充值额度超出系统可表示范围",
	)
}

func TestStripeCreditedQuotaIncludesGroupRatio(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldTopupGroupRatio := common.TopupGroupRatio2JSONString()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":2}`))
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(oldTopupGroupRatio))
	})

	_, err := validateCreditedQuota(getStripeCreditedQuota(2147, "vip"))
	require.NoError(t, err)
	_, err = validateCreditedQuota(getStripeCreditedQuota(10_000, "vip"))
	require.NoError(t, err)
	_, err = validateCreditedQuota(getStripeCreditedQuota(50_000, "vip"))
	require.NoError(t, err)
	_, err = validateCreditedQuota(getStripeCreditedQuota(9_223_372_036_856, "vip"))
	require.Error(t, err)

	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"free":0}`))
	assert.True(t, decimal.NewFromInt(500000).Equal(getStripeCreditedQuota(1, "free")))
}

func TestStripeCreditedQuotaTokensUsesQuotaUnitsDirectly(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	oldTopupGroupRatio := common.TopupGroupRatio2JSONString()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":2}`))
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplayType
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(oldTopupGroupRatio))
	})

	assert.Equal(t, decimal.NewFromInt(3_000_000), getStripeCreditedQuota(1_500_000, "vip"))
}

package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBenefitActivityRequestUsesYuanAmountsAndCalculatesQuota(t *testing.T) {
	originalRate := operation_setting.USDExchangeRate
	originalQuotaPerUnit := common.QuotaPerUnit
	operation_setting.USDExchangeRate = 7.5
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = originalRate
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	request := benefitActivityRequest{}
	require.NoError(t, request.TotalAmount.UnmarshalJSON([]byte("7.50")))
	require.NoError(t, request.FixedAmount.UnmarshalJSON([]byte("1.25")))
	require.NoError(t, request.MinAmount.UnmarshalJSON([]byte("1.00")))
	require.NoError(t, request.MaxAmount.UnmarshalJSON([]byte("2.00")))
	require.NoError(t, request.ClaimPaidThreshold.UnmarshalJSON([]byte("0.50")))
	request.AmountMode = model.BenefitAmountModeFixed
	request.TotalCount = 6

	activity, err := request.toModel()
	require.NoError(t, err)
	assert.Equal(t, int64(750), activity.TotalAmountCents)
	assert.Equal(t, int64(125), activity.FixedAmountCents)
	assert.Equal(t, int64(50), activity.ClaimPaidThresholdCents)
	assert.Equal(t, int64(500000), activity.TotalQuota)
}

func TestBenefitActivityRequestRejectsMoreThanTwoDecimalPlaces(t *testing.T) {
	request := benefitActivityRequest{}
	require.NoError(t, request.TotalAmount.UnmarshalJSON([]byte("1.001")))
	_, err := request.toModel()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "两位小数")
}

func openBenefitControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	db, err := gorm.Open(sqlite.Open("file:benefit_controller_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&model.Group{}, &model.GroupAlias{}, &model.AutoGroupMember{},
		&model.BenefitActivity{}, &model.BenefitActivityShare{},
		&model.BenefitUserVoucher{}, &model.BenefitVoucherLedger{},
		&model.User{}, &model.TopUp{},
	))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		model.InitDBColumns()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newBenefitControllerUser(t *testing.T, db *gorm.DB, id int) *model.User {
	t.Helper()
	group := &model.Group{Code: "benefit", Name: "活动福利", Ratio: 1, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	user := &model.User{Id: id, Username: "benefit-controller-user", Password: "password123", Group: group.Code, GroupId: group.Id, AffCode: "benefit-controller-aff", CreatedAt: 1, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	return user
}

func newBenefitControllerContext(t *testing.T, method, path string, body []byte, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	ctx.Set("id", userID)
	ctx.Set("username", "benefit-controller-user")
	return ctx, recorder
}

func TestCreateBenefitAdminActivityAcceptsYuanAmountsWithoutQuotaInput(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	originalRate := operation_setting.USDExchangeRate
	originalQuotaPerUnit := common.QuotaPerUnit
	operation_setting.USDExchangeRate = 7.5
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = originalRate
		common.QuotaPerUnit = originalQuotaPerUnit
	})
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 20)
	now := common.GetTimestamp()
	body := []byte(`{"name":"元金额活动","description":"测试","group_id":` + strconv.Itoa(user.GroupId) + `,"amount_mode":"fixed","total_amount":7.5,"fixed_amount":1.25,"min_amount":0,"max_amount":0,"total_count":6,"claim_paid_threshold":0,"personal_valid_seconds":3600,"starts_at":` + strconv.FormatInt(now, 10) + `,"ends_at":` + strconv.FormatInt(now+3600, 10) + `}`)
	ctx, recorder := newBenefitControllerContext(t, http.MethodPost, "/api/benefit/admin/activities", body, user.Id)
	CreateBenefitAdminActivity(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var stored model.BenefitActivity
	require.NoError(t, db.Where("name = ?", "元金额活动").First(&stored).Error)
	assert.Equal(t, int64(750), stored.TotalAmountCents)
	assert.Equal(t, int64(500000), stored.TotalQuota)
	assert.Contains(t, recorder.Body.String(), `"total_amount":7.5`)
}

func TestGetBenefitActivitiesReturnsUserEligibilityState(t *testing.T) {
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 17)
	activity := &model.BenefitActivity{
		Name: "周末福利", GroupId: user.GroupId, AmountMode: model.BenefitAmountModeFixed,
		TotalAmountCents: 300, TotalQuota: 3000, TotalCount: 3, FixedAmountCents: 100,
		PersonalValidSeconds: 3600, StartsAt: time.Now().Unix() - 100, EndsAt: time.Now().Unix() + 3000,
	}
	require.NoError(t, model.CreateBenefitActivity(activity, 1, 900))
	_, err := model.PublishBenefitActivity(activity.Id, 1, 950)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.TopUp{UserId: user.Id, Money: 10, ActualMoney: 10, PaidAmountCNY: 10, Status: common.TopUpStatusSuccess, TradeNo: "benefit-controller-paid"}).Error)

	ctx, recorder := newBenefitControllerContext(t, http.MethodGet, "/api/benefit/activities", nil, user.Id)
	GetBenefitActivities(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                            `json:"success"`
		Data    []model.BenefitActivityUserView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data, 1)
	assert.True(t, response.Data[0].Eligible)
}

func TestClaimBenefitActivityReturnsGenericIneligibleMessage(t *testing.T) {
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 17)
	activity := &model.BenefitActivity{
		Name: "门槛福利", GroupId: user.GroupId, AmountMode: model.BenefitAmountModeFixed,
		TotalAmountCents: 100, TotalQuota: 1000, TotalCount: 1, FixedAmountCents: 100,
		ClaimPaidThresholdCents: 1000, PersonalValidSeconds: 3600, StartsAt: time.Now().Unix() - 100, EndsAt: time.Now().Unix() + 3000,
	}
	require.NoError(t, model.CreateBenefitActivity(activity, 1, 900))
	_, err := model.PublishBenefitActivity(activity.Id, 1, 950)
	require.NoError(t, err)

	ctx, recorder := newBenefitControllerContext(t, http.MethodPost, "/api/benefit/activities/1/claim", []byte(`{}`), user.Id)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ClaimBenefitActivity(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response["success"])
	assert.Equal(t, "不符合领取条件", response["message"])
}

func TestTerminateBenefitAdminActivityIgnoresClientProvidedNow(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 18)
	activity := &model.BenefitActivity{
		Name: "时间审计", GroupId: user.GroupId, AmountMode: model.BenefitAmountModeFixed,
		TotalAmountCents: 100, TotalQuota: 1000, TotalCount: 1, FixedAmountCents: 100,
		PersonalValidSeconds: 3600,
		StartsAt:             time.Now().Add(-time.Hour).Unix(), EndsAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.CreateBenefitActivity(activity, user.Id, time.Now().Add(-time.Minute).Unix()))
	_, err := model.PublishBenefitActivity(activity.Id, user.Id, time.Now().Add(-time.Minute).Unix())
	require.NoError(t, err)

	ctx, recorder := newBenefitControllerContext(t, http.MethodPost, "/api/benefit/admin/activities/1/terminate", []byte(`{"mode":"all","confirm":true,"reason":"审计测试","now":1}`), user.Id)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(activity.Id)}}
	started := common.GetTimestamp()
	TerminateBenefitAdminActivity(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var stored model.BenefitActivity
	require.NoError(t, db.First(&stored, activity.Id).Error)
	assert.GreaterOrEqual(t, stored.TerminatedAt, started)
	assert.NotEqual(t, int64(1), stored.TerminatedAt)
}

func TestPauseBenefitAdminActivityIgnoresClientProvidedNow(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 19)
	createdAt := time.Now().Add(-time.Minute).Unix()
	activity := &model.BenefitActivity{
		Name: "暂停时间审计", GroupId: user.GroupId, AmountMode: model.BenefitAmountModeFixed,
		TotalAmountCents: 100, TotalQuota: 1000, TotalCount: 1, FixedAmountCents: 100,
		PersonalValidSeconds: 3600,
		StartsAt:             time.Now().Add(-time.Hour).Unix(), EndsAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.CreateBenefitActivity(activity, user.Id, createdAt))
	_, err := model.PublishBenefitActivity(activity.Id, user.Id, createdAt)
	require.NoError(t, err)

	ctx, recorder := newBenefitControllerContext(t, http.MethodPost, "/api/benefit/admin/activities/1/pause", []byte(`{"now":1}`), user.Id)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(activity.Id)}}
	started := common.GetTimestamp()
	PauseBenefitAdminActivity(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var stored model.BenefitActivity
	require.NoError(t, db.First(&stored, activity.Id).Error)
	assert.Equal(t, model.BenefitActivityStatusPaused, stored.Status)
	assert.GreaterOrEqual(t, stored.UpdatedAt, started)
	assert.NotEqual(t, int64(1), stored.UpdatedAt)
}

func TestBenefitLookupDoesNotBreakLegacyDatabaseWithoutBenefitTables(t *testing.T) {
	oldDB := model.DB
	t.Cleanup(func() { model.DB = oldDB })
	db, err := gorm.Open(sqlite.Open("file:benefit_legacy_lookup?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db

	available, err := model.GetBenefitVoucherAvailableQuota(1, 1, 100)
	require.NoError(t, err)
	assert.Zero(t, available)
}

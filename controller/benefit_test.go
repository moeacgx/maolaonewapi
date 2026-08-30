package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

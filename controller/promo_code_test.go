package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePromoCodeCannotForgeLifecycleCounts(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	promo := &model.PromoCode{
		Name:           "admin update guard",
		Code:           "ADMIN_UPDATE_GUARD",
		Status:         common.RedemptionCodeStatusEnabled,
		DiscountType:   model.PromoCodeDiscountTypePercent,
		DiscountValue:  10,
		AppliesToTopup: true,
		MaxRedeemCount: 10,
	}
	require.NoError(t, promo.Insert())
	require.NoError(t, db.Model(promo).Updates(map[string]interface{}{
		"redeemed_count": 2,
		"reserved_count": 1,
	}).Error)

	requestBody := `{"id":` + strconv.Itoa(promo.Id) + `,"name":"updated name","code":"ADMIN_UPDATE_GUARD","status":1,"discount_type":"percent","discount_value":15,"applies_to_topup":true,"applies_to_all_subscription":false,"subscription_plan_ids":"","max_redeem_count":10,"redeemed_count":99,"reserved_count":99,"expired_time":0}`

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/promo_code/", strings.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdatePromoCode(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var stored model.PromoCode
	require.NoError(t, db.First(&stored, promo.Id).Error)
	assert.Equal(t, "updated name", stored.Name)
	assert.Equal(t, 2, stored.RedeemedCount)
	assert.Equal(t, 1, stored.ReservedCount)
}

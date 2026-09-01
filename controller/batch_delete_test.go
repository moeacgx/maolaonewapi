package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBatchDeleteIDs(t *testing.T) {
	ids, err := normalizeBatchDeleteIDs("优惠码", []int{3, 1, 3, 2})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, ids)

	_, err = normalizeBatchDeleteIDs("优惠码", nil)
	require.EqualError(t, err, "优惠码 ID 数量必须为 1-500")

	_, err = normalizeBatchDeleteIDs("优惠码", []int{1, 0})
	require.EqualError(t, err, "优惠码 ID 必须为正整数")
}

func TestNormalizeBatchDeleteIDsRejectsTooManyIDs(t *testing.T) {
	ids := make([]int, maxBatchDeleteIDs+1)
	for i := range ids {
		ids[i] = i + 1
	}
	_, err := normalizeBatchDeleteIDs("福利活动", ids)
	require.EqualError(t, err, "福利活动 ID 数量必须为 1-500")
}

func TestBuildBatchDeleteResultReportsRequestedIDsNotFound(t *testing.T) {
	result := buildBatchDeleteResult([]int{1, 2, 3}, []int{1, 3})
	require.Equal(t, []int{1, 3}, result.DeletedIds)
	require.Equal(t, []model.BatchDeleteSkipped{{Id: 2, Reason: "not_found"}}, result.Skipped)
}

func TestBatchDeletePromoCodesReturnsNotFoundForMissingRequestedID(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	db := openBenefitControllerTestDB(t)
	admin := newBenefitControllerUser(t, db, 900)
	require.NoError(t, db.AutoMigrate(&model.PromoCode{}))
	promo := &model.PromoCode{
		Name: "批量删除测试", Code: "BATCH_RESULT_PROMO", Status: 1,
		DiscountType: model.PromoCodeDiscountTypePercent, DiscountValue: 10,
		AppliesToTopup: true,
	}
	require.NoError(t, db.Create(promo).Error)

	ctx, recorder := newBenefitControllerContext(t, http.MethodDelete, "/api/promo_code/batch", []byte(`{"ids":[`+strconv.Itoa(promo.Id)+`,999999]}`), admin.Id)
	BatchDeletePromoCodes(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data model.BatchDeleteResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, []int{promo.Id}, payload.Data.DeletedIds)
	assert.Equal(t, []model.BatchDeleteSkipped{{Id: 999999, Reason: "not_found"}}, payload.Data.Skipped)
}

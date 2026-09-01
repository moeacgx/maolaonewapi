package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBenefitVoucherLedgerRejectsOtherUser(t *testing.T) {
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 701)
	other := &model.User{Id: 702, Username: "other", Status: 1, GroupId: user.GroupId, Group: user.Group}
	require.NoError(t, db.Create(other).Error)
	activity := &model.BenefitActivity{Name: "流水活动", GroupId: user.GroupId, Status: model.BenefitActivityStatusEnded}
	require.NoError(t, db.Create(activity).Error)
	voucher := &model.BenefitUserVoucher{ActivityId: activity.Id, UserId: user.Id, OriginalQuota: 100, RemainingQuota: 100, Status: model.BenefitVoucherStatusActive}
	require.NoError(t, db.Create(voucher).Error)
	require.NoError(t, db.Create(&model.BenefitVoucherLedger{ActivityId: activity.Id, VoucherId: voucher.Id, UserId: user.Id, Type: model.BenefitLedgerTypeVoid, QuotaDelta: 0, BalanceAfter: 100}).Error)
	_, modelErr := model.GetBenefitVoucherLedgerForUser(voucher.Id, other.Id)
	require.ErrorIs(t, modelErr, model.ErrBenefitVoucherForbidden)

	ctx, recorder := newBenefitControllerContext(t, http.MethodGet, "/api/benefit/vouchers/1/ledger", nil, other.Id)
	assert.Equal(t, other.Id, ctx.GetInt("id"))
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(voucher.Id)}}
	GetBenefitVoucherLedger(ctx)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestGetBenefitVoucherLedgerStripsAdminMetadataForOwner(t *testing.T) {
	db := openBenefitControllerTestDB(t)
	user := newBenefitControllerUser(t, db, 703)
	activity := &model.BenefitActivity{Name: "流水活动", GroupId: user.GroupId, Status: model.BenefitActivityStatusEnded}
	require.NoError(t, db.Create(activity).Error)
	voucher := &model.BenefitUserVoucher{ActivityId: activity.Id, UserId: user.Id, OriginalQuota: 100, RemainingQuota: 100, Status: model.BenefitVoucherStatusActive}
	require.NoError(t, db.Create(voucher).Error)
	require.NoError(t, db.Create(&model.BenefitVoucherLedger{ActivityId: activity.Id, VoucherId: voucher.Id, UserId: user.Id, Type: model.BenefitLedgerTypeVoid, Metadata: `{"operator_id":99,"reason":"secret"}`}).Error)

	ctx, recorder := newBenefitControllerContext(t, http.MethodGet, "/api/benefit/vouchers/1/ledger", nil, user.Id)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(voucher.Id)}}
	GetBenefitVoucherLedger(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.NotContains(t, recorder.Body.String(), "operator_id")
}

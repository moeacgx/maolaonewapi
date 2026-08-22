package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGameControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Log{}, &model.GameWallet{}, &model.GameWalletTransaction{},
		&model.GamePrediction{}, &model.GamePredictionOption{}, &model.GamePredictionBet{},
	))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch
		common.SetDatabaseTypes(oldMainType, oldLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedGameControllerUser(t *testing.T, db *gorm.DB, id int, quota int) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id: id, Username: fmt.Sprintf("game_controller_%d", id), Quota: int64(quota),
		Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("game_controller_aff_%d", id),
	}).Error)
}

func gameControllerContext(t *testing.T, method string, target string, body string, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	return ctx, recorder
}

func decodeGameControllerResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetGameWalletResponseShape(t *testing.T) {
	db := setupGameControllerTest(t)
	seedGameControllerUser(t, db, 1, 0)
	ctx, recorder := gameControllerContext(t, http.MethodGet, "/api/game/wallet", "", 1)

	GetGameWallet(ctx)

	response := decodeGameControllerResponse(t, recorder)
	assert.ElementsMatch(t, []string{"success", "message", "data"}, mapKeys(response))
	assert.Equal(t, true, response["success"])
	data := response["data"].(map[string]any)
	assert.ElementsMatch(t, []string{"id", "user_id", "balance", "created_at", "updated_at"}, mapKeys(data))
	assert.EqualValues(t, 1, data["user_id"])
}

func TestGameTransactionsControllerCannotReadAnotherUserLedger(t *testing.T) {
	db := setupGameControllerTest(t)
	seedGameControllerUser(t, db, 1, 0)
	seedGameControllerUser(t, db, 2, 0)
	wallet1 := model.GameWallet{UserID: 1}
	wallet2 := model.GameWallet{UserID: 2}
	require.NoError(t, db.Create(&wallet1).Error)
	require.NoError(t, db.Create(&wallet2).Error)
	require.NoError(t, db.Create(&[]model.GameWalletTransaction{
		{UserID: 1, WalletID: wallet1.ID, Type: model.GameWalletTransactionTypeBet, TokenAmount: 5},
		{UserID: 2, WalletID: wallet2.ID, Type: model.GameWalletTransactionTypeBet, TokenAmount: 9},
	}).Error)
	ctx, recorder := gameControllerContext(t, http.MethodGet, "/api/game/transactions?p=1&page_size=20", "", 1)

	GetGameWalletTransactions(ctx)

	response := decodeGameControllerResponse(t, recorder)
	data := response["data"].(map[string]any)
	assert.ElementsMatch(t, []string{"page", "page_size", "total", "items"}, mapKeys(data))
	assert.EqualValues(t, 1, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 1)
	transaction := items[0].(map[string]any)
	assert.EqualValues(t, 1, transaction["user_id"])
	assert.NotContains(t, transaction, "request_id")
}

func TestGameControllerMasksUnexpectedDatabaseErrors(t *testing.T) {
	db := setupGameControllerTest(t)
	seedGameControllerUser(t, db, 1, 0)
	require.NoError(t, db.Migrator().DropTable(&model.GameWalletTransaction{}))
	ctx, recorder := gameControllerContext(t, http.MethodGet, "/api/game/transactions?p=1&page_size=20", "", 1)

	GetGameWalletTransactions(ctx)

	response := decodeGameControllerResponse(t, recorder)
	assert.Equal(t, false, response["success"])
	assert.Equal(t, "游戏服务暂时不可用", response["message"])
	assert.NotContains(t, recorder.Body.String(), "no such table")
}

func TestGameExchangeControllerSupportsIdempotencyHeaderWithoutChangingBody(t *testing.T) {
	db := setupGameControllerTest(t)
	seedGameControllerUser(t, db, 1, 100)
	invoke := func() map[string]any {
		ctx, recorder := gameControllerContext(t, http.MethodPost, "/api/game/exchange/quota-to-token", `{"quota":10}`, 1)
		ctx.Request.Header.Set("Idempotency-Key", "controller-exchange-retry")
		ExchangeQuotaToGameTokens(ctx)
		return decodeGameControllerResponse(t, recorder)
	}

	first := invoke()
	retry := invoke()

	assert.Equal(t, true, first["success"])
	assert.Equal(t, first["data"].(map[string]any)["id"], retry["data"].(map[string]any)["id"])
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, int64(90), user.Quota)
}

func TestGameExchangeControllerReturnsStableBatchError(t *testing.T) {
	db := setupGameControllerTest(t)
	seedGameControllerUser(t, db, 1, 100)
	common.BatchUpdateEnabled = true
	ctx, recorder := gameControllerContext(t, http.MethodPost, "/api/game/exchange/quota-to-token", `{"quota":10}`, 1)

	ExchangeQuotaToGameTokens(ctx)

	response := decodeGameControllerResponse(t, recorder)
	assert.Equal(t, false, response["success"])
	assert.Equal(t, service.ErrGameBatchQuotaUnsupported.Error(), response["message"])
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, int64(100), user.Quota)
}

func TestGamePredictionDetailResponseKeepsOrderedPublicOptions(t *testing.T) {
	db := setupGameControllerTest(t)
	seedGameControllerUser(t, db, 10, 0)
	prediction, err := service.CreateGamePrediction(service.CreateGamePredictionRequest{
		Title: "公开预测", Description: "公开详情", Options: []string{"先", "后"},
		CloseTime: time.Now().Add(time.Hour).Unix(), CreatedBy: 10,
	})
	require.NoError(t, err)
	ctx, recorder := gameControllerContext(t, http.MethodGet, fmt.Sprintf("/api/game/predictions/%d", prediction.ID), "", 999)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(prediction.ID)}}

	GetGamePrediction(ctx)

	response := decodeGameControllerResponse(t, recorder)
	assert.Equal(t, true, response["success"])
	data := response["data"].(map[string]any)
	options := data["options"].([]any)
	require.Len(t, options, 2)
	assert.EqualValues(t, 1, options[0].(map[string]any)["index"])
	assert.EqualValues(t, 2, options[1].(map[string]any)["index"])
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

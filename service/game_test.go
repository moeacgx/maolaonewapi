package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/game_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGameTest(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	require.NoError(t, model.DB.AutoMigrate(
		&model.User{},
		&model.Log{},
		&model.GameWallet{},
		&model.GameWalletTransaction{},
		&model.GamePrediction{},
		&model.GamePredictionOption{},
		&model.GamePredictionBet{},
	))
	require.NoError(t, model.DB.Exec("DELETE FROM game_prediction_bets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_prediction_options").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_predictions").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_wallet_transactions").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM game_wallets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users").Error)
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM game_prediction_bets").Error
		_ = model.DB.Exec("DELETE FROM game_prediction_options").Error
		_ = model.DB.Exec("DELETE FROM game_predictions").Error
		_ = model.DB.Exec("DELETE FROM game_wallet_transactions").Error
		_ = model.DB.Exec("DELETE FROM game_wallets").Error
		_ = model.DB.Exec("DELETE FROM users").Error
		_ = model.LOG_DB.Exec("DELETE FROM logs").Error
	})
	game_setting.SetTokenExchangeRateForTest(100)
	game_setting.SetAwardFeeRateForTest(0.10)
}

func seedGameUser(t *testing.T, id int, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{
		Id:       id,
		Username: fmt.Sprintf("game_user_%d", id),
		Quota:    int64(quota),
		Status:   common.UserStatusEnabled,
		AffCode:  fmt.Sprintf("game_aff_%d", id),
	}).Error)
}

func getGameUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return int(user.Quota)
}

func getGameWalletBalance(t *testing.T, userID int) int64 {
	t.Helper()
	var wallet model.GameWallet
	require.NoError(t, model.DB.Where("user_id = ?", userID).First(&wallet).Error)
	return wallet.Balance
}

func closeGamePredictionForTest(t *testing.T, predictionID int) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", predictionID).Update("close_time", time.Now().Add(-time.Minute).Unix()).Error)
}

func createSettledGamePredictionFixture(t *testing.T) int {
	t.Helper()
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:       "明天苹果会开发布会吗？",
		Description: "以公开新闻为准",
		Options:     []string{"确实会开", "不会开"},
		CloseTime:   time.Now().Add(time.Hour).Unix(),
		SettleTime:  time.Now().Add(2 * time.Hour).Unix(),
		JudgeMode:   model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)

	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 1000}).Error)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 2, Balance: 1000}).Error)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 100)
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(2, prediction.ID, prediction.Options[1].ID, 300)
	require.NoError(t, err)

	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)
	return prediction.ID
}

func TestGameExchangeQuotaToTokensMovesBalance(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)

	tx, err := ExchangeQuotaToGameTokens(1, 500)

	require.NoError(t, err)
	assert.Equal(t, model.GameWalletTransactionTypeExchangeIn, tx.Type)
	assert.Equal(t, 500, tx.QuotaAmount)
	assert.EqualValues(t, 50000, tx.TokenAmount)
	assert.Equal(t, 500, getGameUserQuota(t, 1))
	assert.EqualValues(t, 50000, getGameWalletBalance(t, 1))
}

func TestGameExchangeQuotaToTokensAcceptsMaxQuotaBoundary(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, common.MaxQuota)

	tx, err := ExchangeQuotaToGameTokens(1, common.MaxQuota)
	require.NoError(t, err)
	assert.Equal(t, common.MaxQuota, tx.QuotaAmount)
	assert.Zero(t, getGameUserQuota(t, 1))
	assert.EqualValues(t, int64(common.MaxQuota)*100, getGameWalletBalance(t, 1))
}

func TestGameExchangeQuotaToTokensRejectsBatchModeWithoutMutation(t *testing.T) {
	setupGameTest(t)
	userID := 990001
	seedGameUser(t, userID, 1000)
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = false })

	_, err := ExchangeQuotaToGameTokensWithRequestID(userID, 500, "batch-rejected")

	require.ErrorIs(t, err, ErrGameBatchQuotaUnsupported)
	assert.Equal(t, 1000, getGameUserQuota(t, userID))
	var wallets, transactions int64
	require.NoError(t, model.DB.Model(&model.GameWallet{}).Where("user_id = ?", userID).Count(&wallets).Error)
	require.NoError(t, model.DB.Model(&model.GameWalletTransaction{}).Where("user_id = ?", userID).Count(&transactions).Error)
	assert.Zero(t, wallets)
	assert.Zero(t, transactions)
}

func TestGameExchangeTokensToQuotaRejectsBatchModeWithoutMutation(t *testing.T) {
	setupGameTest(t)
	userID := 990002
	seedGameUser(t, userID, 1000)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: userID, Balance: 50000}).Error)
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = false })

	_, err := ExchangeGameTokensToQuotaWithRequestID(userID, 10000, "batch-rejected")

	require.ErrorIs(t, err, ErrGameBatchQuotaUnsupported)
	assert.Equal(t, 1000, getGameUserQuota(t, userID))
	assert.EqualValues(t, 50000, getGameWalletBalance(t, userID))
	var transactions int64
	require.NoError(t, model.DB.Model(&model.GameWalletTransaction{}).Where("user_id = ?", userID).Count(&transactions).Error)
	assert.Zero(t, transactions)
}

func TestGameExchangeTokensToQuotaMovesBalance(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 50000}).Error)

	tx, err := ExchangeGameTokensToQuota(1, 20000)

	require.NoError(t, err)
	assert.Equal(t, model.GameWalletTransactionTypeExchangeOut, tx.Type)
	assert.EqualValues(t, 20000, tx.TokenAmount)
	assert.Equal(t, 200, tx.QuotaAmount)
	assert.Equal(t, 1200, getGameUserQuota(t, 1))
	assert.EqualValues(t, 30000, getGameWalletBalance(t, 1))
}

func TestGameExchangeTokensToQuotaRequiresWholeExchangeUnit(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 50000}).Error)

	_, err := ExchangeGameTokensToQuota(1, 20050)

	require.ErrorIs(t, err, ErrGameInvalidAmount)
	assert.Equal(t, 1000, getGameUserQuota(t, 1))
	assert.EqualValues(t, 50000, getGameWalletBalance(t, 1))
}

func TestGameExchangeQuotaToTokensRejectsWalletOverflow(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	nearMax := int64(math.MaxInt64 - 10)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: nearMax}).Error)

	_, err := ExchangeQuotaToGameTokens(1, 1)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.Equal(t, 1000, getGameUserQuota(t, 1))
	assert.EqualValues(t, nearMax, getGameWalletBalance(t, 1))
}

func TestGameExchangeTokensToQuotaEnforcesDatabaseQuotaLimit(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, common.MaxQuota-1)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 300}).Error)

	tx, err := ExchangeGameTokensToQuotaWithRequestID(1, 100, "quota-max-boundary")
	require.NoError(t, err)
	assert.Equal(t, 1, tx.QuotaAmount)
	assert.Equal(t, common.MaxQuota, getGameUserQuota(t, 1))
	assert.EqualValues(t, 200, getGameWalletBalance(t, 1))

	_, err = ExchangeGameTokensToQuotaWithRequestID(1, 100, "quota-overflow")
	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.Equal(t, common.MaxQuota, getGameUserQuota(t, 1))
	assert.EqualValues(t, 200, getGameWalletBalance(t, 1))
}

func TestGameExchangeTokensToQuotaRejectsConversionAboveDatabaseLimit(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	tokens := (int64(common.MaxQuota) + 1) * 100
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: tokens}).Error)

	_, err := ExchangeGameTokensToQuota(1, tokens)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.Zero(t, getGameUserQuota(t, 1))
	assert.Equal(t, tokens, getGameWalletBalance(t, 1))
}

func TestGameCreatePredictionValidatesAndTrimsServerSide(t *testing.T) {
	setupGameTest(t)

	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:       "  明天苹果会开发布会吗？  ",
		Description: "  以公开新闻为准  ",
		Options:     []string{"  确实会开  ", "  不会开  "},
		CloseTime:   time.Now().Add(time.Hour).Unix(),
		SettleTime:  time.Now().Add(2 * time.Hour).Unix(),
		JudgeMode:   model.GamePredictionJudgeModeManual,
	})

	require.NoError(t, err)
	assert.Equal(t, "明天苹果会开发布会吗？", prediction.Title)
	assert.Equal(t, "以公开新闻为准", prediction.Description)
	require.Len(t, prediction.Options, 2)
	assert.Equal(t, "确实会开", prediction.Options[0].Title)
	assert.Equal(t, "不会开", prediction.Options[1].Title)
}

func TestGameCreatePredictionRejectsInvalidTimes(t *testing.T) {
	setupGameTest(t)

	_, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "过期预测",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(-time.Minute).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.ErrorIs(t, err, ErrGamePredictionInvalidTime)

	_, err = CreateGamePrediction(CreateGamePredictionRequest{
		Title:      "结算时间错误",
		Options:    []string{"会", "不会"},
		CloseTime:  time.Now().Add(time.Hour).Unix(),
		SettleTime: time.Now().Add(30 * time.Minute).Unix(),
		JudgeMode:  model.GamePredictionJudgeModeManual,
	})
	require.ErrorIs(t, err, ErrGamePredictionInvalidTime)
}

func TestGameSetAnswerRejectsBeforeCloseTime(t *testing.T) {
	setupGameTest(t)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "未封盘不能判题",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)

	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)

	require.Error(t, err)
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.Equal(t, model.GamePredictionStatusOpen, updated.Status)
	assert.Equal(t, 0, updated.AnswerOptionID)
}

func TestGameSettlePredictionRejectsBeforeCloseTime(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "未封盘不能结算",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Updates(map[string]interface{}{
		"answer_option_id": prediction.Options[0].ID,
		"status":           model.GamePredictionStatusAnswered,
	}).Error)

	_, err = SettleGamePrediction(prediction.ID, 99)

	require.Error(t, err)
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.Equal(t, model.GamePredictionStatusAnswered, updated.Status)
	assert.EqualValues(t, 90, getGameWalletBalance(t, 1))
}

func TestGamePlaceBetRejectsInsufficientBalanceWithoutChangingPool(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 50}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "余额不足下注测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)

	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 100)

	require.ErrorIs(t, err, ErrGameInsufficientBalance)
	assert.EqualValues(t, 50, getGameWalletBalance(t, 1))
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, updated.TotalPool)
	assert.EqualValues(t, 0, updated.Options[0].PoolAmount)
	var betCount int64
	require.NoError(t, model.DB.Model(&model.GamePredictionBet{}).Where("prediction_id = ?", prediction.ID).Count(&betCount).Error)
	assert.EqualValues(t, 0, betCount)
}

func TestGamePlaceBetRejectsClosedPredictionWithoutChangingWallet(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "关闭后下注测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Update("close_time", time.Now().Add(-time.Minute).Unix()).Error)

	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)

	require.ErrorIs(t, err, ErrGamePredictionClosed)
	assert.EqualValues(t, 100, getGameWalletBalance(t, 1))
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, updated.TotalPool)
}

func TestGamePlaceBetRejectsPoolOverflowWithoutChangingWallet(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "奖池溢出下注测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	nearMax := int64(math.MaxInt64 - 5)
	require.NoError(t, model.DB.Model(&model.GamePrediction{}).Where("id = ?", prediction.ID).Update("total_pool", nearMax).Error)
	require.NoError(t, model.DB.Model(&model.GamePredictionOption{}).Where("id = ?", prediction.Options[0].ID).Update("pool_amount", nearMax).Error)

	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.EqualValues(t, 100, getGameWalletBalance(t, 1))
	updated, err := GetGamePrediction(prediction.ID)
	require.NoError(t, err)
	assert.EqualValues(t, nearMax, updated.TotalPool)
	assert.EqualValues(t, nearMax, updated.Options[0].PoolAmount)
}

func TestGameSettlePredictionDistributesPoolAndDeductsProfitFee(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	predictionID := createSettledGamePredictionFixture(t)

	result, err := SettleGamePrediction(predictionID, 99)

	require.NoError(t, err)
	assert.EqualValues(t, 400, result.TotalPool)
	assert.EqualValues(t, 100, result.WinningPool)
	assert.EqualValues(t, 1, result.WinnerCount)
	assert.EqualValues(t, 30, result.TotalFee)
	assert.EqualValues(t, 370, result.TotalPayout)
	assert.EqualValues(t, 1270, getGameWalletBalance(t, 1))
	assert.EqualValues(t, 700, getGameWalletBalance(t, 2))

	var payout model.GameWalletTransaction
	require.NoError(t, model.DB.Where("user_id = ? AND type = ?", 1, model.GameWalletTransactionTypePayout).First(&payout).Error)
	assert.EqualValues(t, 400, payout.TokenAmount)
	assert.EqualValues(t, 30, payout.FeeAmount)
	assert.Contains(t, payout.Content, "手续费")
}

func TestGameSettlePredictionAllocatesRemainderToWinners(t *testing.T) {
	setupGameTest(t)
	game_setting.SetAwardFeeRateForTest(0)
	for userID := 1; userID <= 4; userID++ {
		seedGameUser(t, userID, 0)
		require.NoError(t, model.DB.Create(&model.GameWallet{UserID: userID, Balance: 100}).Error)
	}
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "尾差测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	for userID := 1; userID <= 3; userID++ {
		_, err = PlaceGamePredictionBet(userID, prediction.ID, prediction.Options[0].ID, 1)
		require.NoError(t, err)
	}
	_, err = PlaceGamePredictionBet(4, prediction.ID, prediction.Options[1].ID, 97)
	require.NoError(t, err)
	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)

	result, err := SettleGamePrediction(prediction.ID, 99)

	require.NoError(t, err)
	assert.EqualValues(t, 100, result.TotalPool)
	assert.EqualValues(t, 100, result.TotalPayout+result.TotalFee)
	var payouts []model.GamePredictionBet
	require.NoError(t, model.DB.Where("prediction_id = ? AND status = ?", prediction.ID, model.GamePredictionBetStatusWon).Order("id asc").Find(&payouts).Error)
	require.Len(t, payouts, 3)
	assert.EqualValues(t, 100, payouts[0].GrossPayout+payouts[1].GrossPayout+payouts[2].GrossPayout)
}

func TestGameSettlePredictionUsesIntegerMathForLargePools(t *testing.T) {
	setupGameTest(t)
	game_setting.SetAwardFeeRateForTest(0)
	largePool := int64(9_007_199_254_740_993)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: largePool}).Error)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 2, Balance: largePool}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "大额池子精度测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, largePool)
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(2, prediction.ID, prediction.Options[1].ID, 1)
	require.NoError(t, err)
	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)

	result, err := SettleGamePrediction(prediction.ID, 99)

	require.NoError(t, err)
	assert.EqualValues(t, largePool+1, result.TotalPool)
	assert.EqualValues(t, largePool+1, result.TotalPayout)
	var winner model.GamePredictionBet
	require.NoError(t, model.DB.Where("prediction_id = ? AND user_id = ?", prediction.ID, 1).First(&winner).Error)
	assert.EqualValues(t, largePool+1, winner.GrossPayout)
}

func TestGameCreatePredictionRejectsUnsupportedAutoJudge(t *testing.T) {
	setupGameTest(t)

	_, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:      "自动判题不可用",
		Options:    []string{"会", "不会"},
		CloseTime:  time.Now().Add(time.Hour).Unix(),
		SettleTime: time.Now().Add(2 * time.Hour).Unix(),
		JudgeMode:  model.GamePredictionJudgeModeAuto,
	})

	require.ErrorIs(t, err, ErrGameAutoJudgeUnsupported)
}

func TestGameSettlePredictionRejectsWalletOverflow(t *testing.T) {
	setupGameTest(t)
	game_setting.SetAwardFeeRateForTest(0)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: math.MaxInt64}).Error)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 2, Balance: 1}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title:     "派奖溢出测试",
		Options:   []string{"会", "不会"},
		CloseTime: time.Now().Add(time.Hour).Unix(),
		JudgeMode: model.GamePredictionJudgeModeManual,
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 1)
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(2, prediction.ID, prediction.Options[1].ID, 1)
	require.NoError(t, err)
	closeGamePredictionForTest(t, prediction.ID)
	_, err = SetGamePredictionAnswer(prediction.ID, prediction.Options[0].ID, 99)
	require.NoError(t, err)

	_, err = SettleGamePrediction(prediction.ID, 99)

	require.ErrorIs(t, err, ErrGameAmountOverflow)
	assert.EqualValues(t, int64(math.MaxInt64-1), getGameWalletBalance(t, 1))
	var updated model.GamePrediction
	require.NoError(t, model.DB.Where("id = ?", prediction.ID).First(&updated).Error)
	assert.Equal(t, model.GamePredictionStatusAnswered, updated.Status)
}

func TestGameSettlePredictionIsIdempotent(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	predictionID := createSettledGamePredictionFixture(t)

	_, err := SettleGamePrediction(predictionID, 99)
	require.NoError(t, err)
	_, err = SettleGamePrediction(predictionID, 99)

	require.ErrorIs(t, err, ErrGamePredictionAlreadySettled)
	assert.EqualValues(t, 1270, getGameWalletBalance(t, 1))
	var payoutCount int64
	require.NoError(t, model.DB.Model(&model.GameWalletTransaction{}).Where("type = ?", model.GameWalletTransactionTypePayout).Count(&payoutCount).Error)
	assert.EqualValues(t, 1, payoutCount)
}

func TestGameExchangeRequestIDIsIdempotentAndScoped(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 1000)
	seedGameUser(t, 2, 1000)

	first, err := ExchangeQuotaToGameTokensWithRequestID(1, 100, "exchange-retry")
	require.NoError(t, err)
	retry, err := ExchangeQuotaToGameTokensWithRequestID(1, 100, "exchange-retry")
	require.NoError(t, err)
	otherUser, err := ExchangeQuotaToGameTokensWithRequestID(2, 100, "exchange-retry")
	require.NoError(t, err)

	assert.Equal(t, first.ID, retry.ID)
	assert.NotEqual(t, first.ID, otherUser.ID)
	assert.Equal(t, 900, getGameUserQuota(t, 1))
	assert.EqualValues(t, 10000, getGameWalletBalance(t, 1))
	var count int64
	require.NoError(t, model.DB.Model(&model.GameWalletTransaction{}).
		Where("user_id = ? AND type = ?", 1, model.GameWalletTransactionTypeExchangeIn).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	_, err = ExchangeQuotaToGameTokensWithRequestID(1, 101, "exchange-retry")
	require.ErrorIs(t, err, ErrGameIdempotencyConflict)
}

func TestGameBetRequestIDPreventsRetryButNotIntentionalDuplicate(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: 100}).Error)
	prediction, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title: "重试下注", Options: []string{"是", "否"}, CloseTime: time.Now().Add(time.Hour).Unix(),
	})
	require.NoError(t, err)

	first, err := PlaceGamePredictionBetWithRequestID(1, prediction.ID, prediction.Options[0].ID, 10, "bet-retry")
	require.NoError(t, err)
	retry, err := PlaceGamePredictionBetWithRequestID(1, prediction.ID, prediction.Options[0].ID, 10, "bet-retry")
	require.NoError(t, err)
	intentional, err := PlaceGamePredictionBet(1, prediction.ID, prediction.Options[0].ID, 10)
	require.NoError(t, err)

	assert.Equal(t, first.ID, retry.ID)
	assert.NotEqual(t, first.ID, intentional.ID)
	assert.EqualValues(t, 80, getGameWalletBalance(t, 1))
	_, err = PlaceGamePredictionBetWithRequestID(1, prediction.ID, prediction.Options[1].ID, 10, "bet-retry")
	require.ErrorIs(t, err, ErrGameIdempotencyConflict)
}

func TestGameTransactionsAreOwnerScoped(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 100)
	seedGameUser(t, 2, 100)
	_, err := ExchangeQuotaToGameTokens(1, 10)
	require.NoError(t, err)

	page := &common.PageInfo{Page: 1, PageSize: 20}
	ownerItems, ownerTotal, err := ListGameWalletTransactions(1, page)
	require.NoError(t, err)
	otherItems, otherTotal, err := ListGameWalletTransactions(2, page)
	require.NoError(t, err)

	require.Len(t, ownerItems, 1)
	assert.Equal(t, 1, ownerItems[0].UserID)
	assert.EqualValues(t, 1, ownerTotal)
	assert.Empty(t, otherItems)
	assert.Zero(t, otherTotal)
}

func TestGameMutationBoundsAndPredictionOwnership(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 100)
	require.NoError(t, model.DB.Create(&model.GameWallet{UserID: 1, Balance: MaxGameMutationAmount}).Error)

	_, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title: strings.Repeat("x", MaxGamePredictionTitleBytes+1), Options: []string{"是", "否"}, CloseTime: time.Now().Add(time.Hour).Unix(),
	})
	require.ErrorIs(t, err, ErrGameTextTooLong)
	_, err = ExchangeQuotaToGameTokensWithRequestID(1, 1, strings.Repeat("r", MaxGameRequestIDBytes+1))
	require.ErrorIs(t, err, ErrGameInvalidRequestID)

	first, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title: "第一个", Options: []string{"甲", "乙"}, CloseTime: time.Now().Add(time.Hour).Unix(),
	})
	require.NoError(t, err)
	second, err := CreateGamePrediction(CreateGamePredictionRequest{
		Title: "第二个", Options: []string{"丙", "丁"}, CloseTime: time.Now().Add(time.Hour).Unix(),
	})
	require.NoError(t, err)
	_, err = PlaceGamePredictionBet(1, first.ID, second.Options[0].ID, 1)
	require.ErrorIs(t, err, ErrGamePredictionInvalidOption)
	_, err = PlaceGamePredictionBet(1, first.ID, first.Options[0].ID, MaxGameMutationAmount+1)
	require.ErrorIs(t, err, ErrGameInvalidAmount)
}

func TestGameConcurrentWalletCreationReturnsSingleWallet(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)

	const workers = 8
	ids := make(chan int, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wallet, err := GetOrCreateGameWallet(1)
			if err != nil {
				errs <- err
				return
			}
			ids <- wallet.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var walletID int
	for id := range ids {
		if walletID == 0 {
			walletID = id
		}
		assert.Equal(t, walletID, id)
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.GameWallet{}).Where("user_id = ?", 1).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestGameConcurrentSettlementHasSinglePayoutLedger(t *testing.T) {
	setupGameTest(t)
	seedGameUser(t, 1, 0)
	seedGameUser(t, 2, 0)
	predictionID := createSettledGamePredictionFixture(t)

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := SettleGamePrediction(predictionID, 99)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	successes := 0
	alreadySettled := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrGamePredictionAlreadySettled):
			alreadySettled++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, alreadySettled)
	assert.EqualValues(t, 1270, getGameWalletBalance(t, 1))
	var payoutCount int64
	require.NoError(t, model.DB.Model(&model.GameWalletTransaction{}).
		Where("prediction_id = ? AND type = ?", predictionID, model.GameWalletTransactionTypePayout).Count(&payoutCount).Error)
	assert.EqualValues(t, 1, payoutCount)
}

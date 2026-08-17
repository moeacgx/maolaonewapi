package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type gameQuotaExchangeRequest struct {
	Quota     int    `json:"quota"`
	RequestID string `json:"request_id"`
}

type gameTokenExchangeRequest struct {
	Tokens    int64  `json:"tokens"`
	RequestID string `json:"request_id"`
}

type gamePredictionBetRequest struct {
	OptionID  int    `json:"option_id"`
	Amount    int64  `json:"amount"`
	RequestID string `json:"request_id"`
}

type gamePredictionCreateRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Options     []string `json:"options"`
	CloseTime   int64    `json:"close_time"`
	SettleTime  int64    `json:"settle_time"`
	JudgeMode   string   `json:"judge_mode"`
}

type gamePredictionAnswerRequest struct {
	OptionID    int `json:"option_id"`
	AnswerIndex int `json:"answer_index"`
}

func gameMutationRequestID(c *gin.Context, bodyRequestID string) (string, error) {
	bodyRequestID = strings.TrimSpace(bodyRequestID)
	headerRequestID := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if bodyRequestID != "" && headerRequestID != "" && bodyRequestID != headerRequestID {
		return "", service.ErrGameIdempotencyConflict
	}
	if bodyRequestID != "" {
		return bodyRequestID, nil
	}
	return headerRequestID, nil
}

func writeGameError(c *gin.Context, err error) {
	known := []error{
		service.ErrGameUserNotFound,
		service.ErrGameInvalidAmount,
		service.ErrGameInsufficientBalance,
		service.ErrGameInsufficientQuota,
		service.ErrGamePredictionNotFound,
		service.ErrGamePredictionClosed,
		service.ErrGamePredictionAlreadySettled,
		service.ErrGamePredictionAnswerNotSet,
		service.ErrGamePredictionInvalidTime,
		service.ErrGamePredictionInvalidOption,
		service.ErrGamePredictionInvalidOptions,
		service.ErrGamePredictionInvalidQuestion,
		service.ErrGameAmountOverflow,
		service.ErrGameTextTooLong,
		service.ErrGameInvalidRequestID,
		service.ErrGameIdempotencyConflict,
		service.ErrGameAutoJudgeUnsupported,
		service.ErrGameBatchQuotaUnsupported,
	}
	for _, sentinel := range known {
		if errors.Is(err, sentinel) {
			common.ApiError(c, sentinel)
			return
		}
	}
	common.SysLog(fmt.Sprintf("game controller internal error: %v", err))
	common.ApiErrorMsg(c, "游戏服务暂时不可用")
}

func writeGameValidationError(c *gin.Context) {
	common.ApiErrorMsg(c, "游戏请求参数无效")
}

func GetGameWallet(c *gin.Context) {
	wallet, err := service.GetOrCreateGameWallet(c.GetInt("id"))
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, wallet)
}

func GetGameWalletTransactions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	transactions, total, err := service.ListGameWalletTransactions(c.GetInt("id"), pageInfo)
	if err != nil {
		writeGameError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(transactions)
	common.ApiSuccess(c, pageInfo)
}
func ExchangeQuotaToGameTokens(c *gin.Context) {
	var req gameQuotaExchangeRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		writeGameValidationError(c)
		return
	}
	requestID, err := gameMutationRequestID(c, req.RequestID)
	if err != nil {
		writeGameError(c, err)
		return
	}
	tx, err := service.ExchangeQuotaToGameTokensWithRequestID(c.GetInt("id"), req.Quota, requestID)
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, tx)
}
func ExchangeGameTokensToQuota(c *gin.Context) {
	var req gameTokenExchangeRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		writeGameValidationError(c)
		return
	}
	requestID, err := gameMutationRequestID(c, req.RequestID)
	if err != nil {
		writeGameError(c, err)
		return
	}
	tx, err := service.ExchangeGameTokensToQuotaWithRequestID(c.GetInt("id"), req.Tokens, requestID)
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, tx)
}

func ListGamePredictions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	predictions, total, err := service.ListGamePredictionsPage(false, pageInfo)
	if err != nil {
		writeGameError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(predictions)
	common.ApiSuccess(c, pageInfo)
}
func GetGamePrediction(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		writeGameValidationError(c)
		return
	}
	prediction, err := service.GetGamePrediction(id)
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, prediction)
}
func PlaceGamePredictionBet(c *gin.Context) {
	predictionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		writeGameValidationError(c)
		return
	}
	var req gamePredictionBetRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		writeGameValidationError(c)
		return
	}
	requestID, err := gameMutationRequestID(c, req.RequestID)
	if err != nil {
		writeGameError(c, err)
		return
	}
	bet, err := service.PlaceGamePredictionBetWithRequestID(c.GetInt("id"), predictionID, req.OptionID, req.Amount, requestID)
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, bet)
}

func AdminListGamePredictions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	predictions, total, err := service.ListAdminGamePredictionsPage(pageInfo)
	if err != nil {
		writeGameError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(predictions)
	common.ApiSuccess(c, pageInfo)
}
func AdminCreateGamePrediction(c *gin.Context) {
	var req gamePredictionCreateRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		writeGameValidationError(c)
		return
	}
	prediction, err := service.CreateGamePrediction(service.CreateGamePredictionRequest{
		Title:       req.Title,
		Description: req.Description,
		Options:     req.Options,
		CloseTime:   req.CloseTime,
		SettleTime:  req.SettleTime,
		JudgeMode:   req.JudgeMode,
		CreatedBy:   c.GetInt("id"),
	})
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, prediction)
}
func AdminSetGamePredictionAnswer(c *gin.Context) {
	predictionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		writeGameValidationError(c)
		return
	}
	var req gamePredictionAnswerRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		writeGameValidationError(c)
		return
	}
	var prediction *model.GamePrediction
	if req.OptionID > 0 {
		prediction, err = service.SetGamePredictionAnswer(predictionID, req.OptionID, c.GetInt("id"))
	} else {
		prediction, err = service.SetGamePredictionAnswerByIndex(predictionID, req.AnswerIndex, c.GetInt("id"))
	}
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, prediction)
}
func AdminSettleGamePrediction(c *gin.Context) {
	predictionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		writeGameValidationError(c)
		return
	}
	result, err := service.SettleGamePrediction(predictionID, c.GetInt("id"))
	if err != nil {
		writeGameError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

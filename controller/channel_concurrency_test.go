package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelPersistsAndReturnsConcurrencyLimit(t *testing.T) {
	channel := setupLegacyChannelStatusUpdateTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewBufferString(fmt.Sprintf(`{"id":%d,"concurrency_limit":4}`, channel.Id)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ConcurrencyLimit *int `json:"concurrency_limit"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotNil(t, response.Data.ConcurrencyLimit)
	assert.Equal(t, 4, *response.Data.ConcurrencyLimit)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	require.NotNil(t, stored.ConcurrencyLimit)
	assert.Equal(t, 4, *stored.ConcurrencyLimit)
}

func TestEditTagChannelsPersistsConcurrencyLimit(t *testing.T) {
	channel := setupLegacyChannelStatusUpdateTestDB(t)
	tag := "concurrency-tag"
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"tag": tag}).Error)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/tag", bytes.NewBufferString(fmt.Sprintf(`{"tag":%q,"concurrency_limit":6}`, tag)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	EditTagChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	require.NotNil(t, stored.ConcurrencyLimit)
	assert.Equal(t, 6, *stored.ConcurrencyLimit)
}

func TestUpdateChannelRejectsNegativeConcurrencyLimit(t *testing.T) {
	channel := setupLegacyChannelStatusUpdateTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewBufferString(fmt.Sprintf(`{"id":%d,"concurrency_limit":-1}`, channel.Id)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannel(ctx)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.NotEqual(t, -1, stored.GetConcurrencyLimit())
}

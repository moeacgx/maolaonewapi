package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationArchiveRoutesAreRootOnlyAndPreviewCleanContent(t *testing.T) {
	root, admin := setupSecurityAuditRouterTestDB(t)
	oldRateLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = oldRateLimit })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	require.NoError(t, model.DB.Create(&model.Group{Id: 701, Code: "audit-prod", Name: "Audit Prod", Status: model.GroupStatusActive}).Error)
	record, err := service.StoreConversationArchive(t.Context(), service.ConversationArchiveInput{
		RequestId: "archive-router-request", UserId: admin.Id, Username: admin.Username,
		GroupId: 701, GroupCode: "audit-prod", GroupName: "Audit Prod", Model: "gpt-test",
		Protocol: "openai_chat_completions", RawBody: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`), RetentionDays: 30,
	})
	require.NoError(t, err)

	unauthenticated := httptest.NewRecorder()
	engine.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/extensions/conversation-archive/config", nil))
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/extensions/conversation-archive/config", nil)
	adminRequest.Header.Set("Authorization", securityAuditAuthorization(t, admin.Id))
	adminRecorder := httptest.NewRecorder()
	engine.ServeHTTP(adminRecorder, adminRequest)
	require.Equal(t, http.StatusForbidden, adminRecorder.Code)

	rootAuth := securityAuditAuthorization(t, root.Id)
	configRequest := httptest.NewRequest(http.MethodGet, "/api/extensions/conversation-archive/config", nil)
	configRequest.Header.Set("Authorization", rootAuth)
	configRecorder := httptest.NewRecorder()
	engine.ServeHTTP(configRecorder, configRequest)
	require.Equal(t, http.StatusOK, configRecorder.Code)
	var configResponse struct {
		Success bool                                  `json:"success"`
		Data    service.ConversationArchiveConfigView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(configRecorder.Body.Bytes(), &configResponse))
	require.True(t, configResponse.Success)
	require.Equal(t, int64(1), configResponse.Data.ConfigVersion)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/extensions/conversation-archive/conversations?group_code=audit-prod&user_id=502", nil)
	listRequest.Header.Set("Authorization", rootAuth)
	listRecorder := httptest.NewRecorder()
	engine.ServeHTTP(listRecorder, listRequest)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	var listResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.ConversationArchive `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listResponse))
	require.True(t, listResponse.Success)
	require.Len(t, listResponse.Data.Items, 1)
	assert.Empty(t, listResponse.Data.Items[0].Content)

	detailRequest := httptest.NewRequest(http.MethodGet, "/api/extensions/conversation-archive/conversations/"+strconv.FormatInt(record.Id, 10), nil)
	detailRequest.Header.Set("Authorization", rootAuth)
	detailRecorder := httptest.NewRecorder()
	engine.ServeHTTP(detailRecorder, detailRequest)
	require.Equal(t, http.StatusOK, detailRecorder.Code)
	var detailResponse struct {
		Success bool                      `json:"success"`
		Data    model.ConversationArchive `json:"data"`
	}
	require.NoError(t, common.Unmarshal(detailRecorder.Body.Bytes(), &detailResponse))
	require.True(t, detailResponse.Success)
	assert.Contains(t, string(detailResponse.Data.Content), "hello")
}

func TestConversationArchiveListRejectsOversizedUserFilter(t *testing.T) {
	root, _ := setupSecurityAuditRouterTestDB(t)
	oldRateLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = oldRateLimit })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	values := make([]string, service.ConversationArchiveMaxUsers+1)
	for i := range values {
		values[i] = strconv.Itoa(i + 1)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/extensions/conversation-archive/conversations?user_ids="+strings.Join(values, ","), nil)
	request.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestConversationArchiveConfigKeepsOnlyTheConfiguredRecentConversations(t *testing.T) {
	root, _ := setupSecurityAuditRouterTestDB(t)
	oldRateLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = oldRateLimit })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	configRequest := httptest.NewRequest(http.MethodPut, "/api/extensions/conversation-archive/config", strings.NewReader(`{"expected_version":1,"enabled":true,"group_codes":[],"user_ids":[],"max_body_bytes":65536,"retention_days":30,"max_archive_count":2}`))
	configRequest.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	configRequest.Header.Set("Content-Type", "application/json")
	configRecorder := httptest.NewRecorder()
	engine.ServeHTTP(configRecorder, configRequest)
	require.Equal(t, http.StatusOK, configRecorder.Code)

	for _, requestID := range []string{"archive-cap-1", "archive-cap-2", "archive-cap-3"} {
		_, err := service.StoreConversationArchive(t.Context(), service.ConversationArchiveInput{
			RequestId: requestID, UserId: root.Id, GroupCode: "default", Model: "gpt-test",
			Protocol: "openai_chat_completions", RawBody: []byte(`{"messages":[{"role":"user","content":"hello"}]}`), RetentionDays: 30,
		})
		require.NoError(t, err)
	}

	rows, total, err := service.ListConversationArchives(t.Context(), service.ConversationArchiveFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	assert.Equal(t, "archive-cap-3", rows[0].RequestId)
	assert.Equal(t, "archive-cap-2", rows[1].RequestId)
}

func TestConversationArchiveClearRequiresConfirmedRootRequest(t *testing.T) {
	root, admin := setupSecurityAuditRouterTestDB(t)
	oldRateLimit := common.GlobalApiRateLimitEnable
	oldCriticalRateLimit := common.CriticalRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	common.CriticalRateLimitEnable = false
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = oldRateLimit
		common.CriticalRateLimitEnable = oldCriticalRateLimit
	})
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	_, err := service.StoreConversationArchive(t.Context(), service.ConversationArchiveInput{
		RequestId: "archive-clear", UserId: root.Id, GroupCode: "default", Model: "gpt-test",
		Protocol: "openai_chat_completions", RawBody: []byte(`{"messages":[{"role":"user","content":"clear me"}]}`), RetentionDays: 30,
	})
	require.NoError(t, err)

	adminRequest := httptest.NewRequest(http.MethodPost, "/api/extensions/conversation-archive/conversations/clear", strings.NewReader(`{"confirm":true}`))
	adminRequest.Header.Set("Authorization", securityAuditAuthorization(t, admin.Id))
	adminRequest.Header.Set("Content-Type", "application/json")
	adminRecorder := httptest.NewRecorder()
	engine.ServeHTTP(adminRecorder, adminRequest)
	require.Equal(t, http.StatusForbidden, adminRecorder.Code)

	unconfirmedRequest := httptest.NewRequest(http.MethodPost, "/api/extensions/conversation-archive/conversations/clear", strings.NewReader(`{"confirm":false}`))
	unconfirmedRequest.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	unconfirmedRequest.Header.Set("Content-Type", "application/json")
	unconfirmedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(unconfirmedRecorder, unconfirmedRequest)
	require.Equal(t, http.StatusBadRequest, unconfirmedRecorder.Code)

	confirmedRequest := httptest.NewRequest(http.MethodPost, "/api/extensions/conversation-archive/conversations/clear", strings.NewReader(`{"confirm":true}`))
	confirmedRequest.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	confirmedRequest.Header.Set("Content-Type", "application/json")
	confirmedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(confirmedRecorder, confirmedRequest)
	require.Equal(t, http.StatusOK, confirmedRecorder.Code)

	_, total, err := service.ListConversationArchives(t.Context(), service.ConversationArchiveFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Zero(t, total)
}

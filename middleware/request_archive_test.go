package middleware

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func requestArchiveMiddlewareTestLocalPath(t *testing.T, components ...string) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return filepath.Join(append([]string{base}, components...)...)
}

func TestRequestArchiveRunsBeforeKlingConversionAndOnlyQueuesOnce(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditHTTPTestDB(t, guard.URL)
	enableMiddlewareRequestArchive(t, "kling-archive")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/kling/v1/videos/text2video",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")
			c.Next()
		},
		RequestArchive(),
		KlingRequestConvert(),
		PromptAudit(),
		func(c *gin.Context) {
			storage, storageErr := common.GetBodyStorage(c)
			require.NoError(t, storageErr)
			converted, storageErr := storage.Bytes()
			require.NoError(t, storageErr)
			var payload map[string]interface{}
			require.NoError(t, common.Unmarshal(converted, &payload))
			require.Equal(t, "kling-v2", payload["model"])
			require.Contains(t, payload, "metadata")
			c.Status(http.StatusNoContent)
		},
	)
	original := []byte(`{"model_name":"kling-v2","prompt":"a quiet mountain lake","custom":"preserve-me"}`)
	request := httptest.NewRequest(http.MethodPost,
		"/kling/v1/videos/text2video?token=must-not-be-stored", bytes.NewReader(original))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer authorization-must-not-be-stored")
	request.AddCookie(&http.Cookie{Name: "session", Value: "cookie-must-not-be-stored"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	var count int64
	require.NoError(t, model.DB.Model(&model.RequestArchiveJob{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var job model.RequestArchiveJob
	require.NoError(t, model.DB.First(&job).Error)
	plaintext, err := service.DecryptRequestArchivePayload(&job)
	require.NoError(t, err)
	require.Equal(t, original, plaintext)
	require.Equal(t, "/kling/v1/videos/text2video", job.Path)
	require.Equal(t, "application/json", job.ContentType)
	require.NotContains(t, job.Path, "token")
	persistedMetadata, err := common.Marshal(job)
	require.NoError(t, err)
	for _, secret := range []string{
		"must-not-be-stored", "authorization-must-not-be-stored", "cookie-must-not-be-stored",
	} {
		require.NotContains(t, string(persistedMetadata), secret)
		require.NotContains(t, string(job.RequestCiphertext), secret)
	}
}

func TestRequestArchivePreservesJimengGetResultBodyBeforeConversion(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditHTTPTestDB(t, guard.URL)
	enableMiddlewareRequestArchive(t, "jimeng-archive")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/jimeng/",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")
			c.Next()
		},
		RequestArchive(),
		JimengRequestConvert(),
		PromptAudit(),
		func(c *gin.Context) {
			require.Equal(t, http.MethodGet, c.Request.Method)
			require.Equal(t, "/v1/video/generations/task-jimeng-42", c.Request.URL.Path)
			require.Equal(t, "task-jimeng-42", c.GetString("task_id"))
			storage, storageErr := common.GetBodyStorage(c)
			require.NoError(t, storageErr)
			converted, storageErr := storage.Bytes()
			require.NoError(t, storageErr)
			var payload map[string]interface{}
			require.NoError(t, common.Unmarshal(converted, &payload))
			require.Equal(t, "jimeng-video", payload["model"])
			require.Contains(t, payload, "metadata")
			c.Status(http.StatusNoContent)
		},
	)
	original := []byte(`{"req_key":"jimeng-video","prompt":"fetch prompt","task_id":"task-jimeng-42","custom":"preserve-me"}`)
	request := httptest.NewRequest(http.MethodPost,
		"/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31&AccessKey=must-not-be-stored",
		bytes.NewReader(original))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	var jobs []model.RequestArchiveJob
	require.NoError(t, model.DB.Order("id ASC").Find(&jobs).Error)
	require.Len(t, jobs, 1, "协议转换和 PromptAudit 不得重复归档同一请求")
	require.Equal(t, http.MethodPost, jobs[0].Method)
	require.Equal(t, "/jimeng/", jobs[0].Path)
	require.NotContains(t, jobs[0].Path, "AccessKey")
	plaintext, err := service.DecryptRequestArchivePayload(&jobs[0])
	require.NoError(t, err)
	require.Equal(t, original, plaintext)
}

func enableMiddlewareRequestArchive(t *testing.T, targetID string) {
	t.Helper()
	config, err := service.GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = service.SaveRequestArchiveConfig(context.Background(), service.RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
		ActiveTargetId:        targetID,
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []service.RequestArchiveUpdateTarget{{
			Id: targetID, Name: targetID, Type: model.RequestArchiveTargetLocal,
			Enabled: true, LocalPath: requestArchiveMiddlewareTestLocalPath(t, "archive"),
		}},
	}, 1)
	require.NoError(t, err)
}

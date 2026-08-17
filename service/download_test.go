package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDownloadWorkerLogRedactsOriginURL(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodPost, req.Method)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(worker.Close)

	originalWorkerURL := system_setting.WorkerUrl
	originalWorkerKey := system_setting.WorkerValidKey
	originalHTTPClient := httpClient
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	system_setting.WorkerUrl = worker.URL
	system_setting.WorkerValidKey = "worker-test-key"
	httpClient = worker.Client()
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		system_setting.WorkerUrl = originalWorkerURL
		system_setting.WorkerValidKey = originalWorkerKey
		httpClient = originalHTTPClient
		*fetchSetting = originalFetchSetting
	})

	var logBuffer bytes.Buffer
	common.LogWriterMu.Lock()
	originalWriter := gin.DefaultWriter
	gin.DefaultWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = originalWriter
		common.LogWriterMu.Unlock()
	})

	originURL := "https://files.example/archive?token=download-secret&signature=signature-secret"
	resp, err := DoDownloadRequest(originURL, "test download")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())

	logOutput := logBuffer.String()
	require.Contains(t, logOutput, "downloading file from worker: https://***.example/***?")
	require.Contains(t, logOutput, "token=***")
	require.Contains(t, logOutput, "signature=***")
	require.Contains(t, logOutput, "reason: test download")
	require.NotContains(t, logOutput, "download-secret")
	require.NotContains(t, logOutput, "signature-secret")
}

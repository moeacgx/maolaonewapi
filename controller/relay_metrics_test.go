package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskErrorToMetricErrorPreservesFailureStatus(t *testing.T) {
	taskErr := &taskdto.TaskError{
		Code:       "task_not_exist",
		Message:    "origin task does not exist",
		StatusCode: http.StatusBadRequest,
		LocalError: true,
		Error:      errors.New("task_origin_not_exist"),
	}

	metricErr := taskErrorToMetricError(taskErr)

	require.NotNil(t, metricErr)
	require.Equal(t, types.ErrorCodeBadResponseStatusCode, metricErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, metricErr.StatusCode)
	require.Equal(t, taskErr.Error.Error(), metricErr.Error())
	require.Nil(t, taskErrorToMetricError(nil))
}

func TestApplyImageTaskAsyncPreConsumeOnlyAffectsMarkedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	marked, _ := gin.CreateTestContext(httptest.NewRecorder())
	marked.Set(imageTaskAsyncContextKey, true)
	markedInfo := &relaycommon.RelayInfo{}
	applyImageTaskAsyncPreConsume(marked, markedInfo)
	require.True(t, markedInfo.ForcePreConsume)

	unmarked, _ := gin.CreateTestContext(httptest.NewRecorder())
	unmarkedInfo := &relaycommon.RelayInfo{}
	applyImageTaskAsyncPreConsume(unmarked, unmarkedInfo)
	require.False(t, unmarkedInfo.ForcePreConsume)
}

package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestDisplayGroupListUsesTokenGroupDisplayNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyTokenGroupDetails, []model.GroupReference{
		{Code: "codex-pro", Name: "codex-value"},
		{Code: "gemini", Name: "Gemini Official"},
	})

	assert.Equal(t, "codex-value,Gemini Official", DisplayGroupList(c, "codex-pro,gemini"))
}

func TestDisplayGroupNameFallsBackToCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	assert.Equal(t, "missing-group", DisplayGroupName(c, "missing-group"))
}

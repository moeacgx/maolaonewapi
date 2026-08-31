package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetModelRequestReadsCanvasMultipartImageEditModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "edit this image"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("image-data"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/edits", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())

	modelRequest, shouldSelectChannel, err := getModelRequest(context)
	require.NoError(t, err)
	require.True(t, shouldSelectChannel)
	require.Equal(t, "gpt-image-2", modelRequest.Model)
}

func TestGetModelRequestReadsStandardJSONGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","group":"benefit"}`))
	context.Request.Header.Set("Content-Type", gin.MIMEJSON)

	modelRequest, shouldSelectChannel, err := getModelRequest(context)
	require.NoError(t, err)
	require.True(t, shouldSelectChannel)
	require.Equal(t, "gpt-test", modelRequest.Model)
	require.Equal(t, "benefit", modelRequest.Group)
}

func TestStandardChatCompletionsGroupSelectionKeepsBenefitGateSemantics(t *testing.T) {
	for _, test := range []struct {
		name        string
		body        string
		inherited   string
		tokenGroup  string
		wantGroup   string
		wantBenefit bool
	}{
		{name: "explicit", body: `{"model":"gpt-test","group":"default"}`, inherited: "default", tokenGroup: "default", wantGroup: "default", wantBenefit: true},
		{name: "auto", body: `{"model":"gpt-test","group":"auto"}`, inherited: "auto", tokenGroup: "auto", wantGroup: "auto", wantBenefit: false},
		{name: "inherited", body: `{"model":"gpt-test"}`, inherited: "default", wantGroup: "default", wantBenefit: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(test.body))
			ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyTokenGroup, test.tokenGroup)
			if test.tokenGroup == "default" {
				common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"default"})
			}

			request, _, err := getModelRequest(ctx)
			require.NoError(t, err)
			usingGroup, err := applyRequestedGroup(ctx, test.inherited, request.Group)
			require.NoError(t, err)
			require.Equal(t, test.wantGroup, usingGroup)
			require.Equal(t, test.wantBenefit, common.GetContextKeyBool(ctx, constant.ContextKeyBenefitGroupExplicit))
		})
	}
}

func TestApplyRequestedGroupPreservesBenefitGateSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"default"})

	usingGroup, err := applyRequestedGroup(ctx, "default", "default")
	require.NoError(t, err)
	require.Equal(t, "default", usingGroup)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyBenefitGroupExplicit))
	require.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))

	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	usingGroup, err = applyRequestedGroup(ctx, "auto", "auto")
	require.NoError(t, err)
	require.Equal(t, "auto", usingGroup)
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyBenefitGroupExplicit))

	usingGroup, err = applyRequestedGroup(ctx, "default", "")
	require.NoError(t, err)
	require.Equal(t, "default", usingGroup)
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyBenefitGroupExplicit))
}

func TestApplyRequestedGroupRejectsGroupOutsideExplicitTokenBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"vip"})

	_, err := applyRequestedGroup(ctx, "vip", "default")
	require.Error(t, err)
	require.Equal(t, "", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyBenefitGroupExplicit))

	_, err = applyRequestedGroup(ctx, "vip", "auto")
	require.Error(t, err)
}

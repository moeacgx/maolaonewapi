package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelMultiKeyConversion(t *testing.T) {
	tests := []struct {
		name        string
		patch       map[string]any
		wantKey     string
		wantMulti   bool
		wantMode    constant.MultiKeyMode
		wantSuccess bool
	}{
		{"默认追加清理空行并去重", map[string]any{"convert_to_multi_key": true, "key": " new-key \r\n\r\ngroup-display-key\nnew-key\nlast-key "}, "group-display-key\nnew-key\nlast-key", true, constant.MultiKeyModeRandom, true},
		{"留空保留旧密钥", map[string]any{"convert_to_multi_key": true}, "group-display-key", true, constant.MultiKeyModeRandom, true},
		{"显式覆盖并轮询", map[string]any{"convert_to_multi_key": true, "key_mode": "replace", "key": "new-key\r\n next-key ", "multi_key_mode": "polling"}, "new-key\nnext-key", true, constant.MultiKeyModePolling, true},
		{"空覆盖拒绝", map[string]any{"convert_to_multi_key": true, "key_mode": "replace", "key": " \r\n "}, "group-display-key", false, "", false},
		{"非法更新模式拒绝", map[string]any{"convert_to_multi_key": true, "key_mode": "invalid"}, "group-display-key", false, "", false},
		{"非法轮询策略拒绝", map[string]any{"convert_to_multi_key": true, "multi_key_mode": "invalid"}, "group-display-key", false, "", false},
		{"缺省不自动转换", map[string]any{"key": "key-one\nkey-two"}, "key-one\nkey-two", false, "", true},
		{"false不转换且忽略伪造状态", map[string]any{"convert_to_multi_key": false, "channel_info": map[string]any{"is_multi_key": true}}, "group-display-key", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, channel := setupChannelGroupDisplayControllerTestDB(t)
			tt.patch["id"] = channel.Id
			success, response := requestMultiKeyChannelUpdate(t, tt.patch, common.RoleRootUser)
			require.Equal(t, tt.wantSuccess, success, response)
			assert.NotContains(t, response, "group-display-key")
			stored, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, tt.wantKey, stored.Key)
			assert.Equal(t, tt.wantMulti, stored.ChannelInfo.IsMultiKey)
			assert.Equal(t, tt.wantMode, stored.ChannelInfo.MultiKeyMode)
			assert.Equal(t, channel.Status, stored.Status)
			if tt.wantSuccess {
				assertChannelGroupState(t, channel.Id, group)
			}
			if tt.wantMulti {
				assert.Equal(t, len(stored.GetKeys()), stored.ChannelInfo.MultiKeySize)
				key, _, apiErr := stored.GetNextEnabledKey()
				require.Nil(t, apiErr)
				assert.Contains(t, stored.GetKeys(), key)
				if tt.wantMode == constant.MultiKeyModePolling {
					assert.Equal(t, "new-key", key)
					key, _, apiErr = stored.GetNextEnabledKey()
					require.Nil(t, apiErr)
					assert.Equal(t, "next-key", key)
				}
			}
		})
	}
}

func requestMultiKeyChannelUpdate(t *testing.T, patch map[string]any, role int) (bool, string) {
	t.Helper()
	body, err := common.Marshal(patch)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("role", role)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response.Success, recorder.Body.String()
}

func TestUpdateChannelMultiKeyConversionRejectsSpecialCredentials(t *testing.T) {
	for _, sourceType := range []int{constant.ChannelTypeCodex, constant.ChannelTypeVertexAi} {
		for _, targetType := range []int{0, constant.ChannelTypeOpenAI} {
			t.Run(fmt.Sprintf("source_%d_target_%d", sourceType, targetType), func(t *testing.T) {
				_, channel := setupChannelGroupDisplayControllerTestDB(t)
				require.NoError(t, model.DB.Model(channel).Update("type", sourceType).Error)
				patch := map[string]any{"id": channel.Id, "convert_to_multi_key": true}
				if targetType != 0 {
					patch["type"] = targetType
				}
				success, _ := requestMultiKeyChannelUpdate(t, patch, common.RoleRootUser)
				assert.False(t, success)
				stored, err := model.GetChannelById(channel.Id, true)
				require.NoError(t, err)
				assert.False(t, stored.ChannelInfo.IsMultiKey)
				assert.Equal(t, sourceType, stored.Type)
				assert.Equal(t, channel.Key, stored.Key)
			})
		}
	}
}

func TestUpdateChannelMultiKeyConversionRequiresSensitivePermission(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	success, _ := requestMultiKeyChannelUpdate(t, map[string]any{"id": channel.Id, "convert_to_multi_key": true}, common.RoleCommonUser)
	assert.False(t, success)
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Equal(t, channel.Key, stored.Key)
}

func TestUpdateChannelMultiKeyConversionPreservesExistingKeyStatus(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	channel.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModeRandom, MultiKeyStatusList: map[int]int{0: 2}, MultiKeyPollingIndex: 1}
	require.NoError(t, model.DB.Model(channel).Updates(map[string]any{"key": "disabled-key\nworking-key", "channel_info": channel.ChannelInfo}).Error)
	success, response := requestMultiKeyChannelUpdate(t, map[string]any{"id": channel.Id, "convert_to_multi_key": true}, common.RoleRootUser)
	require.True(t, success, response)
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, channel.ChannelInfo.MultiKeyStatusList, stored.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
	key, _, apiErr := stored.GetNextEnabledKey()
	require.Nil(t, apiErr)
	assert.Equal(t, "working-key", key)
}

func TestUpdateChannelMultiKeyConversionRejectsManagedDeployment(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	require.NoError(t, model.DB.Model(channel).Update("other_info", `{"source":"ionet"}`).Error)
	success, _ := requestMultiKeyChannelUpdate(t, map[string]any{"id": channel.Id, "convert_to_multi_key": true, "other_info": ""}, common.RoleRootUser)
	assert.False(t, success)
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Equal(t, `{"source":"ionet"}`, stored.OtherInfo)
}

func TestUpdateChannelMultiKeyConversionFalseDoesNotRequireSensitivePermission(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	success, response := requestMultiKeyChannelUpdate(t, map[string]any{"id": channel.Id, "convert_to_multi_key": false, "weight": 7}, common.RoleAdminUser)
	require.True(t, success, response)
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.False(t, stored.ChannelInfo.IsMultiKey)
	assert.Equal(t, 7, stored.GetWeight())
}

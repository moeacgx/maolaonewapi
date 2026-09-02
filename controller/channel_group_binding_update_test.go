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

type channelGroupUpdateResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Group        string                 `json:"group"`
		GroupIds     []int                  `json:"group_ids"`
		GroupDetails []model.GroupReference `json:"group_details"`
	} `json:"data"`
}

func updateChannelForTest(t *testing.T, body string) channelGroupUpdateResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response channelGroupUpdateResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func assertChannelGroupState(t *testing.T, channelID int, group *model.Group) {
	t.Helper()
	var bindings []model.ChannelGroupBinding
	require.NoError(t, model.DB.Where("channel_id = ?", channelID).Order("position ASC").Find(&bindings).Error)
	require.Len(t, bindings, 1)
	assert.Equal(t, group.Id, bindings[0].GroupId)

	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", channelID).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, group.Id, abilities[0].GroupId)
	assert.Equal(t, group.Code, abilities[0].Group)
}

func TestUpdateChannelPartialFieldsPreserveGroupBindings(t *testing.T) {
	tests := []struct {
		name  string
		field string
	}{
		{name: "priority", field: `"priority":7`},
		{name: "weight", field: `"weight":9`},
		{name: "concurrency_limit", field: `"concurrency_limit":4`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			group, channel := setupChannelGroupDisplayControllerTestDB(t)
			response := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,%s}`, channel.Id, test.field))

			require.True(t, response.Success)
			assert.Equal(t, group.Code, response.Data.Group)
			assert.Equal(t, []int{group.Id}, response.Data.GroupIds)
			require.Len(t, response.Data.GroupDetails, 1)
			assert.Equal(t, group.Code, response.Data.GroupDetails[0].Code)
			assertChannelGroupState(t, channel.Id, group)
		})
	}
}

func TestUpdateChannelMultiKeyPartialFieldPreservesGroupBindings(t *testing.T) {
	group, channel := setupChannelGroupDisplayControllerTestDB(t)
	channel.Key = "key-one\nkey-two"
	channel.ChannelInfo.IsMultiKey = true
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          channel.Key,
		"channel_info": channel.ChannelInfo,
	}).Error)

	response := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"weight":5}`, channel.Id))
	require.True(t, response.Success)
	assert.Equal(t, group.Code, response.Data.Group)
	assert.Equal(t, []int{group.Id}, response.Data.GroupIds)
	assertChannelGroupState(t, channel.Id, group)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 2, stored.ChannelInfo.MultiKeySize)
}

func TestUpdateChannelPersistsVendorID(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	vendorID := 42

	response := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"vendor_id":%d}`, channel.Id, vendorID))
	require.True(t, response.Success)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, stored.VendorID)
	assert.Equal(t, vendorID, *stored.VendorID)
}

func TestUpdateChannelClearsVendorID(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	vendorID := 42
	channel.VendorID = &vendorID
	require.NoError(t, channel.Update())

	response := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"vendor_id":null}`, channel.Id))
	require.True(t, response.Success)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Nil(t, stored.VendorID)
}

func TestUpdateChannelPartialFieldPreservesVendorID(t *testing.T) {
	_, channel := setupChannelGroupDisplayControllerTestDB(t)
	vendorID := 42
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("vendor_id", vendorID).Error)

	response := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"weight":5}`, channel.Id))
	require.True(t, response.Success)

	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, stored.VendorID)
	assert.Equal(t, vendorID, *stored.VendorID)
}

func TestUpdateChannelExplicitGroupReplacementAndEmptyRejection(t *testing.T) {
	oldGroup, channel := setupChannelGroupDisplayControllerTestDB(t)
	newGroup := &model.Group{Code: "group_3", Name: "Third group", Ratio: 1, Status: model.GroupStatusActive}
	require.NoError(t, model.DB.Create(newGroup).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: oldGroup.Code, GroupId: oldGroup.Id, Model: channel.Models, ChannelId: channel.Id,
		Enabled: true,
	}).Error)
	var initialBindings int64
	require.NoError(t, model.DB.Model(&model.ChannelGroupBinding{}).Where("channel_id = ?", channel.Id).Count(&initialBindings).Error)
	require.Equal(t, int64(1), initialBindings)

	emptyIDs := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"group_ids":[]}`, channel.Id))
	assert.False(t, emptyIDs.Success)
	assertChannelGroupState(t, channel.Id, oldGroup)

	emptyGroup := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"group":""}`, channel.Id))
	assert.False(t, emptyGroup.Success)
	assertChannelGroupState(t, channel.Id, oldGroup)

	replaced := updateChannelForTest(t, fmt.Sprintf(`{"id":%d,"group_ids":[%d]}`, channel.Id, newGroup.Id))
	require.True(t, replaced.Success)
	assert.Equal(t, newGroup.Code, replaced.Data.Group)
	assert.Equal(t, []int{newGroup.Id}, replaced.Data.GroupIds)
	require.Len(t, replaced.Data.GroupDetails, 1)
	assert.Equal(t, newGroup.Code, replaced.Data.GroupDetails[0].Code)
	assertChannelGroupState(t, channel.Id, newGroup)
}

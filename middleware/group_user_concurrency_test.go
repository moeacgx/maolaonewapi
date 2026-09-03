package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupUserConcurrencyContextReleasesOnlyItsOwnGroupSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	first := &gin.Context{}
	first.Keys = make(map[string]any)
	first.Set("id", 801)
	group := &model.Group{Id: 901, Code: "benefit", SingleUserConcurrencyLimit: 1}

	require.True(t, tryAcquireGroupUserConcurrencyForContext(first, group))
	second := &gin.Context{}
	second.Keys = make(map[string]any)
	second.Set("id", 801)
	assert.False(t, tryAcquireGroupUserConcurrencyForContext(second, group))

	releaseGroupUserConcurrencyForContext(first)
	assert.True(t, tryAcquireGroupUserConcurrencyForContext(second, group))
	releaseGroupUserConcurrencyForContext(second)
	releaseGroupUserConcurrencyForContext(second)
}

func TestGroupUserConcurrencyContextChangesLeaseWhenRetryMovesGroup(t *testing.T) {
	first := &gin.Context{}
	first.Keys = make(map[string]any)
	first.Set("id", 802)
	groupA := &model.Group{Id: 902, Code: "a", SingleUserConcurrencyLimit: 1}
	groupB := &model.Group{Id: 903, Code: "b", SingleUserConcurrencyLimit: 1}

	require.True(t, tryAcquireGroupUserConcurrencyForContext(first, groupA))
	require.True(t, tryAcquireGroupUserConcurrencyForContext(first, groupB))

	otherA, acquiredA := model.TryAcquireGroupUserConcurrency(802, groupA.Id, 1)
	require.True(t, acquiredA)
	otherA.Release()
	releaseGroupUserConcurrencyForContext(first)

	_, exists := common.GetContextKey(first, constant.ContextKeyGroupUserConcurrency)
	assert.False(t, exists)
}

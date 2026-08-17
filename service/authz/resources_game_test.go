package authz

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameAdminPermissionsFailClosedForUsers(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	assert.True(t, Can(1, common.RoleRootUser, GameAdminRead))
	assert.True(t, Can(1, common.RoleRootUser, GameAdminWrite))
	assert.True(t, Can(2, common.RoleAdminUser, GameAdminRead))
	assert.True(t, Can(2, common.RoleAdminUser, GameAdminWrite))
	assert.False(t, Can(3, common.RoleCommonUser, GameAdminRead))
	assert.False(t, Can(3, common.RoleCommonUser, GameAdminWrite))
}

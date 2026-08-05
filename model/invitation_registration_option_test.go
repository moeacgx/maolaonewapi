package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInvitationRegisterEnabledOptionUpdatesRuntime(t *testing.T) {
	originalValue := common.InvitationRegisterEnabled
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.InvitationRegisterEnabled = originalValue
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()

	require.NoError(t, updateOptionMap("InvitationRegisterEnabled", "true"))
	require.True(t, common.InvitationRegisterEnabled)
	common.OptionMapRWMutex.RLock()
	require.Equal(t, "true", common.OptionMap["InvitationRegisterEnabled"])
	common.OptionMapRWMutex.RUnlock()

	require.NoError(t, updateOptionMap("InvitationRegisterEnabled", "false"))
	require.False(t, common.InvitationRegisterEnabled)
}

func TestInvitationRegistrationRevalidationLocksInviter(t *testing.T) {
	originalUsingSQLite := common.UsingSQLite
	common.UsingSQLite = false
	t.Cleanup(func() { common.UsingSQLite = originalUsingSQLite })

	db, err := gorm.Open(
		postgres.Open("host=localhost port=9911 user=gorm dbname=gorm sslmode=disable"),
		&gorm.Config{DisableAutomaticPing: true, DryRun: true},
	)
	require.NoError(t, err)

	lockedUserRead := false
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(
		"test:invitation_registration_lock",
		func(tx *gorm.DB) {
			if tx.Statement.Table != "users" {
				return
			}
			_, lockedUserRead = tx.Statement.Clauses["FOR"]
		},
	))

	_, _ = GetActiveInviterIdByAffCodeForUpdateWithDB(db, "locked-inviter")
	require.True(t, lockedUserRead)
}

package extension

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func notificationTestManifest() Manifest {
	return Manifest{
		ID:      "orders",
		Name:    "Orders",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		Permissions: PermissionConfig{
			Roles:        []string{"root"},
			Capabilities: []string{CapabilityNotificationEventsPublish},
		},
		Notifications: NotificationContribution{Events: []NotificationEventContribution{{
			ID:              "created",
			Label:           "Order created",
			DefaultTemplate: "{{mention}} order {{order_id}}",
			Variables: []NotificationVariable{{
				Name:     "order_id",
				Type:     "string",
				Required: true,
			}},
		}}},
	}
}

func TestManifestNotificationContributionSchema(t *testing.T) {
	manifest := notificationTestManifest()
	require.NoError(t, manifest.Validate())
	require.Equal(t, "string", manifest.Notifications.Events[0].Variables[0].Type)

	tests := []struct {
		name  string
		apply func(*Manifest)
	}{
		{
			name: "missing capability",
			apply: func(candidate *Manifest) {
				candidate.Permissions.Capabilities = nil
			},
		},
		{
			name: "capability without event",
			apply: func(candidate *Manifest) {
				candidate.Notifications.Events = nil
			},
		},
		{
			name: "reserved variable",
			apply: func(candidate *Manifest) {
				candidate.Notifications.Events[0].Variables = []NotificationVariable{{Name: "mention"}}
				candidate.Notifications.Events[0].DefaultTemplate = "{{mention}}"
			},
		},
		{
			name: "unknown template variable",
			apply: func(candidate *Manifest) {
				candidate.Notifications.Events[0].DefaultTemplate = "{{missing}}"
			},
		},
		{
			name: "duplicate event id",
			apply: func(candidate *Manifest) {
				candidate.Notifications.Events = append(candidate.Notifications.Events, candidate.Notifications.Events[0])
			},
		},
		{
			name: "unsupported variable type",
			apply: func(candidate *Manifest) {
				candidate.Notifications.Events[0].Variables[0].Type = "object"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := notificationTestManifest()
			test.apply(&candidate)
			require.Error(t, candidate.Validate())
		})
	}
}

func TestResolveNotificationEventEnforcesModuleRoleCapabilityAndWhitelist(t *testing.T) {
	manifest := notificationTestManifest()
	require.NoError(t, manifest.Validate())
	manager := &Manager{modules: map[string]Module{
		"orders": {Manifest: manifest, Enabled: true},
	}}

	resolved, err := manager.ResolveNotificationEvent("orders", "created", common.RoleRootUser)
	require.NoError(t, err)
	require.Equal(t, "extension.orders.created", resolved.EventType)

	_, err = manager.ResolveNotificationEvent("orders", "created", common.RoleAdminUser)
	require.ErrorContains(t, err, "无权")
	_, err = manager.ResolveNotificationEvent("orders", "created", common.RoleRootUser+1)
	require.ErrorContains(t, err, "无权")
	_, err = manager.ResolveNotificationEvent("orders", "missing", common.RoleRootUser)
	require.ErrorContains(t, err, "未声明该通知事件")

	module := manager.modules["orders"]
	module.Permissions.Capabilities = nil
	manager.modules["orders"] = module
	_, err = manager.ResolveNotificationEvent("orders", "created", common.RoleRootUser)
	require.ErrorContains(t, err, "未声明通知事件发布能力")

	module.Permissions.Capabilities = []string{CapabilityNotificationEventsPublish}
	module.Enabled = false
	manager.modules["orders"] = module
	_, err = manager.ResolveNotificationEvent("orders", "created", common.RoleRootUser)
	require.ErrorContains(t, err, "未启用")
}

func TestNotificationEventsFiltersDisabledAndInvalidModules(t *testing.T) {
	manifest := notificationTestManifest()
	require.NoError(t, manifest.Validate())
	manager := &Manager{modules: map[string]Module{
		"orders":  {Manifest: manifest, Enabled: true},
		"stopped": {Manifest: manifest, Enabled: false},
		"invalid": {Manifest: manifest, Enabled: true, Error: "C:\\secret\\manifest.json: invalid"},
	}}

	require.Len(t, manager.NotificationEvents(false), 1)
	require.Len(t, manager.NotificationEvents(true), 2)
}

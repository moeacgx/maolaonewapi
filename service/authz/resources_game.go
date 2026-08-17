package authz

const ResourceGame = "game"

var (
	GameAdminRead  = Permission{Resource: ResourceGame, Action: ActionRead}
	GameAdminWrite = Permission{Resource: ResourceGame, Action: ActionWrite}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceGame,
		LabelKey: "Game Management",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read game predictions",
				DescriptionKey: "View all game predictions in the administrative console.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Manage game predictions",
				DescriptionKey: "Create, answer, and settle game predictions.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
}

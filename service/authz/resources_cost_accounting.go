package authz

const (
	ResourceCostAccounting = "cost_accounting"
	ActionReconcile        = "reconcile"
)

var (
	CostAccountingRead      = Permission{Resource: ResourceCostAccounting, Action: ActionRead}
	CostAccountingWrite     = Permission{Resource: ResourceCostAccounting, Action: ActionWrite}
	CostAccountingReconcile = Permission{Resource: ResourceCostAccounting, Action: ActionReconcile}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceCostAccounting,
		LabelKey: "Cost accounting",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read cost accounting",
				DescriptionKey: "View supplier cost rules, accounting details, anomalies, and profit reports.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Manage cost accounting",
				DescriptionKey: "Create, edit, validate, activate, and retire supplier cost rules.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionReconcile,
				LabelKey:       "Reconcile cost accounting",
				DescriptionKey: "Resolve unknown supplier costs and failed revenue recognition with an audit reason.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
}

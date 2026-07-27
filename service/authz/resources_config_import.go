package authz

const ResourceConfigImport = "config_import"

const ActionPublish = "publish"

var (
	ConfigImportRead    = Permission{Resource: ResourceConfigImport, Action: ActionRead}
	ConfigImportWrite   = Permission{Resource: ResourceConfigImport, Action: ActionWrite}
	ConfigImportPublish = Permission{Resource: ResourceConfigImport, Action: ActionPublish}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceConfigImport,
		LabelKey: "Channel configuration import",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "Read channel imports", DescriptionKey: "View imported configuration batches without secrets.", DefaultRoles: []string{BuiltInRoleAdmin}},
			{Action: ActionWrite, LabelKey: "Manage channel imports", DescriptionKey: "Upload, bind, resolve, stage, and validate imported configuration.", DefaultRoles: []string{BuiltInRoleAdmin}},
			{Action: ActionPublish, LabelKey: "Publish channel imports", DescriptionKey: "Publish reviewed imported configuration atomically.", DefaultRoles: []string{BuiltInRoleAdmin}},
		},
	})
}

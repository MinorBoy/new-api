package router

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigImportActivateRouteRequiresPublishPermission(t *testing.T) {
	assertConfigImportRoutePermission(t, http.MethodPost, "/batches/:id/activate", authz.ConfigImportPublish, controller.ActivateConfigImportBatch)

	for _, resource := range authz.Catalog() {
		if resource.Resource != authz.ResourceConfigImport {
			continue
		}
		for _, action := range resource.Actions {
			if action.Action == authz.ActionPublish {
				assert.Equal(t, "Publish, activate, and backfill ownership for reviewed imported configuration.", action.DescriptionKey)
				return
			}
		}
	}
	require.Fail(t, "config import publish permission was not registered")
}

func TestConfigImportRouteOwnershipRoutesRequirePublishPermission(t *testing.T) {
	assertConfigImportRoutePermission(t, http.MethodGet, "/route-ownership/backfill-preview", authz.ConfigImportPublish, controller.PreviewConfigImportRouteOwnershipBackfill)
	assertConfigImportRoutePermission(t, http.MethodPost, "/route-ownership/backfill", authz.ConfigImportPublish, controller.ApplyConfigImportRouteOwnershipBackfill)
	assertConfigImportRoutePermission(t, http.MethodPost, "/route-ownership/backfill/:operation_id/rollback", authz.ConfigImportPublish, controller.RollbackConfigImportRouteOwnershipBackfill)
}

func assertConfigImportRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range configImportPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}

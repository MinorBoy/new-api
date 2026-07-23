package router

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
)

func TestRoutingPolicyRoutesUseChannelPermissions(t *testing.T) {
	assertRoutingPolicyRoutePermission(t, http.MethodGet, "/", authz.ChannelRead, controller.ListRoutingPolicies)
	assertRoutingPolicyRoutePermission(t, http.MethodGet, "/candidates", authz.ChannelRead, controller.ListRoutingPolicyCandidates)
	assertRoutingPolicyRoutePermission(t, http.MethodGet, "/:id", authz.ChannelRead, controller.GetRoutingPolicy)
	assertRoutingPolicyRoutePermission(t, http.MethodPost, "/", authz.ChannelWrite, controller.CreateRoutingPolicy)
	assertRoutingPolicyRoutePermission(t, http.MethodPut, "/:id", authz.ChannelWrite, controller.UpdateRoutingPolicy)
	assertRoutingPolicyRoutePermission(t, http.MethodPost, "/:id/status", authz.ChannelWrite, controller.UpdateRoutingPolicyStatus)
	assertRoutingPolicyRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelWrite, controller.DeleteRoutingPolicy)
}

func assertRoutingPolicyRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range routingPolicyPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}

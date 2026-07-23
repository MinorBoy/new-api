package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerRoutingPolicyRoutes(apiRouter *gin.RouterGroup) {
	routingPolicyRoute := apiRouter.Group("/routing-policies")
	routingPolicyRoute.Use(middleware.AdminAuth())
	for _, route := range routingPolicyPermissionRoutes {
		routingPolicyRoute.Handle(route.method, route.path,
			middleware.RequirePermission(route.permission),
			route.handler,
		)
	}
}

var routingPolicyPermissionRoutes = []permissionRoute{
	{method: http.MethodGet, path: "/", permission: authz.ChannelRead, handler: controller.ListRoutingPolicies},
	{method: http.MethodGet, path: "/candidates", permission: authz.ChannelRead, handler: controller.ListRoutingPolicyCandidates},
	{method: http.MethodGet, path: "/:id", permission: authz.ChannelRead, handler: controller.GetRoutingPolicy},
	{method: http.MethodPost, path: "/", permission: authz.ChannelWrite, handler: controller.CreateRoutingPolicy},
	{method: http.MethodPut, path: "/:id", permission: authz.ChannelWrite, handler: controller.UpdateRoutingPolicy},
	{method: http.MethodPost, path: "/:id/status", permission: authz.ChannelWrite, handler: controller.UpdateRoutingPolicyStatus},
	{method: http.MethodDelete, path: "/:id", permission: authz.ChannelWrite, handler: controller.DeleteRoutingPolicy},
}

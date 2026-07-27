package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerConfigImportRoutes(apiRouter *gin.RouterGroup) {
	route := apiRouter.Group("/config-import")
	route.Use(middleware.AdminAuth())
	for _, entry := range configImportPermissionRoutes {
		route.Handle(entry.method, entry.path, middleware.RequirePermission(entry.permission), entry.handler)
	}
}

var configImportPermissionRoutes = []permissionRoute{
	{method: http.MethodPost, path: "/batches", permission: authz.ConfigImportWrite, handler: controller.CreateConfigImportBatch},
	{method: http.MethodGet, path: "/batches", permission: authz.ConfigImportRead, handler: controller.ListConfigImportBatches},
	{method: http.MethodGet, path: "/batches/:id", permission: authz.ConfigImportRead, handler: controller.GetConfigImportBatch},
	{method: http.MethodPut, path: "/batches/:id/bindings", permission: authz.ConfigImportWrite, handler: controller.UpdateConfigImportBindings},
	{method: http.MethodPut, path: "/batches/:id/resolutions", permission: authz.ConfigImportWrite, handler: controller.UpdateConfigImportResolutions},
	{method: http.MethodPost, path: "/batches/:id/stage", permission: authz.ConfigImportWrite, handler: controller.StageConfigImportBatch},
	{method: http.MethodPost, path: "/batches/:id/validate", permission: authz.ConfigImportWrite, handler: controller.ValidateConfigImportBatch},
	{method: http.MethodPost, path: "/batches/:id/publish", permission: authz.ConfigImportPublish, handler: controller.PublishConfigImportBatch},
}

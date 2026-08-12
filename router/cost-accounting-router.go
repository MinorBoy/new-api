package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerCostAccountingRoutes(apiRouter *gin.RouterGroup) {
	costRoute := apiRouter.Group("/cost-accounting")
	costRoute.Use(middleware.AdminAuth())
	for _, route := range costAccountingPermissionRoutes {
		costRoute.Handle(route.method, route.path,
			middleware.RequirePermission(route.permission),
			route.handler,
		)
	}
}

var costAccountingPermissionRoutes = []permissionRoute{
	{method: http.MethodGet, path: "/settings", permission: authz.CostAccountingRead, handler: controller.GetCostAccountingSettings},
	{method: http.MethodPut, path: "/settings", permission: authz.CostAccountingWrite, handler: controller.UpdateCostAccountingSettings},
	{method: http.MethodGet, path: "/catalog", permission: authz.CostAccountingRead, handler: controller.ListSupplierCostCatalog},
	{method: http.MethodGet, path: "/catalog/export", permission: authz.CostAccountingRead, handler: controller.ExportSupplierCostCatalog},
	{method: http.MethodGet, path: "/catalog/:rule_id", permission: authz.CostAccountingRead, handler: controller.GetSupplierCostCatalogDetail},
	{method: http.MethodGet, path: "/route-margin-catalog", permission: authz.CostAccountingRead, handler: controller.ListRouteMarginCatalog},
	{method: http.MethodGet, path: "/route-margin-catalog/export", permission: authz.CostAccountingRead, handler: controller.ExportRouteMarginCatalog},
	{method: http.MethodGet, path: "/rules", permission: authz.CostAccountingRead, handler: controller.ListCostRules},
	{method: http.MethodPost, path: "/rules", permission: authz.CostAccountingWrite, handler: controller.CreateCostRule},
	{method: http.MethodPut, path: "/rules/:id", permission: authz.CostAccountingWrite, handler: controller.UpdateCostRule},
	{method: http.MethodPost, path: "/rules/:id/validate", permission: authz.CostAccountingWrite, handler: controller.ValidateCostRule},
	{method: http.MethodPost, path: "/rules/:id/activate", permission: authz.CostAccountingWrite, handler: controller.ActivateCostRule},
	{method: http.MethodPost, path: "/rules/:id/retire", permission: authz.CostAccountingWrite, handler: controller.RetireCostRule},
	{method: http.MethodGet, path: "/rules/:id/history", permission: authz.CostAccountingRead, handler: controller.GetCostRuleHistory},
	{method: http.MethodPost, path: "/preview", permission: authz.CostAccountingRead, handler: controller.PreviewCostAccounting},
	{method: http.MethodGet, path: "/coverage", permission: authz.CostAccountingRead, handler: controller.GetCostCoverage},
	{method: http.MethodGet, path: "/requests/:id", permission: authz.CostAccountingRead, handler: controller.GetCostAccountingRequest},
	{method: http.MethodGet, path: "/anomalies", permission: authz.CostAccountingRead, handler: controller.ListCostAnomalies},
	{method: http.MethodPost, path: "/attempts/:id/reconcile", permission: authz.CostAccountingReconcile, handler: controller.ReconcileCostAttempt},
	{method: http.MethodPost, path: "/requests/:id/reconcile-revenue", permission: authz.CostAccountingReconcile, handler: controller.ReconcileCostRevenue},
	{method: http.MethodGet, path: "/reports/summary", permission: authz.CostAccountingRead, handler: controller.GetCostReportSummary},
	{method: http.MethodGet, path: "/reports/breakdown", permission: authz.CostAccountingRead, handler: controller.GetCostReportBreakdown},
}

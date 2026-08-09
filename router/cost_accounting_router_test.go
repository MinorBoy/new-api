package router

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
)

func TestCostAccountingRoutesUseDedicatedPermissions(t *testing.T) {
	assertCostRoute(t, http.MethodGet, "/settings", authz.CostAccountingRead, controller.GetCostAccountingSettings)
	assertCostRoute(t, http.MethodPut, "/settings", authz.CostAccountingWrite, controller.UpdateCostAccountingSettings)
	assertCostRoute(t, http.MethodGet, "/rules", authz.CostAccountingRead, controller.ListCostRules)
	assertCostRoute(t, http.MethodPost, "/rules", authz.CostAccountingWrite, controller.CreateCostRule)
	assertCostRoute(t, http.MethodPut, "/rules/:id", authz.CostAccountingWrite, controller.UpdateCostRule)
	assertCostRoute(t, http.MethodPost, "/rules/:id/validate", authz.CostAccountingWrite, controller.ValidateCostRule)
	assertCostRoute(t, http.MethodPost, "/rules/:id/activate", authz.CostAccountingWrite, controller.ActivateCostRule)
	assertCostRoute(t, http.MethodPost, "/rules/:id/retire", authz.CostAccountingWrite, controller.RetireCostRule)
	assertCostRoute(t, http.MethodGet, "/rules/:id/history", authz.CostAccountingRead, controller.GetCostRuleHistory)
	assertCostRoute(t, http.MethodPost, "/preview", authz.CostAccountingRead, controller.PreviewCostAccounting)
	assertCostRoute(t, http.MethodGet, "/coverage", authz.CostAccountingRead, controller.GetCostCoverage)
	assertCostRoute(t, http.MethodGet, "/requests/:id", authz.CostAccountingRead, controller.GetCostAccountingRequest)
	assertCostRoute(t, http.MethodGet, "/anomalies", authz.CostAccountingRead, controller.ListCostAnomalies)
	assertCostRoute(t, http.MethodPost, "/attempts/:id/reconcile", authz.CostAccountingReconcile, controller.ReconcileCostAttempt)
	assertCostRoute(t, http.MethodPost, "/requests/:id/reconcile-revenue", authz.CostAccountingReconcile, controller.ReconcileCostRevenue)
	assertCostRoute(t, http.MethodGet, "/reports/summary", authz.CostAccountingRead, controller.GetCostReportSummary)
	assertCostRoute(t, http.MethodGet, "/reports/breakdown", authz.CostAccountingRead, controller.GetCostReportBreakdown)
	assertCostRoute(t, http.MethodGet, "/catalog", authz.CostAccountingRead, controller.ListSupplierCostCatalog)
	assertCostRoute(t, http.MethodGet, "/catalog/export", authz.CostAccountingRead, controller.ExportSupplierCostCatalog)
	assertCostRoute(t, http.MethodGet, "/catalog/:rule_id", authz.CostAccountingRead, controller.GetSupplierCostCatalogDetail)
}

func assertCostRoute(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range costAccountingPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}

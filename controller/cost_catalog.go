package controller

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var costCatalogSortFields = map[string]struct{}{
	"channel_name": {}, "channel_id": {}, "billable_upstream_model": {},
	"cost_variant_key": {}, "status": {}, "version": {},
	"cost_mode": {}, "source": {}, "effective_from": {},
}

func costCatalogFilterFromQuery(c *gin.Context) (service.CostCatalogFilter, error) {
	channelID, err := optionalCostAccountingQueryInt(c, "channel_id")
	if err != nil {
		return service.CostCatalogFilter{}, err
	}

	page := 1
	if _, exists := c.GetQuery("page"); exists {
		page, err = optionalCostAccountingQueryInt(c, "page")
		if err != nil || page <= 0 {
			return service.CostCatalogFilter{}, errors.New("invalid page")
		}
	}
	pageSize := 50
	if _, exists := c.GetQuery("page_size"); exists {
		pageSize, err = optionalCostAccountingQueryInt(c, "page_size")
		if err != nil || (pageSize != 25 && pageSize != 50 && pageSize != 100) {
			return service.CostCatalogFilter{}, errors.New("invalid page_size")
		}
	}

	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		status = string(types.CostRuleActive)
	}
	if status != string(types.CostRuleActive) && status != string(types.CostRuleDraft) &&
		status != string(types.CostRuleRetired) && status != "all" {
		return service.CostCatalogFilter{}, errors.New("invalid status")
	}
	costMode := strings.TrimSpace(c.Query("cost_mode"))
	if costMode != "" && costMode != string(types.CostModeFree) && costMode != string(types.CostModePerRequest) &&
		costMode != string(types.CostModePerDuration) && costMode != string(types.CostModePerToken) {
		return service.CostCatalogFilter{}, errors.New("invalid cost_mode")
	}

	sortBy := strings.TrimSpace(c.Query("sort_by"))
	if sortBy == "" {
		sortBy = "channel_name"
	}
	if _, ok := costCatalogSortFields[sortBy]; !ok {
		return service.CostCatalogFilter{}, errors.New("invalid sort_by")
	}
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sort_order")))
	if sortOrder == "" {
		sortOrder = "asc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return service.CostCatalogFilter{}, errors.New("invalid sort_order")
	}

	billableModel := strings.TrimSpace(c.Query("billable_upstream_model"))
	if len(billableModel) > 191 {
		return service.CostCatalogFilter{}, errors.New("billable_upstream_model is too long")
	}
	source := strings.TrimSpace(c.Query("source"))
	if len(source) > 32 {
		return service.CostCatalogFilter{}, errors.New("source is too long")
	}
	return service.CostCatalogFilter{
		ChannelID: channelID, BillableUpstreamModel: billableModel,
		CostMode: costMode, Status: status,
		Currency: strings.ToUpper(strings.TrimSpace(c.Query("currency"))), Source: source,
		Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder,
	}, nil
}

func ListSupplierCostCatalog(c *gin.Context) {
	filter, err := costCatalogFilterFromQuery(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	page, err := service.ListSupplierCostCatalog(filter)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func GetSupplierCostCatalogDetail(c *gin.Context) {
	ruleID, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil || ruleID <= 0 {
		writeCostAccountingError(c, errors.New("invalid cost catalog rule ID"))
		return
	}
	detail, err := service.GetSupplierCostCatalogDetail(ruleID)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func ExportSupplierCostCatalog(c *gin.Context) {
	scope := service.CostCatalogExportScope(strings.TrimSpace(c.Query("scope")))
	if scope != service.CostCatalogExportFiltered && scope != service.CostCatalogExportAll {
		writeCostAccountingError(c, errors.New("cost catalog export scope must be filtered or all"))
		return
	}
	filter, err := costCatalogFilterFromQuery(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	file, err := os.CreateTemp("", "new-api-cost-catalog-*.csv")
	if err != nil {
		writeCostAccountingError(c, fmt.Errorf("%w: %v", service.ErrCostCatalogUnavailable, err))
		return
	}
	fileName := file.Name()
	defer os.Remove(fileName)

	count, err := service.WriteSupplierCostCatalogCSV(file, filter, scope)
	if err != nil {
		_ = file.Close()
		writeCostAccountingError(c, err)
		return
	}
	if err := file.Close(); err != nil {
		writeCostAccountingError(c, fmt.Errorf("%w: %v", service.ErrCostCatalogUnavailable, err))
		return
	}

	timestamp := time.Now().Format("20060102-150405")
	asciiScope := string(scope)
	chineseScope := "筛选结果"
	if scope == service.CostCatalogExportAll {
		chineseScope = "全部"
	}
	asciiName := fmt.Sprintf("supplier-cost-catalog-%s-%s.csv", asciiScope, timestamp)
	chineseName := fmt.Sprintf("供应商成本目录-%s-%s.csv", chineseScope, timestamp)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(
		"attachment; filename=%q; filename*=UTF-8''%s", asciiName, url.PathEscape(chineseName),
	))
	c.Header("X-Exported-Row-Count", strconv.Itoa(count))
	c.File(fileName)
}

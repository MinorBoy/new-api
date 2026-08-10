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
	"github.com/gin-gonic/gin"
)

var routeMarginCatalogSortFields = map[string]struct{}{
	"target_name": {}, "channel_name": {}, "upstream_model": {},
	"gross_margin_ppm": {}, "estimated_profit_nano_usd": {},
}

func routeMarginCatalogFilterFromQuery(c *gin.Context) (service.RouteMarginCatalogFilter, error) {
	filter := service.RouteMarginCatalogFilter{
		MinimumMarginPPM: service.DefaultRouteMarginMinimumPPM,
		DurationSeconds:  service.DefaultRouteMarginDurationSeconds,
		GroupRatio:       service.DefaultRouteMarginGroupRatio,
		Scenario:         strings.ToLower(strings.TrimSpace(c.Query("scenario"))),
		Status:           strings.ToLower(strings.TrimSpace(c.Query("status"))),
		Resolution:       strings.ToLower(strings.TrimSpace(c.Query("resolution"))),
		Page:             1, PageSize: 50, SortBy: "gross_margin_ppm", SortOrder: "desc",
	}
	if value, exists := c.GetQuery("min_margin_ppm"); exists {
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return service.RouteMarginCatalogFilter{}, errors.New("invalid min_margin_ppm")
		}
		filter.MinimumMarginPPM = parsed
	}
	if value, exists := c.GetQuery("duration_seconds"); exists {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed <= 0 {
			return service.RouteMarginCatalogFilter{}, errors.New("invalid duration_seconds")
		}
		filter.DurationSeconds = parsed
	}
	if value, exists := c.GetQuery("group_ratio"); exists {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || parsed <= 0 {
			return service.RouteMarginCatalogFilter{}, errors.New("invalid group_ratio")
		}
		filter.GroupRatio = parsed
	}
	if channelID, exists := c.GetQuery("channel_id"); exists {
		parsed, err := strconv.Atoi(strings.TrimSpace(channelID))
		if err != nil || parsed <= 0 {
			return service.RouteMarginCatalogFilter{}, errors.New("invalid channel_id")
		}
		filter.ChannelID = parsed
	}
	filter.CanonicalModel = strings.TrimSpace(c.Query("model"))
	filter.UpstreamModel = strings.TrimSpace(c.Query("upstream_model"))
	filter.TargetName = strings.TrimSpace(c.Query("route_target"))
	for key, value := range map[string]string{
		"model": filter.CanonicalModel, "upstream_model": filter.UpstreamModel,
		"route_target": filter.TargetName, "resolution": filter.Resolution,
	} {
		if len(value) > 191 {
			return service.RouteMarginCatalogFilter{}, errors.New(key + " is too long")
		}
	}
	if value, exists := c.GetQuery("page"); exists {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed <= 0 {
			return service.RouteMarginCatalogFilter{}, errors.New("invalid page")
		}
		filter.Page = parsed
	}
	if value, exists := c.GetQuery("page_size"); exists {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || (parsed != 25 && parsed != 50 && parsed != 100) {
			return service.RouteMarginCatalogFilter{}, errors.New("invalid page_size")
		}
		filter.PageSize = parsed
	}
	if filter.Scenario == "" {
		filter.Scenario = service.RouteMarginScenarioAll
	}
	if filter.Status == "" {
		filter.Status = "all"
	}
	if value, exists := c.GetQuery("sort_by"); exists {
		filter.SortBy = strings.TrimSpace(value)
	}
	if _, ok := routeMarginCatalogSortFields[filter.SortBy]; !ok {
		return service.RouteMarginCatalogFilter{}, errors.New("invalid sort_by")
	}
	if value, exists := c.GetQuery("sort_order"); exists {
		filter.SortOrder = strings.ToLower(strings.TrimSpace(value))
	}
	if filter.SortOrder != "asc" && filter.SortOrder != "desc" {
		return service.RouteMarginCatalogFilter{}, errors.New("invalid sort_order")
	}
	return service.NormalizeRouteMarginCatalogFilter(filter)
}

func ListRouteMarginCatalog(c *gin.Context) {
	filter, err := routeMarginCatalogFilterFromQuery(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	page, err := service.ListRouteMarginCatalog(c.Request.Context(), filter)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func ExportRouteMarginCatalog(c *gin.Context) {
	filter, err := routeMarginCatalogFilterFromQuery(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	file, err := os.CreateTemp("", "new-api-route-margin-catalog-*.csv")
	if err != nil {
		writeCostAccountingError(c, fmt.Errorf("%w: create temporary file: %v", service.ErrRouteMarginCatalogUnavailable, err))
		return
	}
	fileName := file.Name()
	defer os.Remove(fileName)
	count, err := service.WriteRouteMarginCatalogCSV(file, filter, c.Request.Context())
	if closeErr := file.Close(); err == nil {
		if closeErr != nil {
			err = fmt.Errorf("%w: close temporary file: %v", service.ErrRouteMarginCatalogUnavailable, closeErr)
		}
	}
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	timestamp := time.Now().Format("20060102-150405")
	chineseName := fmt.Sprintf("路由毛利目录-%s.csv", timestamp)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", "route-margin-catalog-"+timestamp+".csv", url.PathEscape(chineseName)))
	c.Header("X-Exported-Row-Count", strconv.Itoa(count))
	c.File(fileName)
}

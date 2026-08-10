package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/shopspring/decimal"
)

const RouteMarginCatalogExportMaxRows = 100_000

var ErrRouteMarginCatalogExportTooLarge = errors.New("route margin catalog export exceeds 100000 rows")

var routeMarginCatalogCSVHeader = []string{
	"路由目标", "渠道名称", "渠道 ID", "规范模型", "上游模型", "成本变体",
	"分辨率", "场景", "输出时长（秒）", "分组倍率", "成本模式",
	"预计收入（USD）", "预计成本（USD）", "预计利润（USD）", "毛利率",
	"达标状态", "不通过原因", "成本规则来源", "用户价格来源", "规则 ID", "规则版本",
}

func WriteRouteMarginCatalogCSV(w io.Writer, filter RouteMarginCatalogFilter, ctx context.Context) (int, error) {
	if w == nil {
		return 0, errors.New("route margin catalog export writer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	filter, err := NormalizeRouteMarginCatalogFilter(filter)
	if err != nil {
		return 0, err
	}
	filter.Page = 1
	filter.PageSize = RouteMarginCatalogExportMaxRows + 1
	page, err := listRouteMarginCatalog(ctx, filter)
	if err != nil {
		return 0, err
	}
	if page.Total > RouteMarginCatalogExportMaxRows {
		return 0, ErrRouteMarginCatalogExportTooLarge
	}

	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return 0, fmt.Errorf("%w: write BOM: %v", ErrRouteMarginCatalogUnavailable, err)
	}
	writer := csv.NewWriter(w)
	if err := writer.Write(routeMarginCatalogCSVHeader); err != nil {
		return 0, fmt.Errorf("%w: write CSV header: %v", ErrRouteMarginCatalogUnavailable, err)
	}

	for _, item := range page.Items {
		if err := writer.Write(routeMarginCatalogCSVRecord(item)); err != nil {
			return 0, fmt.Errorf("%w: write CSV row: %v", ErrRouteMarginCatalogUnavailable, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return 0, fmt.Errorf("%w: flush CSV: %v", ErrRouteMarginCatalogUnavailable, err)
	}
	return len(page.Items), nil
}

func routeMarginCatalogCSVRecord(item dto.RouteMarginCatalogItem) []string {
	record := []string{
		item.TargetName, item.ChannelName, strconv.Itoa(item.ChannelID), item.CanonicalModel,
		item.UpstreamModel, item.CostVariantKey, item.Resolution, item.Scenario,
		strconv.Itoa(item.DurationSeconds), item.GroupRatio, string(item.CostMode),
		formatRouteMarginUSD(item.EstimatedRevenueNanoUSD), formatRouteMarginUSD(item.EstimatedCostNanoUSD),
		formatRouteMarginUSD(item.EstimatedProfitNanoUSD), formatRouteMarginPercent(item.GrossMarginPPM),
		strconv.FormatBool(item.Eligible), item.FailureReason, item.CostSource, item.RevenueSource,
		strconv.FormatInt(item.RuleID, 10), strconv.Itoa(item.RuleVersion),
	}
	for index := range record {
		record[index] = safeCatalogCSVCell(record[index])
	}
	return record
}

func formatRouteMarginUSD(value *int64) string {
	if value == nil {
		return ""
	}
	return decimal.NewFromInt(*value).Div(decimal.NewFromInt(1_000_000_000)).String()
}

func formatRouteMarginPercent(value *int64) string {
	if value == nil {
		return ""
	}
	return strings.TrimSuffix(decimal.NewFromInt(*value).Div(decimal.NewFromInt(10_000)).String(), ".0") + "%"
}

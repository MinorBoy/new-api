package service

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const CostCatalogExportMaxRows = 100_000

var ErrCostCatalogExportTooLarge = errors.New("cost catalog export exceeds 100000 rows")

type CostCatalogExportScope string

const (
	CostCatalogExportFiltered CostCatalogExportScope = "filtered"
	CostCatalogExportAll      CostCatalogExportScope = "all"
)

var costCatalogCSVHeader = []string{
	"渠道名称", "渠道 ID", "渠道类型",
	"规则 ID", "计费上游模型", "成本变体", "规则状态", "版本", "Schema 版本",
	"成本模式", "币种",
	"每次原币单价", "每秒原币单价", "每 1M 总 Token 原币单价", "每 1M Completion Token 原币单价", "每 1M 输入 Token 原币单价", "每 1M 输出 Token 原币单价",
	"每次标准化 USD 单价", "每秒标准化 USD 单价", "每 1M 总 Token 标准化 USD 单价", "每 1M Completion Token 标准化 USD 单价", "每 1M 输入 Token 标准化 USD 单价", "每 1M 输出 Token 标准化 USD 单价",
	"15 秒等效 USD/秒（仅比较）",
	"计费倍率", "采购折扣率", "充值换算率", "费率", "兑 USD 汇率",
	"收费事件", "计量来源", "Token 子模式", "免费原因",
	"来源", "备注", "生效时间", "失效时间", "创建时间", "更新时间",
	"价格状态", "问题代码",
}

func WriteSupplierCostCatalogCSV(w io.Writer, filter CostCatalogFilter, scope CostCatalogExportScope) (int, error) {
	if w == nil {
		return 0, errors.New("cost catalog export writer is required")
	}
	normalized, _, _, err := normalizeCostCatalogFilter(filter)
	if err != nil {
		return 0, err
	}
	switch scope {
	case CostCatalogExportFiltered:
	case CostCatalogExportAll:
		normalized = CostCatalogFilter{
			Status: "all", Page: 1, PageSize: 50,
			SortBy: normalized.SortBy, SortOrder: normalized.SortOrder,
		}
	default:
		return 0, errors.New("invalid cost catalog export scope")
	}

	count := 0
	err = walkSupplierCostCatalogProjections(normalized, func(projection costCatalogProjection) error {
		if normalized.Currency != "" && projection.Item.Currency != normalized.Currency {
			return nil
		}
		count++
		if count > CostCatalogExportMaxRows {
			return ErrCostCatalogExportTooLarge
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrCostCatalogExportTooLarge) {
			return 0, ErrCostCatalogExportTooLarge
		}
		return 0, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}
	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}
	csvWriter := csv.NewWriter(w)
	if err := csvWriter.Write(costCatalogCSVHeader); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}

	written := 0
	err = walkSupplierCostCatalogProjections(normalized, func(projection costCatalogProjection) error {
		if normalized.Currency != "" && projection.Item.Currency != normalized.Currency {
			return nil
		}
		if err := csvWriter.Write(costCatalogCSVRecord(projection)); err != nil {
			return err
		}
		written++
		return nil
	})
	csvWriter.Flush()
	if err == nil {
		err = csvWriter.Error()
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}
	return written, nil
}

func costCatalogCSVRecord(projection costCatalogProjection) []string {
	item := projection.Item
	nativePrices := make(map[string]string, len(item.Prices))
	normalizedPrices := make(map[string]string, len(item.Prices))
	if item.PriceStatus == "available" {
		for _, price := range item.Prices {
			nativePrices[price.Key] = price.NativeAmount
			normalizedPrices[price.Key] = price.NormalizedUSDAmount
		}
	}
	comparison := ""
	if item.PriceStatus == "available" && item.Comparison15SEquivalentUSDPerSecond != nil {
		comparison = *item.Comparison15SEquivalentUSDPerSecond
	}

	billingMultiplier := ""
	purchaseDiscountRatio := ""
	rechargeExchangeRatio := ""
	feeRate := ""
	currencyToUSDRate := ""
	zeroCostReason := ""
	if projection.Config != nil {
		billingMultiplier = projection.Config.BillingMultiplier
		purchaseDiscountRatio = projection.Config.PurchaseDiscountRatio
		rechargeExchangeRatio = projection.Config.RechargeExchangeRatio
		feeRate = projection.Config.FeeRate
		currencyToUSDRate = projection.Config.CurrencyToUSDRate
		zeroCostReason = projection.Config.ZeroCostReason
	}
	record := []string{
		item.ChannelName, strconv.Itoa(item.ChannelID), strconv.Itoa(item.ChannelType),
		strconv.FormatInt(item.RuleID, 10), item.BillableUpstreamModel, item.CostVariantKey,
		item.Status, strconv.Itoa(item.Version), strconv.Itoa(item.SchemaVersion),
		string(item.CostMode), item.Currency,
		nativePrices["unit_price"], nativePrices["price_per_second"], nativePrices["total_per_million"],
		nativePrices["completion_per_million"], nativePrices["input_per_million"], nativePrices["output_per_million"],
		normalizedPrices["unit_price"], normalizedPrices["price_per_second"], normalizedPrices["total_per_million"],
		normalizedPrices["completion_per_million"], normalizedPrices["input_per_million"], normalizedPrices["output_per_million"],
		comparison,
		billingMultiplier, purchaseDiscountRatio, rechargeExchangeRatio, feeRate, currencyToUSDRate,
		string(item.ChargeEvent), string(item.MeterSource), string(item.TokenMode), zeroCostReason,
		item.Source, item.Note, costCatalogCSVTimestamp(item.EffectiveFrom), costCatalogCSVTimestamp(item.EffectiveTo),
		strconv.FormatInt(item.CreatedAt, 10), strconv.FormatInt(item.UpdatedAt, 10),
		item.PriceStatus, strings.Join(item.Issues, ";"),
	}
	for index := range record {
		record[index] = safeCatalogCSVCell(record[index])
	}
	return record
}

func costCatalogCSVTimestamp(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func safeCatalogCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

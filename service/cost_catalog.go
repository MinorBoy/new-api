package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrCostCatalogUnavailable = errors.New("cost catalog unavailable")

type CostCatalogFilter struct {
	ChannelID             int
	BillableUpstreamModel string
	CostMode              string
	Status                string
	Currency              string
	Source                string
	Page                  int
	PageSize              int
	SortBy                string
	SortOrder             string
}

type costCatalogProjection struct {
	Item        dto.CostCatalogItem
	Config      *types.CostRuleConfigV1
	CreatedBy   int
	ActivatedBy int
}

func ListSupplierCostCatalog(filter CostCatalogFilter) (dto.CostCatalogPage, error) {
	normalized, offset, overflow, err := normalizeCostCatalogFilter(filter)
	if err != nil {
		return dto.CostCatalogPage{}, err
	}
	page := dto.CostCatalogPage{
		Items:    make([]dto.CostCatalogItem, 0),
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
		Facets: dto.CostCatalogFacets{
			Channels:   make([]dto.CostCatalogChannelFacet, 0),
			Currencies: make([]string, 0),
			Sources:    make([]string, 0),
		},
	}

	if normalized.Currency == "" {
		query := catalogModelQuery(normalized)
		query.Limit = normalized.PageSize
		if !overflow {
			query.Offset = offset
		}
		rows, total, queryErr := model.ListCostCatalogRows(query)
		if queryErr != nil {
			return dto.CostCatalogPage{}, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, queryErr)
		}
		page.Total = total
		if !overflow {
			for _, row := range rows {
				page.Items = append(page.Items, projectCostCatalogRow(row).Item)
			}
		}
	} else {
		var matched int64
		walkErr := walkSupplierCostCatalogProjections(normalized, func(projection costCatalogProjection) error {
			if projection.Item.Currency != normalized.Currency {
				return nil
			}
			if !overflow && matched >= int64(offset) && matched < int64(offset+normalized.PageSize) {
				page.Items = append(page.Items, projection.Item)
			}
			matched++
			return nil
		})
		if walkErr != nil {
			return dto.CostCatalogPage{}, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, walkErr)
		}
		page.Total = matched
	}

	page.Summary, err = summarizeSupplierCostCatalog(normalized)
	if err != nil {
		return dto.CostCatalogPage{}, err
	}
	page.Facets, err = supplierCostCatalogFacets()
	if err != nil {
		return dto.CostCatalogPage{}, err
	}
	return page, nil
}

func GetSupplierCostCatalogDetail(ruleID int64) (*dto.CostCatalogDetail, error) {
	row, err := model.GetCostCatalogRow(ruleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}
	projection := projectCostCatalogRow(*row)
	historyRows, err := model.ListCostCatalogHistoryRows(row.ChannelID, row.BillableUpstreamModel, row.CostVariantKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}
	detail := &dto.CostCatalogDetail{
		Rule: dto.CostCatalogHistoryEntry{
			CostCatalogItem: projection.Item,
			CreatedBy:       projection.CreatedBy,
			ActivatedBy:     projection.ActivatedBy,
		},
		Config:  projection.Config,
		History: make([]dto.CostCatalogHistoryEntry, 0, len(historyRows)),
	}
	for _, historyRow := range historyRows {
		historyProjection := projectCostCatalogRow(historyRow)
		detail.History = append(detail.History, dto.CostCatalogHistoryEntry{
			CostCatalogItem: historyProjection.Item,
			CreatedBy:       historyProjection.CreatedBy,
			ActivatedBy:     historyProjection.ActivatedBy,
		})
	}
	return detail, nil
}

func walkSupplierCostCatalogProjections(filter CostCatalogFilter, visit func(costCatalogProjection) error) error {
	query := catalogModelQuery(filter)
	query.Offset = 0
	query.Limit = 0
	return model.WalkCostCatalogRows(query, 500, func(rows []model.CostCatalogRow) error {
		for _, row := range rows {
			if err := visit(projectCostCatalogRow(row)); err != nil {
				return err
			}
		}
		return nil
	})
}

func normalizeCostCatalogFilter(filter CostCatalogFilter) (CostCatalogFilter, int, bool, error) {
	filter.BillableUpstreamModel = strings.TrimSpace(filter.BillableUpstreamModel)
	filter.CostMode = strings.TrimSpace(filter.CostMode)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Currency = strings.ToUpper(strings.TrimSpace(filter.Currency))
	filter.Source = strings.TrimSpace(filter.Source)
	filter.SortBy = strings.TrimSpace(filter.SortBy)
	filter.SortOrder = strings.ToLower(strings.TrimSpace(filter.SortOrder))
	if filter.Status == "" {
		filter.Status = string(types.CostRuleActive)
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 50
	}
	if filter.Page < 1 {
		return CostCatalogFilter{}, 0, false, errors.New("cost catalog page must be positive")
	}
	if filter.PageSize != 25 && filter.PageSize != 50 && filter.PageSize != 100 {
		return CostCatalogFilter{}, 0, false, errors.New("cost catalog page size must be 25, 50, or 100")
	}
	pageIndex := filter.Page - 1
	if pageIndex > math.MaxInt/filter.PageSize {
		return filter, 0, true, nil
	}
	return filter, pageIndex * filter.PageSize, false, nil
}

func catalogModelQuery(filter CostCatalogFilter) model.CostCatalogQuery {
	return model.CostCatalogQuery{
		ChannelID:             filter.ChannelID,
		BillableUpstreamModel: filter.BillableUpstreamModel,
		CostMode:              filter.CostMode,
		Status:                filter.Status,
		Source:                filter.Source,
		SortBy:                filter.SortBy,
		SortOrder:             filter.SortOrder,
	}
}

func projectCostCatalogRow(row model.CostCatalogRow) costCatalogProjection {
	item := dto.CostCatalogItem{
		RuleID: row.ID, ChannelID: row.ChannelID, ChannelName: row.ChannelName,
		ChannelType: row.ChannelType, ChannelMissing: row.ChannelMissing,
		BillableUpstreamModel: row.BillableUpstreamModel, CostVariantKey: row.CostVariantKey,
		Version: row.Version, Status: row.Status, CostMode: types.CostMode(row.CostMode),
		SchemaVersion: row.SchemaVersion, Prices: make([]dto.CostCatalogPrice, 0),
		Source: row.Source, Note: row.Note, EffectiveFrom: row.EffectiveFrom,
		EffectiveTo: row.EffectiveTo, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		PriceStatus: "available", Issues: make([]string, 0),
	}
	projection := costCatalogProjection{Item: item, CreatedBy: row.CreatedBy, ActivatedBy: row.ActivatedBy}
	if row.ChannelMissing {
		addCostCatalogIssue(&projection.Item, "channel_missing")
	}
	if row.SchemaVersion != 1 {
		addCostCatalogIssue(&projection.Item, "invalid_config")
		projection.Item.PriceStatus = "unavailable"
		return projection
	}

	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(row.ConfigJSON, &config); err != nil {
		addCostCatalogIssue(&projection.Item, "invalid_config")
		projection.Item.PriceStatus = "unavailable"
		return projection
	}
	projection.Config = &config
	projection.Item.Currency = strings.ToUpper(strings.TrimSpace(config.Currency))
	projection.Item.ChargeEvent = config.ChargeEvent
	projection.Item.MeterSource = config.MeterSource
	projection.Item.TokenMode = config.TokenMode

	mode := types.CostMode(row.CostMode)
	if err := validateRulePriceShape(mode, config); err != nil {
		addCostCatalogIssue(&projection.Item, "invalid_config")
		projection.Item.PriceStatus = "unavailable"
		return projection
	}
	if mode != types.CostModeFree && projection.Item.Currency == "" {
		addCostCatalogIssue(&projection.Item, "invalid_config")
		projection.Item.PriceStatus = "unavailable"
		return projection
	}

	valid := true
	switch mode {
	case types.CostModeFree:
		valid = strings.TrimSpace(config.ZeroCostReason) != ""
	case types.CostModePerRequest:
		valid = appendCostCatalogPrice(&projection.Item, "unit_price", "per_request", config.UnitPrice, config.NormalizedUSDPrices.UnitPrice)
		if valid {
			normalized, _ := decimal.NewFromString(projection.Item.Prices[0].NormalizedUSDAmount)
			comparison := normalized.Div(decimal.NewFromInt(15)).String()
			projection.Item.Comparison15SEquivalentUSDPerSecond = &comparison
		}
	case types.CostModePerImage:
		valid = appendCostCatalogPrice(&projection.Item, "unit_price", "per_image", config.UnitPrice, config.NormalizedUSDPrices.UnitPrice)
	case types.CostModePerDuration:
		valid = appendCostCatalogPrice(&projection.Item, "price_per_second", "per_second", config.PricePerSecond, config.NormalizedUSDPrices.PricePerSecond)
	case types.CostModePerToken:
		switch config.TokenMode {
		case types.CostTokenModeTotal:
			valid = appendCostCatalogPrice(&projection.Item, "total_per_million", "per_million_tokens", config.TotalPerMillion, config.NormalizedUSDPrices.TotalPerMillion)
		case types.CostTokenModeCompletion:
			valid = appendCostCatalogPrice(&projection.Item, "completion_per_million", "per_million_completion_tokens", config.CompletionPerMillion, config.NormalizedUSDPrices.CompletionPerMillion)
		case types.CostTokenModeInputOutput:
			inputValid := appendCostCatalogPrice(&projection.Item, "input_per_million", "per_million_input_tokens", config.InputPerMillion, config.NormalizedUSDPrices.InputPerMillion)
			outputValid := appendCostCatalogPrice(&projection.Item, "output_per_million", "per_million_output_tokens", config.OutputPerMillion, config.NormalizedUSDPrices.OutputPerMillion)
			valid = inputValid && outputValid
		default:
			valid = false
		}
	default:
		valid = false
	}
	if !valid {
		if len(projection.Item.Issues) == 0 || (len(projection.Item.Issues) == 1 && projection.Item.Issues[0] == "channel_missing") {
			addCostCatalogIssue(&projection.Item, "invalid_config")
		}
		projection.Item.Prices = make([]dto.CostCatalogPrice, 0)
		projection.Item.Comparison15SEquivalentUSDPerSecond = nil
		projection.Item.PriceStatus = "unavailable"
	}
	return projection
}

func appendCostCatalogPrice(item *dto.CostCatalogItem, key, unit string, nativeValue, normalizedValue *string) bool {
	if nativeValue == nil {
		addCostCatalogIssue(item, "invalid_config")
		return false
	}
	if normalizedValue == nil {
		addCostCatalogIssue(item, "missing_normalized_price")
		return false
	}
	nativeAmount, err := decimal.NewFromString(strings.TrimSpace(*nativeValue))
	if err != nil || nativeAmount.IsNegative() {
		addCostCatalogIssue(item, "invalid_config")
		return false
	}
	normalizedAmount, err := decimal.NewFromString(strings.TrimSpace(*normalizedValue))
	if err != nil || normalizedAmount.IsNegative() {
		addCostCatalogIssue(item, "invalid_config")
		return false
	}
	item.Prices = append(item.Prices, dto.CostCatalogPrice{
		Key: key, Unit: unit, NativeAmount: nativeAmount.String(), NormalizedUSDAmount: normalizedAmount.String(),
	})
	return true
}

func addCostCatalogIssue(item *dto.CostCatalogItem, issue string) {
	for _, existing := range item.Issues {
		if existing == issue {
			return
		}
	}
	item.Issues = append(item.Issues, issue)
}

func summarizeSupplierCostCatalog(filter CostCatalogFilter) (dto.CostCatalogSummary, error) {
	filter.Status = "all"
	channels := make(map[int]struct{})
	var summary dto.CostCatalogSummary
	err := walkSupplierCostCatalogProjections(filter, func(projection costCatalogProjection) error {
		if filter.Currency != "" && projection.Item.Currency != filter.Currency {
			return nil
		}
		channels[projection.Item.ChannelID] = struct{}{}
		switch projection.Item.Status {
		case string(types.CostRuleActive):
			summary.ActiveRuleCount++
		case string(types.CostRuleDraft):
			summary.DraftRuleCount++
		case string(types.CostRuleRetired):
			summary.RetiredRuleCount++
		}
		return nil
	})
	if err != nil {
		return dto.CostCatalogSummary{}, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}
	summary.ChannelCount = int64(len(channels))
	return summary, nil
}

func supplierCostCatalogFacets() (dto.CostCatalogFacets, error) {
	channelSet := make(map[int]dto.CostCatalogChannelFacet)
	currencySet := make(map[string]struct{})
	sourceSet := make(map[string]struct{})
	err := walkSupplierCostCatalogProjections(CostCatalogFilter{Status: "all"}, func(projection costCatalogProjection) error {
		item := projection.Item
		channelSet[item.ChannelID] = dto.CostCatalogChannelFacet{
			ID: item.ChannelID, Name: item.ChannelName, Type: item.ChannelType, Missing: item.ChannelMissing,
		}
		if item.Currency != "" {
			currencySet[item.Currency] = struct{}{}
		}
		if item.Source != "" {
			sourceSet[item.Source] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return dto.CostCatalogFacets{}, fmt.Errorf("%w: %v", ErrCostCatalogUnavailable, err)
	}

	facets := dto.CostCatalogFacets{
		Channels:   make([]dto.CostCatalogChannelFacet, 0, len(channelSet)),
		Currencies: make([]string, 0, len(currencySet)),
		Sources:    make([]string, 0, len(sourceSet)),
	}
	for _, channel := range channelSet {
		facets.Channels = append(facets.Channels, channel)
	}
	for currency := range currencySet {
		facets.Currencies = append(facets.Currencies, currency)
	}
	for source := range sourceSet {
		facets.Sources = append(facets.Sources, source)
	}
	sort.Slice(facets.Channels, func(i, j int) bool {
		if facets.Channels[i].Missing != facets.Channels[j].Missing {
			return !facets.Channels[i].Missing
		}
		left := strings.ToLower(facets.Channels[i].Name)
		right := strings.ToLower(facets.Channels[j].Name)
		if left != right {
			return left < right
		}
		return facets.Channels[i].ID < facets.Channels[j].ID
	})
	sort.Strings(facets.Currencies)
	sort.Strings(facets.Sources)
	return facets, nil
}

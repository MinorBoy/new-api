package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
)

const (
	DefaultRouteMarginMinimumPPM      int64 = 300000
	DefaultRouteMarginDurationSeconds       = 4
	DefaultRouteMarginGroupRatio            = 1.0
	maxRouteMarginGroupRatio                = 100.0
)

const (
	RouteMarginScenarioAll       = "all"
	RouteMarginScenarioNoVideo   = "no_video"
	RouteMarginScenarioWithVideo = "with_video"
)

var ErrRouteMarginCatalogUnavailable = errors.New("route margin catalog unavailable")

type RouteMarginCatalogFilter struct {
	ChannelID        int
	CanonicalModel   string
	UpstreamModel    string
	TargetName       string
	Resolution       string
	MinimumMarginPPM int64
	DurationSeconds  int
	GroupRatio       float64
	Scenario         string
	Status           string
	Page             int
	PageSize         int
	SortBy           string
	SortOrder        string
}

func ListRouteMarginCatalog(ctx context.Context, filter RouteMarginCatalogFilter) (dto.RouteMarginCatalogPage, error) {
	filter, err := NormalizeRouteMarginCatalogFilter(filter)
	if err != nil {
		return dto.RouteMarginCatalogPage{}, err
	}
	return listRouteMarginCatalog(ctx, filter)
}

func listRouteMarginCatalog(ctx context.Context, filter RouteMarginCatalogFilter) (dto.RouteMarginCatalogPage, error) {
	rows, err := model.ListActiveImportedRouteMarginTargets(model.RouteMarginTargetQuery{
		ChannelID: filter.ChannelID, CanonicalModel: filter.CanonicalModel,
		UpstreamModel: filter.UpstreamModel, TargetName: filter.TargetName,
	})
	if err != nil {
		return dto.RouteMarginCatalogPage{}, fmt.Errorf("%w: list route targets: %v", ErrRouteMarginCatalogUnavailable, err)
	}

	candidates := make([]CostRuleCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, CostRuleCandidate{
			ChannelID: row.ChannelID, BillableUpstreamModel: row.UpstreamModel,
			CostVariantKey: row.CostVariantKey,
		})
	}
	rules, err := ActiveCostRules(candidates, true)
	if err != nil {
		return dto.RouteMarginCatalogPage{}, fmt.Errorf("%w: list active cost rules: %v", ErrRouteMarginCatalogUnavailable, err)
	}

	allItems := make([]dto.RouteMarginCatalogItem, 0, len(rows)*2)
	for _, row := range rows {
		for _, resolution := range routeMarginResolutions(row, filter.Resolution) {
			for _, scenario := range routeMarginScenarios(filter.Scenario) {
				allItems = append(allItems, evaluateRouteMarginScenario(ctx, row, rules, resolution, scenario, filter))
			}
		}
	}

	items := make([]dto.RouteMarginCatalogItem, 0, len(allItems))
	for _, item := range allItems {
		if filter.Status == "eligible" && !item.Eligible {
			continue
		}
		if filter.Status == "ineligible" && item.Eligible {
			continue
		}
		items = append(items, item)
	}
	page := dto.RouteMarginCatalogPage{
		Items: make([]dto.RouteMarginCatalogItem, 0), Page: filter.Page, PageSize: filter.PageSize,
		Summary: summarizeRouteMarginItems(allItems), Facets: routeMarginFacets(allItems),
	}
	sortRouteMarginItems(items, filter.SortBy, filter.SortOrder)
	page.Total = len(items)
	if filter.Page <= math.MaxInt/filter.PageSize {
		offset := (filter.Page - 1) * filter.PageSize
		if offset < len(items) {
			end := offset + filter.PageSize
			if end > len(items) {
				end = len(items)
			}
			page.Items = items[offset:end]
		}
	}
	return page, nil
}

func NormalizeRouteMarginCatalogFilter(filter RouteMarginCatalogFilter) (RouteMarginCatalogFilter, error) {
	filter.CanonicalModel = strings.TrimSpace(filter.CanonicalModel)
	filter.UpstreamModel = strings.TrimSpace(filter.UpstreamModel)
	filter.TargetName = strings.TrimSpace(filter.TargetName)
	filter.Resolution = strings.ToLower(strings.TrimSpace(filter.Resolution))
	filter.Scenario = strings.ToLower(strings.TrimSpace(filter.Scenario))
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.SortBy = strings.TrimSpace(filter.SortBy)
	filter.SortOrder = strings.ToLower(strings.TrimSpace(filter.SortOrder))
	if filter.MinimumMarginPPM < -1_000_000 || filter.MinimumMarginPPM > 1_000_000 {
		return RouteMarginCatalogFilter{}, errors.New("minimum margin must be between -100% and 100%")
	}
	if filter.DurationSeconds < 1 || filter.DurationSeconds > relaycommon.MaxTaskDurationSeconds {
		return RouteMarginCatalogFilter{}, errors.New("duration is out of range")
	}
	if math.IsNaN(filter.GroupRatio) || math.IsInf(filter.GroupRatio, 0) || filter.GroupRatio <= 0 || filter.GroupRatio > maxRouteMarginGroupRatio {
		return RouteMarginCatalogFilter{}, errors.New("group ratio is out of range")
	}
	if filter.Scenario == "" {
		filter.Scenario = RouteMarginScenarioAll
	}
	if filter.Scenario != RouteMarginScenarioAll && filter.Scenario != RouteMarginScenarioNoVideo && filter.Scenario != RouteMarginScenarioWithVideo {
		return RouteMarginCatalogFilter{}, errors.New("invalid scenario")
	}
	if filter.Status == "" {
		filter.Status = "all"
	}
	if filter.Status != "all" && filter.Status != "eligible" && filter.Status != "ineligible" {
		return RouteMarginCatalogFilter{}, errors.New("invalid status")
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 50
	}
	if filter.Page < 1 || (filter.PageSize != 25 && filter.PageSize != 50 && filter.PageSize != 100) {
		return RouteMarginCatalogFilter{}, errors.New("invalid pagination")
	}
	if filter.SortBy == "" {
		filter.SortBy = "gross_margin_ppm"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "desc"
	}
	if filter.SortOrder != "asc" && filter.SortOrder != "desc" {
		return RouteMarginCatalogFilter{}, errors.New("invalid sort order")
	}
	switch filter.SortBy {
	case "target_name", "channel_name", "upstream_model", "gross_margin_ppm", "estimated_profit_nano_usd":
	default:
		return RouteMarginCatalogFilter{}, errors.New("invalid sort field")
	}
	return filter, nil
}

func routeMarginResolutions(row model.RouteMarginTargetRow, requested string) []string {
	constraints := modelrouting.Constraints{}
	if err := common.UnmarshalJsonStr(row.Constraints, &constraints); err != nil {
		if requested != "" {
			return nil
		}
		return []string{strings.ToLower(strings.TrimSpace(row.DefaultResolution))}
	}
	if requested != "" {
		for _, resolution := range constraints.OutputResolutions {
			if strings.EqualFold(strings.TrimSpace(resolution), requested) {
				return []string{requested}
			}
		}
		return nil
	}
	resolutions := make([]string, 0, len(constraints.OutputResolutions))
	for _, resolution := range constraints.OutputResolutions {
		resolution = strings.ToLower(strings.TrimSpace(resolution))
		if resolution != "" {
			resolutions = append(resolutions, resolution)
		}
	}
	if len(resolutions) == 0 && row.DefaultResolution != "" {
		resolutions = append(resolutions, strings.ToLower(strings.TrimSpace(row.DefaultResolution)))
	}
	return resolutions
}

func routeMarginScenarios(scenario string) []string {
	if scenario == RouteMarginScenarioNoVideo || scenario == RouteMarginScenarioWithVideo {
		return []string{scenario}
	}
	return []string{RouteMarginScenarioNoVideo, RouteMarginScenarioWithVideo}
}

func evaluateRouteMarginScenario(ctx context.Context, row model.RouteMarginTargetRow, rules map[CostRuleCandidate]*model.ChannelModelCostRule, resolution, scenario string, filter RouteMarginCatalogFilter) dto.RouteMarginCatalogItem {
	item := dto.RouteMarginCatalogItem{
		TargetID: row.TargetID, TargetName: row.TargetName, PolicyID: row.PolicyID,
		GroupName: row.GroupName, CanonicalModel: row.CanonicalModel,
		ChannelID: row.ChannelID, ChannelName: row.ChannelName, ChannelType: row.ChannelType,
		UpstreamModel: row.UpstreamModel, CostVariantKey: row.CostVariantKey,
		Resolution: resolution, DurationSeconds: filter.DurationSeconds, Scenario: scenario,
		GroupRatio:                strconv.FormatFloat(filter.GroupRatio, 'f', -1, 64),
		RequestedMinimumMarginPPM: filter.MinimumMarginPPM, ConfiguredMinimumMarginBPS: row.MinimumExpectedMarginBPS,
		Eligible: false, RevenueSource: "runtime_billing_settings",
	}

	rule := rules[CostRuleCandidate{ChannelID: row.ChannelID, BillableUpstreamModel: row.UpstreamModel, CostVariantKey: row.CostVariantKey}]
	if rule == nil {
		item.FailureReason = string(ProfitReasonCostRuleMissing)
		return item
	}
	item.RuleID = rule.ID
	item.RuleVersion = rule.Version
	item.CostMode = types.CostMode(rule.CostMode)
	item.CostSource = rule.Source

	duration := filter.DurationSeconds
	hasVideo := scenario == RouteMarginScenarioWithVideo
	ratio := filter.GroupRatio
	revenueNanoUSD, revenueErr := PreviewRoutingRevenue(ctx, RoutingRevenuePreviewInput{
		OriginModelName: row.CanonicalModel, Group: row.GroupName,
		RequestPath: "/v1/video/generations", RelayMode: relayconstant.RelayModeVideoSubmit,
		DurationSeconds: &duration, OutputResolution: resolution, HasReferenceVideo: hasVideo,
		InputVideoDurationMS: 0, UserId: 0, GroupRatioOverride: &ratio,
	})
	facts := ProfitRoutingFacts{OutputDurationSeconds: duration}
	if profile, ok := seedancepricing.Profile(resolution); ok {
		facts.Width, facts.Height = profile.Width, profile.Height
		facts.FrameRateNum, facts.FrameRateDen = profile.FrameRateNum, profile.FrameRateDen
	}
	if types.CostMode(rule.CostMode) == types.CostModePerToken {
		if facts.Width <= 0 {
			item.FailureReason = string(ProfitReasonMeterUnknown)
			return item
		}
		inputTokens, outputTokens, totalTokens, tokenErr := EstimateSeedanceTokens(facts)
		if tokenErr != nil {
			item.FailureReason = string(ProfitReasonMeterUnknown)
			return item
		}
		facts.InputTokens, facts.OutputTokens, facts.TotalTokens = inputTokens, outputTokens, totalTokens
	}

	evaluation := evaluateCandidateProfit(ProfitChannelFilterInput{
		Ctx: ctx, Facts: facts, RevenueNanoUSD: revenueNanoUSD, HasRevenue: revenueErr == nil,
		GlobalMarginBPS: -10000,
	}, rules, ProfitRoutingCandidate{ChannelID: row.ChannelID, PredictedUpstreamModel: row.UpstreamModel, CostVariantKey: row.CostVariantKey}, 0, false, nil)
	if revenueErr != nil && evaluation.Reason == "" {
		evaluation.Reason = ProfitReasonRevenueUnknown
	}
	if evaluation.EstimatedRevenueNanoUSD > 0 {
		value := evaluation.EstimatedRevenueNanoUSD
		item.EstimatedRevenueNanoUSD = &value
	}
	item.EstimatedCostNanoUSD = evaluation.EstimatedCostNanoUSD
	item.EstimatedProfitNanoUSD = evaluation.EstimatedProfitNanoUSD
	item.GrossMarginPPM = evaluation.GrossMarginPPM
	if evaluation.Reason != "" {
		item.FailureReason = string(evaluation.Reason)
	}
	if item.GrossMarginPPM != nil {
		item.Eligible = *item.GrossMarginPPM >= filter.MinimumMarginPPM
		if !item.Eligible {
			item.FailureReason = string(ProfitReasonMarginBelowThreshold)
		}
	}
	return item
}

func summarizeRouteMarginItems(items []dto.RouteMarginCatalogItem) dto.RouteMarginCatalogSummary {
	targets := make(map[int]struct{})
	byTarget := make(map[int][]dto.RouteMarginCatalogItem)
	for _, item := range items {
		targets[item.TargetID] = struct{}{}
		byTarget[item.TargetID] = append(byTarget[item.TargetID], item)
	}
	summary := dto.RouteMarginCatalogSummary{TargetCount: len(targets), ScenarioCount: len(items)}
	for _, targetItems := range byTarget {
		eligible := 0
		for _, item := range targetItems {
			if item.Eligible {
				eligible++
				summary.EligibleScenarioCount++
			}
		}
		switch {
		case eligible == 0:
			summary.IneligibleTargetCount++
		case eligible == len(targetItems):
			summary.FullyEligibleTargetCount++
			summary.EligibleTargetCount++
		default:
			summary.PartiallyEligibleTargetCount++
			summary.EligibleTargetCount++
		}
	}
	return summary
}

func routeMarginFacets(items []dto.RouteMarginCatalogItem) dto.RouteMarginCatalogFacets {
	resolutions := make(map[string]struct{})
	models := make(map[string]struct{})
	channels := make(map[int]dto.CostCatalogChannelFacet)
	for _, item := range items {
		resolutions[item.Resolution] = struct{}{}
		models[item.CanonicalModel] = struct{}{}
		channels[item.ChannelID] = dto.CostCatalogChannelFacet{ID: item.ChannelID, Name: item.ChannelName, Type: item.ChannelType}
	}
	facets := dto.RouteMarginCatalogFacets{Channels: make([]dto.CostCatalogChannelFacet, 0, len(channels)), Resolutions: make([]string, 0, len(resolutions)), CanonicalModels: make([]string, 0, len(models))}
	for _, channel := range channels {
		facets.Channels = append(facets.Channels, channel)
	}
	for resolution := range resolutions {
		facets.Resolutions = append(facets.Resolutions, resolution)
	}
	for modelName := range models {
		facets.CanonicalModels = append(facets.CanonicalModels, modelName)
	}
	sort.Slice(facets.Channels, func(i, j int) bool { return facets.Channels[i].ID < facets.Channels[j].ID })
	sort.Strings(facets.Resolutions)
	sort.Strings(facets.CanonicalModels)
	return facets
}

func sortRouteMarginItems(items []dto.RouteMarginCatalogItem, sortBy, sortOrder string) {
	desc := sortOrder == "desc"
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		cmp := 0
		switch sortBy {
		case "target_name":
			cmp = strings.Compare(left.TargetName, right.TargetName)
		case "channel_name":
			cmp = strings.Compare(left.ChannelName, right.ChannelName)
		case "upstream_model":
			cmp = strings.Compare(left.UpstreamModel, right.UpstreamModel)
		case "gross_margin_ppm":
			cmp = compareNullableInt64(left.GrossMarginPPM, right.GrossMarginPPM)
		case "estimated_profit_nano_usd":
			cmp = compareNullableInt64(left.EstimatedProfitNanoUSD, right.EstimatedProfitNanoUSD)
		}
		if cmp == 0 {
			return strings.Compare(left.TargetName+left.Resolution+left.Scenario, right.TargetName+right.Resolution+right.Scenario) < 0
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareNullableInt64(left, right *int64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

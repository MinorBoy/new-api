package model

import (
	"github.com/QuantumNous/new-api/types"
)

type RouteMarginTargetQuery struct {
	ChannelID      int
	CanonicalModel string
	UpstreamModel  string
	TargetName     string
}

type RouteMarginTargetRow struct {
	TargetID                 int    `gorm:"column:target_id"`
	TargetName               string `gorm:"column:target_name"`
	PolicyID                 int    `gorm:"column:policy_id"`
	GroupName                string `gorm:"column:group_name"`
	CanonicalModel           string `gorm:"column:canonical_model"`
	DefaultResolution        string `gorm:"column:default_resolution"`
	DefaultDuration          int    `gorm:"column:default_duration"`
	DefaultRatio             string `gorm:"column:default_ratio"`
	ChannelID                int    `gorm:"column:channel_id"`
	ChannelName              string `gorm:"column:channel_name"`
	ChannelType              int    `gorm:"column:channel_type"`
	UpstreamModel            string `gorm:"column:upstream_model"`
	CostVariantKey           string `gorm:"column:cost_variant_key"`
	MinimumExpectedMarginBPS *int   `gorm:"column:minimum_expected_margin_bps"`
	Constraints              string `gorm:"column:constraints"`
}

func ListActiveImportedRouteMarginTargets(query RouteMarginTargetQuery) ([]RouteMarginTargetRow, error) {
	policyTable := DB.NamingStrategy.TableName("RoutingPolicy")
	targetTable := DB.NamingStrategy.TableName("RouteTarget")
	channelTable := DB.NamingStrategy.TableName("Channel")

	db := DB.Table(targetTable+" AS targets").
		Select("targets.id AS target_id, targets.name AS target_name, targets.policy_id, "+
			"targets.channel_id, targets.upstream_model, targets.cost_variant_key, "+
			"targets.minimum_expected_margin_bps, targets.constraints, "+
			"policies.group_name, policies.model AS canonical_model, "+
			"policies.default_resolution, policies.default_duration, policies.default_ratio, "+
			"COALESCE(channels.name, '') AS channel_name, COALESCE(channels.type, 0) AS channel_type").
		Joins("JOIN "+policyTable+" AS policies ON policies.id = targets.policy_id").
		Joins("LEFT JOIN "+channelTable+" AS channels ON channels.id = targets.channel_id").
		Where("policies.enabled = ? AND targets.enabled = ?", true, true).
		Where("targets.retired_at IS NULL AND targets.managed_by = ?", string(types.RouteTargetManagedByConfigImport))
	if query.ChannelID > 0 {
		db = db.Where("targets.channel_id = ?", query.ChannelID)
	}
	if query.CanonicalModel != "" {
		db = db.Where("policies.model = ?", query.CanonicalModel)
	}
	if query.UpstreamModel != "" {
		db = db.Where("targets.upstream_model = ?", query.UpstreamModel)
	}
	if query.TargetName != "" {
		db = db.Where("targets.name = ?", query.TargetName)
	}

	rows := make([]RouteMarginTargetRow, 0)
	err := db.Order("targets.id ASC").Scan(&rows).Error
	return rows, err
}

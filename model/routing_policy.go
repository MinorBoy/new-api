package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

type RoutingPolicy struct {
	ID                int           `json:"id"`
	GroupName         string        `json:"group_name" gorm:"type:varchar(64);not null;uniqueIndex:idx_routing_policy_group_model,priority:1"`
	Model             string        `json:"model" gorm:"type:varchar(191);not null;uniqueIndex:idx_routing_policy_group_model,priority:2"`
	Enabled           bool          `json:"enabled" gorm:"not null"`
	DefaultResolution string        `json:"default_resolution" gorm:"type:varchar(16);not null"`
	DefaultDuration   int           `json:"default_duration" gorm:"not null"`
	DefaultRatio      string        `json:"default_ratio" gorm:"type:varchar(16);not null"`
	CreatedAt         int64         `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64         `json:"updated_at" gorm:"autoUpdateTime"`
	Targets           []RouteTarget `json:"targets" gorm:"-"`
}

type RouteTarget struct {
	ID                       int    `json:"id"`
	PolicyID                 int    `json:"policy_id" gorm:"not null;index"`
	ChannelID                int    `json:"channel_id" gorm:"not null;index"`
	Name                     string `json:"name" gorm:"type:varchar(128);not null"`
	UpstreamModel            string `json:"upstream_model" gorm:"type:varchar(255);not null"`
	CostVariantKey           string `json:"cost_variant_key" gorm:"type:varchar(64);not null;index"`
	TargetPriority           int    `json:"target_priority" gorm:"not null;index"`
	MinimumExpectedMarginBPS *int   `json:"minimum_expected_margin_bps"`
	Constraints              string `json:"constraints" gorm:"type:text;not null"`
	Enabled                  bool   `json:"enabled" gorm:"not null"`
	ManagedBy                string `json:"managed_by" gorm:"type:varchar(32);not null;index"`
	SourceBatchID            *int64 `json:"source_batch_id,omitempty" gorm:"index"`
	RetiredAt                *int64 `json:"retired_at,omitempty" gorm:"index"`
	CreatedAt                int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt                int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type RoutingCandidateChannel struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Status   int    `json:"status"`
	Priority int64  `json:"priority"`
	Weight   uint   `json:"weight"`
}

type RoutingAvailabilityKey struct {
	CanonicalModel string
	ChannelID      int
}

func ListEnabledRoutingPoliciesByGroup(groupName string) ([]RoutingPolicy, error) {
	return ListEnabledRoutingPoliciesByGroupWithDB(DB, groupName)
}

func ListEnabledRoutingPoliciesByGroupWithDB(db *gorm.DB, groupName string) ([]RoutingPolicy, error) {
	var policies []RoutingPolicy
	if err := db.Where("group_name = ? AND enabled = ?", groupName, true).
		Order("model ASC").
		Order("id ASC").
		Find(&policies).Error; err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return policies, nil
	}

	policyIDs := make([]int, 0, len(policies))
	policiesByID := make(map[int]*RoutingPolicy, len(policies))
	for index := range policies {
		policyIDs = append(policyIDs, policies[index].ID)
		policiesByID[policies[index].ID] = &policies[index]
	}
	var targets []RouteTarget
	if err := db.Where("policy_id IN ?", policyIDs).
		Order("policy_id ASC").
		Order("channel_id ASC").
		Order("target_priority DESC").
		Order("id ASC").
		Find(&targets).Error; err != nil {
		return nil, err
	}
	for _, target := range targets {
		policiesByID[target.PolicyID].Targets = append(policiesByID[target.PolicyID].Targets, target)
	}
	return policies, nil
}

func ListRoutingAvailability(groupName string, canonicalModels []string) (map[RoutingAvailabilityKey]struct{}, error) {
	if common.MemoryCacheEnabled {
		return listRoutingAvailabilityFromCache(groupName, canonicalModels), nil
	}
	return ListRoutingAvailabilityWithDB(DB, groupName, canonicalModels)
}

func ListRoutingAvailabilityWithDB(db *gorm.DB, groupName string, canonicalModels []string) (map[RoutingAvailabilityKey]struct{}, error) {
	available := make(map[RoutingAvailabilityKey]struct{})
	models := make([]string, 0, len(canonicalModels))
	modelSet := make(map[string]struct{}, len(canonicalModels))
	for _, modelName := range canonicalModels {
		normalized := modelrouting.NormalizeCanonicalModel(modelName)
		if normalized == "" {
			continue
		}
		if _, exists := modelSet[normalized]; exists {
			continue
		}
		modelSet[normalized] = struct{}{}
		models = append(models, normalized)
	}
	if groupName == "" || len(models) == 0 {
		return available, nil
	}

	var abilities []Ability
	if err := db.Select("model", "channel_id").
		Where(commonGroupCol+" = ? AND model IN ? AND enabled = ?", groupName, models, true).
		Find(&abilities).Error; err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		return available, nil
	}
	channelIDs := make([]int, 0, len(abilities))
	channelIDSet := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, exists := channelIDSet[ability.ChannelId]; exists {
			continue
		}
		channelIDSet[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	var enabledChannelIDs []int
	if err := db.Model(&Channel{}).
		Where("id IN ? AND status = ?", channelIDs, common.ChannelStatusEnabled).
		Pluck("id", &enabledChannelIDs).Error; err != nil {
		return nil, err
	}
	enabledChannels := make(map[int]struct{}, len(enabledChannelIDs))
	for _, channelID := range enabledChannelIDs {
		enabledChannels[channelID] = struct{}{}
	}
	for _, ability := range abilities {
		if _, enabled := enabledChannels[ability.ChannelId]; !enabled {
			continue
		}
		canonicalModel := modelrouting.NormalizeCanonicalModel(ability.Model)
		available[RoutingAvailabilityKey{CanonicalModel: canonicalModel, ChannelID: ability.ChannelId}] = struct{}{}
	}
	return available, nil
}

func listRoutingAvailabilityFromCache(groupName string, canonicalModels []string) map[RoutingAvailabilityKey]struct{} {
	available := make(map[RoutingAvailabilityKey]struct{})
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	for _, modelName := range canonicalModels {
		canonicalModel := modelrouting.NormalizeCanonicalModel(modelName)
		if canonicalModel == "" {
			continue
		}
		for _, channelID := range group2model2channels[groupName][canonicalModel] {
			available[RoutingAvailabilityKey{CanonicalModel: canonicalModel, ChannelID: channelID}] = struct{}{}
		}
	}
	return available
}

func ListRoutingPolicies(groupName, canonicalModel string, channelID, offset, limit int) ([]RoutingPolicy, int64, error) {
	query := DB.Model(&RoutingPolicy{})
	if groupName != "" {
		query = query.Where("group_name = ?", groupName)
	}
	if canonicalModel != "" {
		query = query.Where("model = ?", canonicalModel)
	}
	if channelID > 0 {
		subquery := DB.Model(&RouteTarget{}).Select("policy_id").Where("channel_id = ?", channelID)
		query = query.Where("id IN (?)", subquery)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var policies []RoutingPolicy
	if err := query.Order("updated_at DESC").Order("id DESC").Offset(offset).Limit(limit).Find(&policies).Error; err != nil {
		return nil, 0, err
	}
	if len(policies) == 0 {
		return policies, total, nil
	}
	policyIDs := make([]int, 0, len(policies))
	byID := make(map[int]*RoutingPolicy, len(policies))
	for index := range policies {
		policyIDs = append(policyIDs, policies[index].ID)
		byID[policies[index].ID] = &policies[index]
	}
	var targets []RouteTarget
	if err := DB.Where("policy_id IN ?", policyIDs).
		Order("channel_id ASC, target_priority DESC, id ASC").
		Find(&targets).Error; err != nil {
		return nil, 0, err
	}
	for _, target := range targets {
		policy := byID[target.PolicyID]
		policy.Targets = append(policy.Targets, target)
	}
	return policies, total, nil
}

func ReplaceRoutingPolicy(id int, policy RoutingPolicy, targets []RouteTarget) (*RoutingPolicy, error) {
	var saved *RoutingPolicy
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		saved, err = ReplaceRoutingPolicyWithTx(tx, id, policy, targets)
		return err
	})
	if err != nil {
		return nil, err
	}
	return GetRoutingPolicy(saved.ID)
}

// ReplaceRoutingPolicyWithTx persists a policy and its targets using the
// caller's transaction. Cache refresh belongs to the post-commit caller.
func ReplaceRoutingPolicyWithTx(tx *gorm.DB, id int, policy RoutingPolicy, targets []RouteTarget) (*RoutingPolicy, error) {
	if tx == nil {
		return nil, fmt.Errorf("routing policy transaction is required")
	}
	snapshot := modelrouting.PolicySnapshot{
		ID:             id,
		GroupName:      policy.GroupName,
		CanonicalModel: policy.Model,
		Enabled:        policy.Enabled,
		Defaults: modelrouting.Defaults{
			OutputResolution: policy.DefaultResolution,
			DurationSeconds:  policy.DefaultDuration,
			AspectRatio:      policy.DefaultRatio,
		},
		TargetsByChannel: make(map[int][]modelrouting.Target),
	}
	for _, target := range targets {
		var constraints modelrouting.Constraints
		if err := common.UnmarshalJsonStr(target.Constraints, &constraints); err != nil {
			return nil, fmt.Errorf("decode route target %q constraints: %w", target.Name, err)
		}
		snapshot.TargetsByChannel[target.ChannelID] = append(snapshot.TargetsByChannel[target.ChannelID], modelrouting.Target{
			ID:                       target.ID,
			PolicyID:                 id,
			ChannelID:                target.ChannelID,
			Name:                     target.Name,
			UpstreamModel:            target.UpstreamModel,
			CostVariantKey:           target.CostVariantKey,
			Priority:                 target.TargetPriority,
			MinimumExpectedMarginBPS: target.MinimumExpectedMarginBPS,
			Enabled:                  target.Enabled,
			Constraints:              constraints,
		})
	}
	if err := modelrouting.ValidatePolicy(snapshot, relaycommon.MaxTaskDurationSeconds); err != nil {
		return nil, err
	}

	policy.Targets = nil
	existingByID := make(map[int]RouteTarget)
	var existingTargets []RouteTarget
	if id == 0 {
		policy.ID = 0
		if err := tx.Create(&policy).Error; err != nil {
			return nil, err
		}
	} else {
		var existing RoutingPolicy
		if err := tx.First(&existing, "id = ?", id).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("policy_id = ?", id).Find(&existingTargets).Error; err != nil {
			return nil, err
		}
		for _, target := range existingTargets {
			existingByID[target.ID] = target
		}
		updates := map[string]interface{}{
			"group_name":         policy.GroupName,
			"model":              policy.Model,
			"enabled":            policy.Enabled,
			"default_resolution": policy.DefaultResolution,
			"default_duration":   policy.DefaultDuration,
			"default_ratio":      policy.DefaultRatio,
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return nil, err
		}
		policy.ID = id
	}

	persistedTargets := make([]RouteTarget, len(targets))
	copy(persistedTargets, targets)
	seen := make(map[int]struct{})
	for index := range persistedTargets {
		target := &persistedTargets[index]
		normalized, err := types.NormalizeCostVariantKey(target.CostVariantKey)
		if err != nil {
			return nil, err
		}
		target.CostVariantKey = normalized
		if target.ID < 0 {
			target.ID = 0
		}
		target.PolicyID = policy.ID
		target.CreatedAt = 0
		target.UpdatedAt = 0
		if target.ID == 0 {
			target.ManagedBy = string(types.RouteTargetManagedByManual)
			target.SourceBatchID = nil
			target.RetiredAt = nil
			if err := tx.Create(target).Error; err != nil {
				return nil, err
			}
			continue
		}

		existing, ok := existingByID[target.ID]
		if !ok {
			return nil, fmt.Errorf("route target %d does not belong to policy %d", target.ID, policy.ID)
		}
		if _, duplicate := seen[target.ID]; duplicate {
			return nil, fmt.Errorf("route target %d is duplicated in policy %d", target.ID, policy.ID)
		}
		seen[target.ID] = struct{}{}
		managedBy, err := types.NormalizeRouteTargetManagedBy(existing.ManagedBy)
		if err != nil {
			return nil, err
		}
		target.ManagedBy = string(managedBy)
		target.SourceBatchID = existing.SourceBatchID
		if target.Enabled {
			target.RetiredAt = nil
		} else {
			target.RetiredAt = existing.RetiredAt
		}
		if err := tx.Model(&RouteTarget{}).
			Where("id = ? AND policy_id = ?", target.ID, policy.ID).
			Select("channel_id", "name", "upstream_model", "cost_variant_key", "target_priority", "minimum_expected_margin_bps", "constraints", "enabled", "retired_at", "updated_at").
			Updates(target).Error; err != nil {
			return nil, err
		}
	}

	now := common.GetTimestamp()
	for _, existing := range existingTargets {
		if _, ok := seen[existing.ID]; ok {
			continue
		}
		managedBy, err := types.NormalizeRouteTargetManagedBy(existing.ManagedBy)
		if err != nil {
			return nil, err
		}
		if managedBy == types.RouteTargetManagedByConfigImport {
			if err := tx.Model(&RouteTarget{}).Where("id = ?", existing.ID).Updates(map[string]any{
				"enabled": false, "retired_at": now, "updated_at": now,
			}).Error; err != nil {
				return nil, err
			}
			continue
		}
		if err := tx.Delete(&RouteTarget{}, existing.ID).Error; err != nil {
			return nil, err
		}
	}

	if err := tx.Where("policy_id = ?", policy.ID).
		Order("channel_id ASC, target_priority DESC, id ASC").
		Find(&policy.Targets).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func GetRoutingPolicy(id int) (*RoutingPolicy, error) {
	var policy RoutingPolicy
	if err := DB.First(&policy, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := DB.Where("policy_id = ?", id).Order("channel_id ASC, target_priority DESC, id ASC").Find(&policy.Targets).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func DeleteRoutingPolicy(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("policy_id = ?", id).Delete(&RouteTarget{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&RoutingPolicy{}).Error
	})
}

func ListRoutingCandidates(groupName, canonicalModel string) ([]RoutingCandidateChannel, error) {
	var channelIDs []int
	if err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? AND model = ?", groupName, canonicalModel).
		Distinct("channel_id").
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return nil, err
	}
	if len(channelIDs) == 0 {
		return []RoutingCandidateChannel{}, nil
	}

	var candidates []RoutingCandidateChannel
	if err := DB.Model(&Channel{}).
		Select("id, name, status, priority, weight").
		Where("id IN ?", channelIDs).
		Order("priority DESC").
		Order("id ASC").
		Scan(&candidates).Error; err != nil {
		return nil, err
	}
	return candidates, nil
}

func RefreshRoutingPolicyCacheByChannelIDs(channelIDs []int) error {
	keys, err := routingPolicyKeysByChannelIDs(DB, channelIDs)
	if err != nil {
		return err
	}
	return RefreshRoutingPolicyCacheKeys(keys)
}

func deleteRouteTargetsForChannels(tx *gorm.DB, channelIDs []int) ([]RoutingPolicyKey, error) {
	keys, err := routingPolicyKeysByChannelIDs(tx, channelIDs)
	if err != nil {
		return nil, err
	}
	if len(channelIDs) > 0 {
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&RouteTarget{}).Error; err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func routingPolicyKeysByChannelIDs(db *gorm.DB, channelIDs []int) ([]RoutingPolicyKey, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	var policyIDs []int
	if err := db.Model(&RouteTarget{}).
		Where("channel_id IN ?", channelIDs).
		Distinct("policy_id").
		Pluck("policy_id", &policyIDs).Error; err != nil {
		return nil, err
	}
	if len(policyIDs) == 0 {
		return nil, nil
	}
	var policies []RoutingPolicy
	if err := db.Select("id, group_name, model").Where("id IN ?", policyIDs).Find(&policies).Error; err != nil {
		return nil, err
	}
	keys := make([]RoutingPolicyKey, 0, len(policies))
	for _, policy := range policies {
		keys = append(keys, RoutingPolicyKey{GroupName: policy.GroupName, Model: policy.Model})
	}
	return keys, nil
}

package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
	ID             int    `json:"id"`
	PolicyID       int    `json:"policy_id" gorm:"not null;index"`
	ChannelID      int    `json:"channel_id" gorm:"not null;index"`
	Name           string `json:"name" gorm:"type:varchar(128);not null"`
	UpstreamModel  string `json:"upstream_model" gorm:"type:varchar(255);not null"`
	TargetPriority int    `json:"target_priority" gorm:"not null;index"`
	Constraints    string `json:"constraints" gorm:"type:text;not null"`
	Enabled        bool   `json:"enabled" gorm:"not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type RoutingCandidateChannel struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Status   int    `json:"status"`
	Priority int64  `json:"priority"`
	Weight   uint   `json:"weight"`
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
			ID:            target.ID,
			PolicyID:      id,
			ChannelID:     target.ChannelID,
			Name:          target.Name,
			UpstreamModel: target.UpstreamModel,
			Priority:      target.TargetPriority,
			Enabled:       target.Enabled,
			Constraints:   constraints,
		})
	}
	if err := modelrouting.ValidatePolicy(snapshot, relaycommon.MaxTaskDurationSeconds); err != nil {
		return nil, err
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			tx.Rollback()
			panic(recovered)
		}
	}()

	policy.Targets = nil
	if id == 0 {
		policy.ID = 0
		if err := tx.Create(&policy).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	} else {
		var existing RoutingPolicy
		if err := tx.First(&existing, "id = ?", id).Error; err != nil {
			tx.Rollback()
			return nil, err
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
			tx.Rollback()
			return nil, err
		}
		policy.ID = id
		if err := tx.Where("policy_id = ?", id).Delete(&RouteTarget{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if len(targets) > 0 {
		persistedTargets := make([]RouteTarget, len(targets))
		copy(persistedTargets, targets)
		for index := range persistedTargets {
			persistedTargets[index].ID = 0
			persistedTargets[index].PolicyID = policy.ID
			persistedTargets[index].CreatedAt = 0
			persistedTargets[index].UpdatedAt = 0
		}
		if err := tx.Create(&persistedTargets).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return GetRoutingPolicy(policy.ID)
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

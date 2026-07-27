package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

var (
	ErrCostRuleStateConflict  = errors.New("cost rule state conflict")
	ErrCostActiveRuleConflict = errors.New("multiple active cost rules found")
)

type ChannelModelCostRule struct {
	ID                    int64  `json:"id" gorm:"primaryKey"`
	ChannelID             int    `json:"channel_id" gorm:"uniqueIndex:idx_cost_rule_version,priority:1;index"`
	BillableUpstreamModel string `json:"billable_upstream_model" gorm:"type:varchar(191);uniqueIndex:idx_cost_rule_version,priority:2;index"`
	CostVariantKey        string `json:"cost_variant_key" gorm:"type:varchar(64);not null;uniqueIndex:idx_cost_rule_version,priority:3;index"`
	Version               int    `json:"version" gorm:"uniqueIndex:idx_cost_rule_version,priority:4"`
	Status                string `json:"status" gorm:"type:varchar(32);index"`
	CostMode              string `json:"cost_mode" gorm:"type:varchar(32)"`
	SchemaVersion         int    `json:"schema_version"`
	ConfigJSON            string `json:"config_json" gorm:"type:text"`
	Source                string `json:"source" gorm:"type:varchar(32)"`
	Note                  string `json:"note" gorm:"type:text"`
	CreatedBy             int    `json:"created_by"`
	ActivatedBy           int    `json:"activated_by"`
	EffectiveFrom         *int64 `json:"effective_from,omitempty" gorm:"index"`
	EffectiveTo           *int64 `json:"effective_to,omitempty"`
	CreatedAt             int64  `json:"created_at"`
	UpdatedAt             int64  `json:"updated_at"`
}

func CreateCostRuleDraft(rule *ChannelModelCostRule) error {
	return CreateCostRuleDraftWithTx(DB, rule)
}

// CreateCostRuleDraftWithTx writes a normalized draft using the caller's
// transaction. It intentionally performs no cache refresh because a draft is
// inactive until an explicit activation step.
func CreateCostRuleDraftWithTx(tx *gorm.DB, rule *ChannelModelCostRule) error {
	if tx == nil {
		return errors.New("cost rule transaction is required")
	}
	if rule == nil {
		return errors.New("cost rule is required")
	}
	if rule.Status == "" {
		rule.Status = string(types.CostRuleDraft)
	}
	if rule.Status != string(types.CostRuleDraft) {
		return ErrCostRuleStateConflict
	}
	normalized, err := types.NormalizeCostVariantKey(rule.CostVariantKey)
	if err != nil {
		return err
	}
	rule.CostVariantKey = normalized
	now := common.GetTimestamp()
	if rule.CreatedAt == 0 {
		rule.CreatedAt = now
	}
	if rule.UpdatedAt == 0 {
		rule.UpdatedAt = now
	}
	return tx.Create(rule).Error
}

func ActivateChannelModelCostRule(id int64, adminID int, now int64, validate func(*ChannelModelCostRule) error) (*ChannelModelCostRule, error) {
	var activated *ChannelModelCostRule
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		activated, err = ActivateChannelModelCostRuleWithTx(tx, id, adminID, now, validate)
		return err
	})
	if err != nil {
		return nil, err
	}
	return activated, nil
}

// ActivateChannelModelCostRuleWithTx atomically promotes one draft while the
// caller owns the surrounding transaction. It intentionally leaves cache
// refresh to the post-commit publication flow.
func ActivateChannelModelCostRuleWithTx(tx *gorm.DB, id int64, adminID int, now int64, validate func(*ChannelModelCostRule) error) (*ChannelModelCostRule, error) {
	if tx == nil {
		return nil, errors.New("cost rule transaction is required")
	}
	var activated ChannelModelCostRule
	var candidate ChannelModelCostRule
	if err := tx.Where("id = ?", id).First(&candidate).Error; err != nil {
		return nil, err
	}

	var businessRules []ChannelModelCostRule
	if err := lockForUpdate(tx).
		Where(
			"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ?",
			candidate.ChannelID,
			candidate.BillableUpstreamModel,
			candidate.CostVariantKey,
		).
		Order("id ASC").
		Find(&businessRules).Error; err != nil {
		return nil, err
	}
	var draft *ChannelModelCostRule
	for i := range businessRules {
		if businessRules[i].ID == id {
			draft = &businessRules[i]
			break
		}
	}
	if draft == nil || draft.Status != string(types.CostRuleDraft) {
		return nil, ErrCostRuleStateConflict
	}
	if validate != nil {
		if err := validate(draft); err != nil {
			return nil, err
		}
	}

	activeRules := make([]ChannelModelCostRule, 0, 1)
	for _, rule := range businessRules {
		if rule.Status == string(types.CostRuleActive) {
			activeRules = append(activeRules, rule)
		}
	}
	if len(activeRules) > 1 {
		return nil, ErrCostActiveRuleConflict
	}
	if len(activeRules) == 1 {
		result := tx.Model(&ChannelModelCostRule{}).
			Where("id = ? AND status = ?", activeRules[0].ID, types.CostRuleActive).
			Updates(map[string]any{
				"status":       string(types.CostRuleRetired),
				"effective_to": now,
				"updated_at":   now,
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrCostRuleStateConflict
		}
	}

	result := tx.Model(&ChannelModelCostRule{}).
		Where("id = ? AND status = ?", draft.ID, types.CostRuleDraft).
		Updates(map[string]any{
			"status":         string(types.CostRuleActive),
			"activated_by":   adminID,
			"effective_from": now,
			"effective_to":   nil,
			"updated_at":     now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrCostRuleStateConflict
	}
	if err := tx.First(&activated, draft.ID).Error; err != nil {
		return nil, err
	}
	return &activated, nil
}

func RetireChannelModelCostRule(id int64, adminID int, now int64) error {
	_ = adminID
	return DB.Transaction(func(tx *gorm.DB) error {
		var rule ChannelModelCostRule
		if err := lockForUpdate(tx).Where("id = ?", id).First(&rule).Error; err != nil {
			return err
		}
		if rule.Status != string(types.CostRuleActive) {
			return ErrCostRuleStateConflict
		}
		result := tx.Model(&ChannelModelCostRule{}).
			Where("id = ? AND status = ?", id, types.CostRuleActive).
			Updates(map[string]any{
				"status":       string(types.CostRuleRetired),
				"effective_to": now,
				"updated_at":   now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: rule %d", ErrCostRuleStateConflict, id)
		}
		return nil
	})
}

package model

import (
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"gorm.io/gorm"
)

type RoutingPolicyKey struct {
	GroupName string
	Model     string
}

var routingPolicySnapshots atomic.Value
var routingPolicyCacheMu sync.Mutex

func init() {
	routingPolicySnapshots.Store(map[RoutingPolicyKey]modelrouting.PolicySnapshot{})
}

func GetRoutingPolicySnapshot(group, canonicalModel string) (modelrouting.PolicySnapshot, bool) {
	key := RoutingPolicyKey{GroupName: group, Model: canonicalModel}
	snapshot, ok := routingPolicySnapshots.Load().(map[RoutingPolicyKey]modelrouting.PolicySnapshot)[key]
	return snapshot, ok
}

func InitRoutingPolicyCache() error {
	next, err := loadAllRoutingPolicySnapshots()
	if err != nil {
		return err
	}
	routingPolicyCacheMu.Lock()
	routingPolicySnapshots.Store(next)
	routingPolicyCacheMu.Unlock()
	return nil
}

func RefreshRoutingPolicyCache(group, canonicalModel string) error {
	routingPolicyCacheMu.Lock()
	defer routingPolicyCacheMu.Unlock()

	key := RoutingPolicyKey{GroupName: group, Model: canonicalModel}
	snapshot, enabled, err := loadRoutingPolicySnapshot(key)
	if err != nil {
		return err
	}
	current := routingPolicySnapshots.Load().(map[RoutingPolicyKey]modelrouting.PolicySnapshot)
	next := maps.Clone(current)
	if enabled {
		next[key] = snapshot
	} else {
		delete(next, key)
	}
	routingPolicySnapshots.Store(next)
	return nil
}

func SyncRoutingPolicyCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if err := InitRoutingPolicyCache(); err != nil {
			common.SysError("failed to sync routing policy cache: " + err.Error())
		}
	}
}

func loadAllRoutingPolicySnapshots() (map[RoutingPolicyKey]modelrouting.PolicySnapshot, error) {
	var policies []RoutingPolicy
	if err := DB.Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return nil, err
	}
	next := make(map[RoutingPolicyKey]modelrouting.PolicySnapshot, len(policies))
	if len(policies) == 0 {
		return next, nil
	}

	policyIDs := make([]int, 0, len(policies))
	for _, policy := range policies {
		policyIDs = append(policyIDs, policy.ID)
	}
	var targets []RouteTarget
	if err := DB.Where("policy_id IN ? AND enabled = ?", policyIDs, true).
		Order("policy_id ASC, channel_id ASC, target_priority DESC, id ASC").
		Find(&targets).Error; err != nil {
		return nil, err
	}
	targetsByPolicy := make(map[int][]RouteTarget, len(policies))
	for _, target := range targets {
		targetsByPolicy[target.PolicyID] = append(targetsByPolicy[target.PolicyID], target)
	}
	for _, policy := range policies {
		snapshot, err := routingPolicySnapshotFromRows(policy, targetsByPolicy[policy.ID])
		if err != nil {
			return nil, err
		}
		key := RoutingPolicyKey{GroupName: policy.GroupName, Model: policy.Model}
		next[key] = snapshot
	}
	return next, nil
}

func loadRoutingPolicySnapshot(key RoutingPolicyKey) (modelrouting.PolicySnapshot, bool, error) {
	var policy RoutingPolicy
	err := DB.Where("group_name = ? AND model = ? AND enabled = ?", key.GroupName, key.Model, true).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return modelrouting.PolicySnapshot{}, false, nil
	}
	if err != nil {
		return modelrouting.PolicySnapshot{}, false, err
	}

	var targets []RouteTarget
	if err := DB.Where("policy_id = ? AND enabled = ?", policy.ID, true).
		Order("channel_id ASC, target_priority DESC, id ASC").
		Find(&targets).Error; err != nil {
		return modelrouting.PolicySnapshot{}, false, err
	}
	snapshot, err := routingPolicySnapshotFromRows(policy, targets)
	if err != nil {
		return modelrouting.PolicySnapshot{}, false, err
	}
	return snapshot, true, nil
}

func routingPolicySnapshotFromRows(policy RoutingPolicy, targets []RouteTarget) (modelrouting.PolicySnapshot, error) {
	snapshot := modelrouting.PolicySnapshot{
		ID:             policy.ID,
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
			return modelrouting.PolicySnapshot{}, fmt.Errorf("decode route target %d constraints: %w", target.ID, err)
		}
		snapshot.TargetsByChannel[target.ChannelID] = append(snapshot.TargetsByChannel[target.ChannelID], modelrouting.Target{
			ID:            target.ID,
			PolicyID:      target.PolicyID,
			ChannelID:     target.ChannelID,
			Name:          target.Name,
			UpstreamModel: target.UpstreamModel,
			Priority:      target.TargetPriority,
			Enabled:       target.Enabled,
			Constraints:   constraints,
		})
	}
	if err := modelrouting.ValidatePolicy(snapshot, relaycommon.MaxTaskDurationSeconds); err != nil {
		return modelrouting.PolicySnapshot{}, fmt.Errorf("validate routing policy %d: %w", policy.ID, err)
	}
	return snapshot, nil
}

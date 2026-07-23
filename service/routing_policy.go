package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"gorm.io/gorm"
)

type RoutingPolicyWriteRequest struct {
	GroupName string                    `json:"group_name"`
	Model     string                    `json:"model"`
	Enabled   bool                      `json:"enabled"`
	Defaults  modelrouting.Defaults     `json:"defaults"`
	Targets   []RouteTargetWriteRequest `json:"targets"`
}

type RouteTargetWriteRequest struct {
	ChannelID      int                      `json:"channel_id"`
	Name           string                   `json:"name"`
	UpstreamModel  string                   `json:"upstream_model"`
	TargetPriority int                      `json:"target_priority"`
	Enabled        bool                     `json:"enabled"`
	Constraints    modelrouting.Constraints `json:"constraints"`
}

type RoutingPolicyServiceError struct {
	Code          string `json:"code"`
	Field         string `json:"field,omitempty"`
	TargetIndexes []int  `json:"target_indexes,omitempty"`
	Err           error  `json:"-"`
}

func (e *RoutingPolicyServiceError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Err.Error()
}

func (e *RoutingPolicyServiceError) Unwrap() error {
	return e.Err
}

type RoutingPolicyView struct {
	ID        int                   `json:"id"`
	GroupName string                `json:"group_name"`
	Model     string                `json:"model"`
	Enabled   bool                  `json:"enabled"`
	Defaults  modelrouting.Defaults `json:"defaults"`
	Targets   []RouteTargetView     `json:"targets"`
	CreatedAt int64                 `json:"created_at"`
	UpdatedAt int64                 `json:"updated_at"`
}

type RouteTargetView struct {
	ID             int                      `json:"id"`
	ChannelID      int                      `json:"channel_id"`
	ChannelName    string                   `json:"channel_name"`
	Name           string                   `json:"name"`
	UpstreamModel  string                   `json:"upstream_model"`
	TargetPriority int                      `json:"target_priority"`
	Enabled        bool                     `json:"enabled"`
	Constraints    modelrouting.Constraints `json:"constraints"`
}

func SaveRoutingPolicy(id int, request RoutingPolicyWriteRequest) (*RoutingPolicyView, error) {
	normalizeRoutingPolicyWriteRequest(&request)
	if request.GroupName == "" || strings.EqualFold(request.GroupName, "auto") {
		return nil, newRoutingPolicyServiceError("invalid_group", "group_name", "group_name must be a concrete group")
	}

	candidates, err := model.ListRoutingCandidates(request.GroupName, request.Model)
	if err != nil {
		return nil, newRoutingPolicyServiceError("routing_policy_error", "", err.Error())
	}
	candidateNames := make(map[int]string, len(candidates))
	for _, candidate := range candidates {
		candidateNames[candidate.ID] = candidate.Name
	}
	existingChannelIDs := make(map[int]struct{})
	if id > 0 {
		existing, err := model.GetRoutingPolicy(id)
		if err != nil {
			return nil, err
		}
		if existing.GroupName == request.GroupName && existing.Model == request.Model {
			for _, target := range existing.Targets {
				existingChannelIDs[target.ChannelID] = struct{}{}
			}
		}
	}

	snapshot := modelrouting.PolicySnapshot{
		ID:               id,
		GroupName:        request.GroupName,
		CanonicalModel:   request.Model,
		Enabled:          request.Enabled,
		Defaults:         request.Defaults,
		TargetsByChannel: make(map[int][]modelrouting.Target),
	}
	rows := make([]model.RouteTarget, 0, len(request.Targets))
	for index, target := range request.Targets {
		if target.ChannelID <= 0 {
			return nil, newRoutingPolicyServiceError("invalid_channel", fmt.Sprintf("targets.%d.channel_id", index), "channel is invalid")
		}
		if _, ok := candidateNames[target.ChannelID]; !ok {
			if _, existed := existingChannelIDs[target.ChannelID]; !existed {
				return nil, newRoutingPolicyServiceError("invalid_channel", fmt.Sprintf("targets.%d.channel_id", index), "channel does not declare this group and canonical model")
			}
		}
		if target.Name == "" {
			return nil, newRoutingPolicyServiceError("invalid_target", fmt.Sprintf("targets.%d.name", index), "target name is required")
		}
		if target.UpstreamModel == "" {
			return nil, newRoutingPolicyServiceError("invalid_target", fmt.Sprintf("targets.%d.upstream_model", index), "upstream model is required")
		}
		targetID := -(index + 1)
		snapshot.TargetsByChannel[target.ChannelID] = append(snapshot.TargetsByChannel[target.ChannelID], modelrouting.Target{
			ID:            targetID,
			PolicyID:      id,
			ChannelID:     target.ChannelID,
			Name:          target.Name,
			UpstreamModel: target.UpstreamModel,
			Priority:      target.TargetPriority,
			Enabled:       target.Enabled,
			Constraints:   target.Constraints,
		})
		encoded, err := common.Marshal(target.Constraints)
		if err != nil {
			return nil, newRoutingPolicyServiceError("routing_policy_error", fmt.Sprintf("targets.%d.constraints", index), err.Error())
		}
		rows = append(rows, model.RouteTarget{
			ID:             targetID,
			PolicyID:       id,
			ChannelID:      target.ChannelID,
			Name:           target.Name,
			UpstreamModel:  target.UpstreamModel,
			TargetPriority: target.TargetPriority,
			Enabled:        target.Enabled,
			Constraints:    string(encoded),
		})
	}

	if err := modelrouting.ValidatePolicy(snapshot, relaycommon.MaxTaskDurationSeconds); err != nil {
		return nil, mapRoutingValidationError(err)
	}
	row := model.RoutingPolicy{
		ID:                id,
		GroupName:         request.GroupName,
		Model:             request.Model,
		Enabled:           request.Enabled,
		DefaultResolution: request.Defaults.OutputResolution,
		DefaultDuration:   request.Defaults.DurationSeconds,
		DefaultRatio:      request.Defaults.AspectRatio,
	}
	saved, err := model.ReplaceRoutingPolicy(id, row, rows)
	if err != nil {
		return nil, mapRoutingPersistenceError(err)
	}
	if err := model.RefreshRoutingPolicyCache(saved.GroupName, saved.Model); err != nil {
		return nil, newRoutingPolicyServiceError("routing_policy_error", "", err.Error())
	}
	model.InitChannelCache()
	channelNames, err := routingPolicyChannelNames(saved.Targets)
	if err != nil {
		return nil, err
	}
	return routingPolicyViewFromRow(saved, channelNames)
}

func GetRoutingPolicyView(id int) (*RoutingPolicyView, error) {
	row, err := model.GetRoutingPolicy(id)
	if err != nil {
		return nil, err
	}
	channelNames, err := routingPolicyChannelNames(row.Targets)
	if err != nil {
		return nil, err
	}
	return routingPolicyViewFromRow(row, channelNames)
}

func ListRoutingPolicyViews(groupName, canonicalModel string, channelID, offset, limit int) ([]RoutingPolicyView, int64, error) {
	rows, total, err := model.ListRoutingPolicies(strings.TrimSpace(groupName), strings.TrimSpace(canonicalModel), channelID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	allTargets := make([]model.RouteTarget, 0)
	for _, row := range rows {
		allTargets = append(allTargets, row.Targets...)
	}
	channelNames, err := routingPolicyChannelNames(allTargets)
	if err != nil {
		return nil, 0, err
	}
	views := make([]RoutingPolicyView, 0, len(rows))
	for index := range rows {
		view, err := routingPolicyViewFromRow(&rows[index], channelNames)
		if err != nil {
			return nil, 0, err
		}
		views = append(views, *view)
	}
	return views, total, nil
}

func SetRoutingPolicyStatus(id int, enabled bool) (*RoutingPolicyView, error) {
	view, err := GetRoutingPolicyView(id)
	if err != nil {
		return nil, err
	}
	request := routingPolicyWriteRequestFromView(view)
	request.Enabled = enabled
	return SaveRoutingPolicy(id, request)
}

func RemoveRoutingPolicy(id int) error {
	row, err := model.GetRoutingPolicy(id)
	if err != nil {
		return err
	}
	if err := model.DeleteRoutingPolicy(id); err != nil {
		return err
	}
	if err := model.RefreshRoutingPolicyCache(row.GroupName, row.Model); err != nil {
		return newRoutingPolicyServiceError("routing_policy_error", "", err.Error())
	}
	model.InitChannelCache()
	return nil
}

func normalizeRoutingPolicyWriteRequest(request *RoutingPolicyWriteRequest) {
	request.GroupName = strings.TrimSpace(request.GroupName)
	request.Model = strings.TrimSpace(request.Model)
	request.Defaults.OutputResolution = strings.ToLower(strings.TrimSpace(request.Defaults.OutputResolution))
	request.Defaults.AspectRatio = strings.ToLower(strings.TrimSpace(request.Defaults.AspectRatio))
	for index := range request.Targets {
		target := &request.Targets[index]
		target.Name = strings.TrimSpace(target.Name)
		target.UpstreamModel = strings.TrimSpace(target.UpstreamModel)
		target.Constraints.OutputResolutions = normalizedStrings(target.Constraints.OutputResolutions)
		target.Constraints.AspectRatios = normalizedStrings(target.Constraints.AspectRatios)
		target.Constraints.Durations.Values = normalizedInts(target.Constraints.Durations.Values)
	}
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizedInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Ints(normalized)
	return normalized
}

func mapRoutingValidationError(err error) error {
	var validationErr *modelrouting.ValidationError
	if !errors.As(err, &validationErr) {
		return newRoutingPolicyServiceError("routing_policy_error", "", err.Error())
	}
	serviceErr := &RoutingPolicyServiceError{
		Code:  string(validationErr.Code),
		Field: validationErr.Field,
		Err:   errors.New(validationErr.Message),
	}
	if validationErr.Code == modelrouting.ValidationTargetOverlap {
		serviceErr.Err = errors.New("targets overlap at the same channel priority")
		for _, targetID := range validationErr.TargetIDs {
			if targetID < 0 {
				serviceErr.TargetIndexes = append(serviceErr.TargetIndexes, -targetID-1)
			}
		}
		sort.Ints(serviceErr.TargetIndexes)
	}
	return serviceErr
}

func mapRoutingPersistenceError(err error) error {
	var validationErr *modelrouting.ValidationError
	if errors.As(err, &validationErr) {
		return mapRoutingValidationError(err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return newRoutingPolicyServiceError("routing_policy_error", "", err.Error())
}

func routingPolicyViewFromRow(row *model.RoutingPolicy, channelNames map[int]string) (*RoutingPolicyView, error) {
	view := &RoutingPolicyView{
		ID:        row.ID,
		GroupName: row.GroupName,
		Model:     row.Model,
		Enabled:   row.Enabled,
		Defaults: modelrouting.Defaults{
			OutputResolution: row.DefaultResolution,
			DurationSeconds:  row.DefaultDuration,
			AspectRatio:      row.DefaultRatio,
		},
		Targets:   make([]RouteTargetView, 0, len(row.Targets)),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	for _, target := range row.Targets {
		var constraints modelrouting.Constraints
		if err := common.UnmarshalJsonStr(target.Constraints, &constraints); err != nil {
			return nil, newRoutingPolicyServiceError("routing_policy_error", "", err.Error())
		}
		view.Targets = append(view.Targets, RouteTargetView{
			ID:             target.ID,
			ChannelID:      target.ChannelID,
			ChannelName:    channelNames[target.ChannelID],
			Name:           target.Name,
			UpstreamModel:  target.UpstreamModel,
			TargetPriority: target.TargetPriority,
			Enabled:        target.Enabled,
			Constraints:    constraints,
		})
	}
	return view, nil
}

func routingPolicyWriteRequestFromView(view *RoutingPolicyView) RoutingPolicyWriteRequest {
	request := RoutingPolicyWriteRequest{
		GroupName: view.GroupName,
		Model:     view.Model,
		Enabled:   view.Enabled,
		Defaults:  view.Defaults,
		Targets:   make([]RouteTargetWriteRequest, 0, len(view.Targets)),
	}
	for _, target := range view.Targets {
		request.Targets = append(request.Targets, RouteTargetWriteRequest{
			ChannelID:      target.ChannelID,
			Name:           target.Name,
			UpstreamModel:  target.UpstreamModel,
			TargetPriority: target.TargetPriority,
			Enabled:        target.Enabled,
			Constraints:    target.Constraints,
		})
	}
	return request
}

func routingPolicyChannelNames(targets []model.RouteTarget) (map[int]string, error) {
	channelIDs := make([]int, 0, len(targets))
	seen := make(map[int]struct{}, len(targets))
	for _, target := range targets {
		if _, ok := seen[target.ChannelID]; ok {
			continue
		}
		seen[target.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, target.ChannelID)
	}
	names := make(map[int]string, len(channelIDs))
	if len(channelIDs) == 0 {
		return names, nil
	}
	var channels []struct {
		ID   int
		Name string
	}
	if err := model.DB.Model(&model.Channel{}).Select("id, name").Where("id IN ?", channelIDs).Scan(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		names[channel.ID] = channel.Name
	}
	return names, nil
}

func newRoutingPolicyServiceError(code, field, message string) *RoutingPolicyServiceError {
	return &RoutingPolicyServiceError{Code: code, Field: field, Err: errors.New(message)}
}

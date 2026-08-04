package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DecodeConfigImportBindingRequest only accepts the credential-free binding
// contract. Channel credentials remain owned by the normal channel form.
func DecodeConfigImportBindingRequest(reader io.Reader) (*dto.ConfigImportBindingRequest, error) {
	if reader == nil {
		return nil, configImportError("SCHEMA_BINDING_REQUEST", "binding request is required")
	}
	var request dto.ConfigImportBindingRequest
	if err := common.DecodeJsonStrict(reader, &request); err != nil {
		return nil, configImportError("SCHEMA_BINDING_REQUEST", "invalid binding request: %v", err)
	}
	if len(request.Bindings) == 0 {
		return nil, configImportError("SCHEMA_BINDING_REQUEST", "bindings are required")
	}
	if err := validateConfigImportBindingInputs(request.Bindings); err != nil {
		return nil, err
	}
	return &request, nil
}

// UpdateConfigImportBindings records credential-free line decisions for a
// batch. A create decision only accepts an already-created disabled channel;
// this service never receives or writes a channel credential.
func UpdateConfigImportBindings(
	ctx context.Context,
	adminID int,
	batchID int64,
	bindings []dto.ConfigImportBindingInput,
) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateConfigImportBindingInputs(bindings); err != nil {
		return nil, err
	}

	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := tx.Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		if types.ConfigImportBatchStatus(batch.Status) != types.ConfigImportBatchStatusBinding {
			return configImportError("BINDING_BATCH_STATUS", "batch %d is not accepting bindings", batchID)
		}

		var items []model.ConfigImportItem
		if err := tx.Where("batch_id = ?", batchID).Order("id ASC").Find(&items).Error; err != nil {
			return err
		}
		catalog, err := buildConfigImportBindingCatalog(items)
		if err != nil {
			return err
		}

		for _, input := range bindings {
			line, found := catalog.lines[input.LineRef]
			if !found {
				return configImportError("BINDING_LINE_NOT_FOUND", "line_ref %q does not belong to batch %d", input.LineRef, batchID)
			}
			if input.Action == types.ConfigImportBindingActionSkip {
				var currentItems []model.ConfigImportItem
				if err := tx.Where("batch_id = ?", batchID).Order("id ASC").Find(&currentItems).Error; err != nil {
					return err
				}
				skipStateJSON, err := excludeConfigImportLineDependents(tx, batchID, currentItems, input.LineRef)
				if err != nil {
					return err
				}
				if err := saveConfigImportBindingWithSkipState(tx, batchID, input, adminID, skipStateJSON); err != nil {
					return err
				}
				continue
			}

			channel, err := configImportBindingChannel(tx, input.ChannelID)
			if err != nil {
				return err
			}
			if input.Action == types.ConfigImportBindingActionBind && configImportShouldRecoverCreatedChannel(batch, line, channel) {
				input.Action = types.ConfigImportBindingActionCreate
			}
			if input.Action == types.ConfigImportBindingActionCreate && channel.Status != common.ChannelStatusManuallyDisabled {
				return configImportError("BINDING_NEW_CHANNEL_STATUS", "new channel %d must be manually disabled", channel.Id)
			}
			if err := rejectConfigImportChannelCapabilityConflict(tx, batchID, catalog, line, channel); err != nil {
				return err
			}
			if err := validateConfigImportBindingChannel(catalog, line, channel); err != nil {
				return err
			}
			if err := reconcileConfigImportLineDependents(tx, batchID, input.LineRef); err != nil {
				return err
			}
			if err := saveConfigImportBinding(tx, batchID, input, adminID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetConfigImportBatch(ctx, batchID)
}

func configImportShouldRecoverCreatedChannel(batch model.ConfigImportBatch, line types.ConfigImportChannelLine, channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusManuallyDisabled {
		return false
	}
	providerHint := strings.TrimSpace(line.ProviderTypeHint)
	channelName := strings.TrimSpace(channel.Name)
	return batch.CreatedAt > 0 && channel.CreatedTime >= batch.CreatedAt && providerHint != "" &&
		channelName != "" && strings.EqualFold(channelName, providerHint)
}

func validateConfigImportBindingInputs(bindings []dto.ConfigImportBindingInput) error {
	if len(bindings) == 0 {
		return configImportError("SCHEMA_BINDING_REQUEST", "bindings are required")
	}
	seenLines := make(map[string]struct{}, len(bindings))
	for index := range bindings {
		input := &bindings[index]
		input.LineRef = strings.TrimSpace(input.LineRef)
		if input.LineRef == "" {
			return configImportError("SCHEMA_BINDING_LINE", "bindings[%d].line_ref is required", index)
		}
		if _, exists := seenLines[input.LineRef]; exists {
			return configImportError("SCHEMA_BINDING_LINE", "bindings[%d].line_ref %q is duplicated", index, input.LineRef)
		}
		seenLines[input.LineRef] = struct{}{}
		switch input.Action {
		case types.ConfigImportBindingActionBind, types.ConfigImportBindingActionCreate:
			if input.ChannelID == nil || *input.ChannelID <= 0 {
				return configImportError("SCHEMA_BINDING_CHANNEL", "bindings[%d].channel_id must be positive", index)
			}
		case types.ConfigImportBindingActionSkip:
			if input.ChannelID != nil {
				return configImportError("SCHEMA_BINDING_CHANNEL", "bindings[%d].channel_id is not valid for skip", index)
			}
			if input.CredentialsConfirmed {
				return configImportError("SCHEMA_BINDING_CREDENTIAL_CONFIRMATION", "bindings[%d].credentials_confirmed is not valid for skip", index)
			}
		default:
			return configImportError("SCHEMA_BINDING_ACTION", "bindings[%d].action is invalid", index)
		}
	}
	return nil
}

type configImportBindingCatalog struct {
	channels map[string]types.ConfigImportChannel
	lines    map[string]types.ConfigImportChannelLine
	models   map[string][]string
}

type configImportSkippedItemState struct {
	ID              int64  `json:"id"`
	State           string `json:"state"`
	ExclusionReason string `json:"exclusion_reason"`
}

type configImportSkipStateSnapshot struct {
	Items []configImportSkippedItemState `json:"items"`
}

func buildConfigImportBindingCatalog(items []model.ConfigImportItem) (*configImportBindingCatalog, error) {
	catalog := &configImportBindingCatalog{
		channels: make(map[string]types.ConfigImportChannel),
		lines:    make(map[string]types.ConfigImportChannelLine),
		models:   make(map[string][]string),
	}
	for index := range items {
		item := items[index]
		switch item.EntityType {
		case "channels":
			var channel types.ConfigImportChannel
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &channel); err != nil {
				return nil, fmt.Errorf("decode config import channel %q: %w", item.BusinessID, err)
			}
			catalog.channels[channel.BusinessID] = channel
		case "channel_lines":
			var line types.ConfigImportChannelLine
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &line); err != nil {
				return nil, fmt.Errorf("decode config import line %q: %w", item.BusinessID, err)
			}
			catalog.lines[line.LineRef] = line
		case "model_mappings":
			var mapping types.ConfigImportModelMapping
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
				return nil, fmt.Errorf("decode config import model mapping %q: %w", item.BusinessID, err)
			}
			if modelName := strings.TrimSpace(mapping.UpstreamModel); modelName != "" {
				catalog.models[mapping.LineRef] = append(catalog.models[mapping.LineRef], modelName)
			}
		}
	}
	return catalog, nil
}

func configImportBindingChannel(tx *gorm.DB, channelID *int) (*model.Channel, error) {
	if channelID == nil || *channelID <= 0 {
		return nil, configImportError("SCHEMA_BINDING_CHANNEL", "channel_id must be positive")
	}
	channel := &model.Channel{}
	if err := tx.Where("id = ?", *channelID).First(channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, configImportError("BINDING_CHANNEL_NOT_FOUND", "channel %d does not exist", *channelID)
		}
		return nil, err
	}
	return channel, nil
}

func validateConfigImportBindingChannel(
	catalog *configImportBindingCatalog,
	line types.ConfigImportChannelLine,
	channel *model.Channel,
) error {
	master, found := catalog.channels[line.ChannelRef]
	if !found || master.BusinessID != line.ChannelRef || master.ChannelType == nil {
		return configImportError("BINDING_BUSINESS_IDENTITY", "line_ref %q does not have a typed channel identity", line.LineRef)
	}
	expectedType := normalizedConfigImportBindingChannelType(line.ChannelRef, *master.ChannelType)
	if strings.EqualFold(strings.TrimSpace(line.Protocol), "task") && !isConfigImportTaskChannelType(channel.Type) {
		return configImportError("BINDING_CHANNEL_PROTOCOL", "channel %d type does not support task protocol for line_ref %q", channel.Id, line.LineRef)
	}
	if channel.Type != expectedType {
		return configImportError("BINDING_CHANNEL_TYPE", "channel %d type does not match line_ref %q", channel.Id, line.LineRef)
	}
	requiredModels := catalog.models[line.LineRef]
	if len(requiredModels) == 0 {
		return configImportError("BINDING_CHANNEL_MODEL", "line_ref %q has no declared upstream models", line.LineRef)
	}
	if err := validateConfigImportLineCapability(line, channel); err != nil {
		return err
	}
	return nil
}

func normalizedConfigImportBindingChannelType(channelRef string, sourceType int) int {
	if channelRef == "CH-4STOKEN" && sourceType == constant.ChannelTypeOpenAI {
		return constant.ChannelTypeFourSToken
	}
	if channelRef == "CH-8YES" && sourceType == constant.ChannelTypeOpenAI {
		return constant.ChannelTypeEightYes
	}
	return sourceType
}

func isConfigImportTaskChannelType(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeVolcEngine,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeVidu,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeSora,
		constant.ChannelTypeDimensio,
		constant.ChannelTypeNewAPIVideo,
		constant.ChannelTypeClmmMall,
		constant.ChannelTypeLucen,
		constant.ChannelTypeMegaByAI,
		constant.ChannelTypeCangyuan,
		constant.ChannelTypePaipu,
		constant.ChannelTypeSecure,
		constant.ChannelTypeOmegaAI,
		constant.ChannelTypeFourSToken,
		constant.ChannelTypeEightYes,
		constant.ChannelTypeZ5API:
		return true
	default:
		return false
	}
}

func validateConfigImportLineCapability(line types.ConfigImportChannelLine, channel *model.Channel) error {
	secureGroups := map[string]relaydto.SecureVideoGroup{
		"secure-discount":   relaydto.SecureVideoGroupDiscount,
		"secure-overseas":   relaydto.SecureVideoGroupOverseas,
		"secure-enterprise": relaydto.SecureVideoGroupEnterprise,
	}
	if expectedGroup, secureLine := secureGroups[line.LineRef]; secureLine {
		if channel.Type != constant.ChannelTypeSecure {
			return configImportError("BINDING_LINE_CAPABILITY", "line_ref %q requires a Secure channel", line.LineRef)
		}
		settings := relaydto.ChannelOtherSettings{}
		if channel.OtherSettings != "" {
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
				return configImportError("BINDING_LINE_CAPABILITY", "channel %d has invalid Secure settings", channel.Id)
			}
		}
		if settings.SecureVideoGroup != expectedGroup {
			return configImportError("BINDING_LINE_CAPABILITY", "channel %d Secure group does not match line_ref %q", channel.Id, line.LineRef)
		}
	}

	megaCapabilities := map[string]bool{
		"megabyai-fast-real-person":    true,
		"megabyai-fast-no-real-person": false,
	}
	if expectedRealPerson, megaLine := megaCapabilities[line.LineRef]; megaLine {
		if channel.Type != constant.ChannelTypeMegaByAI || line.SupportsRealPerson == nil || *line.SupportsRealPerson != expectedRealPerson {
			return configImportError("BINDING_LINE_CAPABILITY", "line_ref %q has an invalid MegaByAI real-person capability", line.LineRef)
		}
	}
	return nil
}

func rejectConfigImportChannelCapabilityConflict(
	tx *gorm.DB,
	batchID int64,
	catalog *configImportBindingCatalog,
	line types.ConfigImportChannelLine,
	channel *model.Channel,
) error {
	var existingBindings []model.ConfigImportBinding
	if err := tx.Where("batch_id = ? AND channel_id = ? AND line_ref <> ? AND action IN ?", batchID, channel.Id, line.LineRef,
		[]string{string(types.ConfigImportBindingActionBind), string(types.ConfigImportBindingActionCreate)}).
		Order("line_ref ASC").Find(&existingBindings).Error; err != nil {
		return err
	}
	for _, binding := range existingBindings {
		existingLine, found := catalog.lines[binding.LineRef]
		if !found {
			continue
		}
		if channel.Type == constant.ChannelTypeSecure && strings.HasPrefix(line.LineRef, "secure-") && strings.HasPrefix(existingLine.LineRef, "secure-") {
			return configImportError("BINDING_CHANNEL_LINE_CONFLICT", "channel %d cannot combine Secure capability lines %q and %q", channel.Id, existingLine.LineRef, line.LineRef)
		}
		if channel.Type == constant.ChannelTypeMegaByAI && line.SupportsRealPerson != nil && existingLine.SupportsRealPerson != nil &&
			*line.SupportsRealPerson != *existingLine.SupportsRealPerson {
			return configImportError("BINDING_CHANNEL_LINE_CONFLICT", "channel %d cannot combine MegaByAI real-person capability lines %q and %q", channel.Id, existingLine.LineRef, line.LineRef)
		}
	}
	return nil
}

func saveConfigImportBinding(
	tx *gorm.DB,
	batchID int64,
	input dto.ConfigImportBindingInput,
	adminID int,
) error {
	return saveConfigImportBindingWithSkipState(tx, batchID, input, adminID, "")
}

func saveConfigImportBindingWithSkipState(
	tx *gorm.DB,
	batchID int64,
	input dto.ConfigImportBindingInput,
	adminID int,
	skipStateJSON string,
) error {
	var channelID *int
	confirmedBy := 0
	var confirmedAt *int64
	if input.Action != types.ConfigImportBindingActionSkip {
		channelID = input.ChannelID
	}
	if input.CredentialsConfirmed {
		now := common.GetTimestamp()
		confirmedBy = adminID
		confirmedAt = &now
	}

	var existing model.ConfigImportBinding
	err := tx.Where("batch_id = ? AND line_ref = ?", batchID, input.LineRef).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&model.ConfigImportBinding{
			BatchID: batchID, LineRef: input.LineRef, Action: string(input.Action), ChannelID: channelID,
			CredentialsConfirmedBy: confirmedBy, CredentialsConfirmedAt: confirmedAt, SkipStateJSON: skipStateJSON,
		}).Error
	}
	if err != nil {
		return err
	}
	if input.Action == types.ConfigImportBindingActionSkip && skipStateJSON == "" {
		skipStateJSON = existing.SkipStateJSON
	}
	return tx.Model(&existing).Updates(map[string]any{
		"action":                   string(input.Action),
		"channel_id":               channelID,
		"credentials_confirmed_by": confirmedBy,
		"credentials_confirmed_at": confirmedAt,
		"skip_state_json":          skipStateJSON,
		"updated_at":               common.GetTimestamp(),
	}).Error
}

func excludeConfigImportLineDependents(
	tx *gorm.DB,
	batchID int64,
	items []model.ConfigImportItem,
	lineRef string,
) (string, error) {
	owners, err := configImportActiveSkipOwners(tx, batchID, "")
	if err != nil {
		return "", err
	}
	managedStates := configImportSkippedItemStatesByID(owners)
	mappingRefs := make(map[string]struct{})
	for _, item := range items {
		switch item.EntityType {
		case "model_mappings":
			var mapping types.ConfigImportModelMapping
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
				return "", fmt.Errorf("decode config import model mapping %q: %w", item.BusinessID, err)
			}
			if mapping.LineRef == lineRef {
				mappingRefs[item.BusinessID] = struct{}{}
			}
		}
	}

	snapshot := configImportSkipStateSnapshot{Items: make([]configImportSkippedItemState, 0)}
	managedIDs := make([]int64, 0)
	for _, item := range items {
		excluded, err := configImportItemDependsOnLine(item, lineRef, mappingRefs)
		if err != nil {
			return "", err
		}
		if !excluded {
			continue
		}
		if state, inherited := managedStates[item.ID]; inherited {
			managedIDs = append(managedIDs, item.ID)
			snapshot.Items = append(snapshot.Items, state)
			continue
		}
		if item.State != string(types.ConfigImportItemStateExcluded) {
			managedIDs = append(managedIDs, item.ID)
			snapshot.Items = append(snapshot.Items, configImportSkippedItemState{
				ID: item.ID, State: item.State, ExclusionReason: item.ExclusionReason,
			})
		}
	}
	if len(managedIDs) == 0 {
		return "", nil
	}
	if err := tx.Model(&model.ConfigImportItem{}).Where("id IN ?", managedIDs).Updates(map[string]any{
		"state":            string(types.ConfigImportItemStateExcluded),
		"exclusion_reason": "",
		"updated_at":       common.GetTimestamp(),
	}).Error; err != nil {
		return "", err
	}
	encoded, err := common.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("marshal config import skip state: %w", err)
	}
	return string(encoded), nil
}

func reconcileConfigImportLineDependents(tx *gorm.DB, batchID int64, lineRef string) error {
	var binding model.ConfigImportBinding
	err := tx.Where("batch_id = ? AND line_ref = ?", batchID, lineRef).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || binding.Action != string(types.ConfigImportBindingActionSkip) || binding.SkipStateJSON == "" {
		return nil
	}
	if err != nil {
		return err
	}
	var snapshot configImportSkipStateSnapshot
	if err := common.UnmarshalJsonStr(binding.SkipStateJSON, &snapshot); err != nil {
		return fmt.Errorf("decode config import skip state for line_ref %q: %w", lineRef, err)
	}
	owners, err := configImportActiveSkipOwners(tx, batchID, lineRef)
	if err != nil {
		return err
	}
	managedStates := configImportSkippedItemStatesByID(owners)
	for _, item := range snapshot.Items {
		if _, stillSkipped := managedStates[item.ID]; stillSkipped {
			if err := tx.Model(&model.ConfigImportItem{}).
				Where("id = ? AND batch_id = ?", item.ID, batchID).
				Updates(map[string]any{
					"state":            string(types.ConfigImportItemStateExcluded),
					"exclusion_reason": "",
					"updated_at":       common.GetTimestamp(),
				}).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Model(&model.ConfigImportItem{}).
			Where("id = ? AND batch_id = ? AND state = ? AND exclusion_reason = ?", item.ID, batchID,
				string(types.ConfigImportItemStateExcluded), "").
			Updates(map[string]any{
				"state":            item.State,
				"exclusion_reason": item.ExclusionReason,
				"updated_at":       common.GetTimestamp(),
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

type configImportSkipOwner struct {
	LineRef  string
	Snapshot configImportSkipStateSnapshot
}

func configImportActiveSkipOwners(tx *gorm.DB, batchID int64, excludedLineRef string) ([]configImportSkipOwner, error) {
	var bindings []model.ConfigImportBinding
	query := tx.Where("batch_id = ? AND action = ? AND skip_state_json <> ?", batchID,
		string(types.ConfigImportBindingActionSkip), "")
	if excludedLineRef != "" {
		query = query.Where("line_ref <> ?", excludedLineRef)
	}
	if err := query.Order("line_ref ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	owners := make([]configImportSkipOwner, 0, len(bindings))
	for _, binding := range bindings {
		var snapshot configImportSkipStateSnapshot
		if err := common.UnmarshalJsonStr(binding.SkipStateJSON, &snapshot); err != nil {
			return nil, fmt.Errorf("decode config import skip state for line_ref %q: %w", binding.LineRef, err)
		}
		owners = append(owners, configImportSkipOwner{LineRef: binding.LineRef, Snapshot: snapshot})
	}
	return owners, nil
}

func configImportSkippedItemStatesByID(owners []configImportSkipOwner) map[int64]configImportSkippedItemState {
	states := make(map[int64]configImportSkippedItemState)
	for _, owner := range owners {
		for _, item := range owner.Snapshot.Items {
			if _, found := states[item.ID]; !found {
				states[item.ID] = item
			}
		}
	}
	return states
}

func configImportItemDependsOnLine(
	item model.ConfigImportItem,
	lineRef string,
	mappingRefs map[string]struct{},
) (bool, error) {
	switch item.EntityType {
	case "channel_lines":
		var line types.ConfigImportChannelLine
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &line); err != nil {
			return false, fmt.Errorf("decode config import line %q: %w", item.BusinessID, err)
		}
		return line.LineRef == lineRef, nil
	case "cost_rule_drafts":
		var draft types.ConfigImportCostRuleDraft
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
			return false, fmt.Errorf("decode config import cost rule draft %q: %w", item.BusinessID, err)
		}
		return draft.LineRef == lineRef, nil
	case "model_mappings":
		var mapping types.ConfigImportModelMapping
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
			return false, fmt.Errorf("decode config import model mapping %q: %w", item.BusinessID, err)
		}
		return mapping.LineRef == lineRef, nil
	case "route_blueprints":
		var blueprint types.ConfigImportRouteBlueprint
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
			return false, fmt.Errorf("decode config import route blueprint %q: %w", item.BusinessID, err)
		}
		for _, mappingRef := range blueprint.ModelMappingRefs {
			if _, found := mappingRefs[mappingRef]; found {
				return true, nil
			}
		}
		for _, target := range blueprint.Targets {
			if target.LineRef == lineRef {
				return true, nil
			}
		}
		return false, nil
	case "unresolved_variants":
		var variant types.ConfigImportUnresolvedVariant
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &variant); err != nil {
			return false, fmt.Errorf("decode config import unresolved variant %q: %w", item.BusinessID, err)
		}
		return variant.LineRef == lineRef, nil
	default:
		return false, nil
	}
}

// DecodeConfigImportResolutionRequest decodes the small, credential-free
// conflict decision contract used by the import wizard.
func DecodeConfigImportResolutionRequest(reader io.Reader) (*dto.ConfigImportResolutionRequest, error) {
	if reader == nil {
		return nil, configImportError("SCHEMA_RESOLUTION_REQUEST", "resolution request is required")
	}
	var request dto.ConfigImportResolutionRequest
	if err := common.DecodeJsonStrict(reader, &request); err != nil {
		return nil, configImportError("SCHEMA_RESOLUTION_REQUEST", "invalid resolution request: %v", err)
	}
	if err := validateConfigImportResolutionInputs(request.Resolutions); err != nil {
		return nil, err
	}
	return &request, nil
}

func validateConfigImportResolutionInputs(resolutions []dto.ConfigImportResolutionInput) error {
	if len(resolutions) == 0 {
		return configImportError("SCHEMA_RESOLUTION_REQUEST", "resolutions are required")
	}
	seen := make(map[string]struct{}, len(resolutions))
	for index := range resolutions {
		resolution := &resolutions[index]
		resolution.ItemBusinessID = strings.TrimSpace(resolution.ItemBusinessID)
		resolution.LineRef = strings.TrimSpace(resolution.LineRef)
		resolution.RouteTargetRef = strings.TrimSpace(resolution.RouteTargetRef)
		resolution.Reason = strings.TrimSpace(resolution.Reason)
		if resolution.ItemBusinessID == "" {
			return configImportError("SCHEMA_RESOLUTION_ITEM", "resolutions[%d].item_business_id is required", index)
		}
		if _, exists := seen[resolution.ItemBusinessID]; exists {
			return configImportError("SCHEMA_RESOLUTION_ITEM", "resolutions[%d].item_business_id is duplicated", index)
		}
		seen[resolution.ItemBusinessID] = struct{}{}
		switch resolution.Action {
		case types.ConfigImportResolutionActionUseImport,
			types.ConfigImportResolutionActionKeepExisting:
			if resolution.LineRef != "" || resolution.CostVariantKey != "" || resolution.RouteTargetRef != "" || resolution.Reason != "" {
				return configImportError("SCHEMA_RESOLUTION_FIELDS", "resolutions[%d] has fields that are not valid for %q", index, resolution.Action)
			}
		case types.ConfigImportResolutionActionSplitLine:
			if resolution.LineRef == "" {
				return configImportError("SCHEMA_RESOLUTION_LINE", "resolutions[%d].line_ref is required for split_line", index)
			}
			if resolution.CostVariantKey != "" || resolution.RouteTargetRef != "" || resolution.Reason != "" {
				return configImportError("SCHEMA_RESOLUTION_FIELDS", "resolutions[%d] has fields that are not valid for split_line", index)
			}
		case types.ConfigImportResolutionActionBindVariant:
			canonicalVariantKey, err := types.NormalizeCostVariantKey(resolution.CostVariantKey)
			if err != nil || resolution.CostVariantKey == "" || canonicalVariantKey != resolution.CostVariantKey {
				return configImportError("SCHEMA_RESOLUTION_COST_VARIANT", "resolutions[%d].cost_variant_key is invalid", index)
			}
			if resolution.RouteTargetRef == "" {
				return configImportError("SCHEMA_RESOLUTION_ROUTE_TARGET", "resolutions[%d].route_target_ref is required for bind_variant", index)
			}
			if resolution.LineRef != "" || resolution.Reason != "" {
				return configImportError("SCHEMA_RESOLUTION_FIELDS", "resolutions[%d] has fields that are not valid for bind_variant", index)
			}
		case types.ConfigImportResolutionActionExclude:
			if resolution.Reason == "" {
				return configImportError("SCHEMA_RESOLUTION_REASON", "resolutions[%d].reason is required for exclude", index)
			}
			for _, credentialPattern := range configImportCredentialValuePatterns {
				if credentialPattern.MatchString(resolution.Reason) {
					return configImportError("SECURITY_CREDENTIAL_VALUE", "credential-like value is not allowed in resolutions[%d].reason", index)
				}
			}
			if resolution.LineRef != "" || resolution.CostVariantKey != "" || resolution.RouteTargetRef != "" {
				return configImportError("SCHEMA_RESOLUTION_FIELDS", "resolutions[%d] has fields that are not valid for exclude", index)
			}
		default:
			return configImportError("SCHEMA_RESOLUTION_ACTION", "resolutions[%d].action is invalid", index)
		}
	}
	return nil
}

func DecodeConfigImportRouteReviewRequest(reader io.Reader) (*dto.ConfigImportRouteReviewRequest, error) {
	if reader == nil {
		return nil, configImportError("SCHEMA_ROUTE_REVIEW_REQUEST", "route review request is required")
	}
	var request dto.ConfigImportRouteReviewRequest
	if err := common.DecodeJsonStrict(reader, &request); err != nil {
		return nil, configImportError("SCHEMA_ROUTE_REVIEW_REQUEST", "invalid route review request: %v", err)
	}
	if len(request.Reviews) == 0 {
		return nil, configImportError("SCHEMA_ROUTE_REVIEW_REQUEST", "route reviews are required")
	}
	seen := make(map[string]struct{}, len(request.Reviews))
	for index := range request.Reviews {
		review := &request.Reviews[index]
		review.ItemBusinessID = strings.TrimSpace(review.ItemBusinessID)
		if review.ItemBusinessID == "" {
			return nil, configImportError("SCHEMA_ROUTE_REVIEW_ITEM", "reviews[%d].item_business_id is required", index)
		}
		if _, exists := seen[review.ItemBusinessID]; exists {
			return nil, configImportError("SCHEMA_ROUTE_REVIEW_ITEM", "reviews[%d].item_business_id is duplicated", index)
		}
		seen[review.ItemBusinessID] = struct{}{}
		switch review.MergeMode {
		case types.ConfigImportRouteMergeModeMerge, types.ConfigImportRouteMergeModeReplace, types.ConfigImportRouteMergeModeSkip:
		default:
			return nil, configImportError("SCHEMA_ROUTE_REVIEW_MODE", "reviews[%d].merge_mode is invalid", index)
		}
	}
	return &request, nil
}

func DecodeConfigImportPricingReviewRequest(reader io.Reader) (*dto.ConfigImportPricingReviewRequest, error) {
	if reader == nil {
		return nil, configImportError("SCHEMA_PRICING_REVIEW_REQUEST", "pricing review request is required")
	}
	var request dto.ConfigImportPricingReviewRequest
	if err := common.DecodeJsonStrict(reader, &request); err != nil {
		return nil, configImportError("SCHEMA_PRICING_REVIEW_REQUEST", "invalid pricing review request: %v", err)
	}
	if len(request.SelectedGroups) == 0 {
		return nil, configImportError("SCHEMA_PRICING_REVIEW_GROUPS", "selected_groups are required")
	}
	seen := make(map[string]struct{}, len(request.SelectedGroups))
	for index := range request.SelectedGroups {
		group := strings.TrimSpace(request.SelectedGroups[index])
		if group == "" {
			return nil, configImportError("SCHEMA_PRICING_REVIEW_GROUP", "selected_groups[%d] is required", index)
		}
		if _, exists := seen[group]; exists {
			return nil, configImportError("SCHEMA_PRICING_REVIEW_GROUP", "selected_groups[%d] is duplicated", index)
		}
		seen[group] = struct{}{}
		request.SelectedGroups[index] = group
	}
	return &request, nil
}

func UpdateConfigImportPricingReview(
	ctx context.Context,
	adminID int,
	batchID int64,
	selectedGroups []string,
) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if len(selectedGroups) == 0 {
		return nil, configImportError("SCHEMA_PRICING_REVIEW_GROUPS", "selected_groups are required")
	}
	groups := make([]string, len(selectedGroups))
	copy(groups, selectedGroups)
	seen := make(map[string]struct{}, len(groups))
	for index := range groups {
		groups[index] = strings.TrimSpace(groups[index])
		if groups[index] == "" {
			return nil, configImportError("SCHEMA_PRICING_REVIEW_GROUP", "selected_groups[%d] is required", index)
		}
		if _, exists := seen[groups[index]]; exists {
			return nil, configImportError("SCHEMA_PRICING_REVIEW_GROUP", "selected_groups[%d] is duplicated", index)
		}
		seen[groups[index]] = struct{}{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := tx.Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		status := types.ConfigImportBatchStatus(batch.Status)
		if status != types.ConfigImportBatchStatusBinding && status != types.ConfigImportBatchStatusStaged && status != types.ConfigImportBatchStatusReady {
			return configImportError("PRICING_REVIEW_BATCH_STATUS", "batch %d is not accepting pricing reviews", batchID)
		}
		var items []model.ConfigImportItem
		if err := tx.Where("batch_id = ? AND entity_type = ?", batchID, "sale_proposals").Order("id ASC").Find(&items).Error; err != nil {
			return err
		}
		for index := range items {
			item := &items[index]
			if item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
				continue
			}
			var proposal types.ConfigImportSaleProposal
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &proposal); err != nil {
				return err
			}
			proposal.SelectedGroups = append([]string(nil), groups...)
			encoded, err := common.Marshal(proposal)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.ConfigImportItem{}).Where("id = ?", item.ID).Updates(map[string]any{
				"canonical_json": string(encoded), "state": string(types.ConfigImportItemStateChanged), "updated_at": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		}
		if status == types.ConfigImportBatchStatusReady {
			return tx.Model(&model.ConfigImportBatch{}).Where("id = ?", batchID).Updates(map[string]any{
				"status": string(types.ConfigImportBatchStatusStaged), "updated_at": common.GetTimestamp(),
			}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetConfigImportBatch(ctx, batchID)
}

func UpdateConfigImportRouteReviews(ctx context.Context, adminID int, batchID int64, reviews []dto.ConfigImportRouteReviewInput) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if len(reviews) == 0 {
		return nil, configImportError("SCHEMA_ROUTE_REVIEW_REQUEST", "route reviews are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := tx.Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		status := types.ConfigImportBatchStatus(batch.Status)
		if status != types.ConfigImportBatchStatusBinding && status != types.ConfigImportBatchStatusStaged && status != types.ConfigImportBatchStatusReady {
			return configImportError("ROUTE_REVIEW_BATCH_STATUS", "batch %d is not accepting route reviews", batchID)
		}
		for _, review := range reviews {
			var item model.ConfigImportItem
			if err := tx.Where("batch_id = ? AND business_id = ? AND entity_type = ?", batchID, review.ItemBusinessID, "route_blueprints").First(&item).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return configImportError("ROUTE_REVIEW_ITEM_NOT_FOUND", "route blueprint %q does not belong to batch %d", review.ItemBusinessID, batchID)
				}
				return err
			}
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
				return err
			}
			blueprint.MergeMode = review.MergeMode
			encoded, err := common.Marshal(blueprint)
			if err != nil {
				return err
			}
			updates := map[string]any{"canonical_json": string(encoded), "updated_at": common.GetTimestamp()}
			if review.MergeMode == types.ConfigImportRouteMergeModeSkip {
				updates["state"] = string(types.ConfigImportItemStateExcluded)
				updates["exclusion_reason"] = "route merge mode skip"
				updates["conflict_reason"] = ""
			} else {
				updates["state"] = string(types.ConfigImportItemStateChanged)
				updates["exclusion_reason"] = ""
				updates["conflict_reason"] = ""
			}
			if err := tx.Model(&model.ConfigImportItem{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		if status == types.ConfigImportBatchStatusReady {
			return tx.Model(&model.ConfigImportBatch{}).Where("id = ?", batchID).Updates(map[string]any{
				"status": string(types.ConfigImportBatchStatusStaged), "updated_at": common.GetTimestamp(),
			}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetConfigImportBatch(ctx, batchID)
}

type configImportResolutionDecision struct {
	Action         types.ConfigImportResolutionAction `json:"action"`
	LineRef        string                             `json:"line_ref,omitempty"`
	CostVariantKey string                             `json:"cost_variant_key,omitempty"`
	RouteTargetRef string                             `json:"route_target_ref,omitempty"`
	Reason         string                             `json:"reason,omitempty"`
}

func validateConfigImportResolutionReferences(tx *gorm.DB, batchID int64, item model.ConfigImportItem, resolution dto.ConfigImportResolutionInput) error {
	// Legacy use_import/keep_existing decisions apply to the original proposal
	// rows as well as unresolved variants. Structured split/bind decisions below
	// are intentionally restricted to unresolved conflicts.
	if resolution.Action == types.ConfigImportResolutionActionUseImport ||
		resolution.Action == types.ConfigImportResolutionActionKeepExisting {
		return nil
	}
	var variant types.ConfigImportUnresolvedVariant
	if err := common.UnmarshalJsonStr(item.CanonicalJSON, &variant); err != nil {
		return configImportError("RESOLUTION_ITEM_INVALID", "item %q has invalid unresolved variant data", item.BusinessID)
	}
	switch resolution.Action {
	case types.ConfigImportResolutionActionSplitLine:
		var binding model.ConfigImportBinding
		err := tx.Where("batch_id = ? AND line_ref = ?", batchID, resolution.LineRef).First(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return configImportError("RESOLUTION_LINE_UNBOUND", "line_ref %q must be bound before it can split a variant", resolution.LineRef)
		}
		if err != nil {
			return err
		}
		if binding.Action == string(types.ConfigImportBindingActionSkip) || binding.ChannelID == nil || *binding.ChannelID <= 0 || binding.CredentialsConfirmedAt == nil {
			return configImportError("RESOLUTION_LINE_UNBOUND", "line_ref %q must be bound before it can split a variant", resolution.LineRef)
		}
		if item.EntityType != "unresolved_variants" || item.State != string(types.ConfigImportItemStateConflict) {
			return configImportError("RESOLUTION_ITEM_STATE", "item %q must be an unresolved conflict", item.BusinessID)
		}
	case types.ConfigImportResolutionActionBindVariant:
		if item.EntityType != "unresolved_variants" {
			return configImportError("RESOLUTION_ITEM_STATE", "item %q must be an unresolved conflict", item.BusinessID)
		}
		var routeItems []model.ConfigImportItem
		if err := tx.Where("batch_id = ? AND entity_type = ?", batchID, "route_blueprints").Find(&routeItems).Error; err != nil {
			return err
		}
		for _, routeItem := range routeItems {
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint); err != nil {
				return fmt.Errorf("decode config import route blueprint %q: %w", routeItem.BusinessID, err)
			}
			for _, target := range blueprint.Targets {
				if target.RouteTargetRef == resolution.RouteTargetRef &&
					(variant.LineRef == "" || target.LineRef == variant.LineRef) &&
					(variant.UpstreamModel == "" || target.UpstreamModel == variant.UpstreamModel) {
					if item.State != string(types.ConfigImportItemStateConflict) {
						return configImportError("RESOLUTION_ITEM_STATE", "item %q must be an unresolved conflict", item.BusinessID)
					}
					return nil
				}
			}
		}
		return configImportError("RESOLUTION_ROUTE_TARGET_NOT_FOUND", "route_target_ref %q is not on unresolved line %q", resolution.RouteTargetRef, variant.LineRef)
	}
	if item.State != string(types.ConfigImportItemStateConflict) {
		return configImportError("RESOLUTION_ITEM_STATE", "item %q must be an unresolved conflict", item.BusinessID)
	}
	return nil
}

// UpdateConfigImportResolutions persists normalized conflict decisions. The
// decision is deliberately kept separate from the authoritative item so the
// original import remains auditable and can be staged repeatedly.
func UpdateConfigImportResolutions(
	ctx context.Context,
	adminID int,
	batchID int64,
	resolutions []dto.ConfigImportResolutionInput,
) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if err := validateConfigImportResolutionInputs(resolutions); err != nil {
		return nil, err
	}
	resolutions = append([]dto.ConfigImportResolutionInput(nil), resolutions...)
	sort.Slice(resolutions, func(left, right int) bool {
		return resolutions[left].ItemBusinessID < resolutions[right].ItemBusinessID
	})
	if ctx == nil {
		ctx = context.Background()
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := tx.Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		status := types.ConfigImportBatchStatus(batch.Status)
		if status != types.ConfigImportBatchStatusBinding && status != types.ConfigImportBatchStatusStaged {
			return configImportError("RESOLUTION_BATCH_STATUS", "batch %d is not accepting resolutions", batchID)
		}
		for _, resolution := range resolutions {
			var item model.ConfigImportItem
			if err := tx.Where("batch_id = ? AND business_id = ?", batchID, resolution.ItemBusinessID).First(&item).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return configImportError("RESOLUTION_ITEM_NOT_FOUND", "item %q does not belong to batch %d", resolution.ItemBusinessID, batchID)
				}
				return err
			}
			if err := validateConfigImportResolutionReferences(tx, batchID, item, resolution); err != nil {
				return err
			}
			decisionJSON, err := common.Marshal(configImportResolutionDecision{
				Action:         resolution.Action,
				LineRef:        resolution.LineRef,
				CostVariantKey: resolution.CostVariantKey,
				RouteTargetRef: resolution.RouteTargetRef,
				Reason:         resolution.Reason,
			})
			if err != nil {
				return err
			}
			if err := tx.Create(&model.ConfigImportResolution{
				BatchID: batchID, ItemBusinessID: resolution.ItemBusinessID, Action: string(resolution.Action),
				DecisionJSON: string(decisionJSON), CreatedBy: adminID,
			}).Error; err != nil {
				return err
			}

			updates := map[string]any{"updated_at": common.GetTimestamp()}
			switch resolution.Action {
			case types.ConfigImportResolutionActionExclude:
				updates["state"] = string(types.ConfigImportItemStateExcluded)
				updates["exclusion_reason"] = resolution.Reason
				updates["conflict_reason"] = ""
			case types.ConfigImportResolutionActionKeepExisting:
				updates["state"] = string(types.ConfigImportItemStateUnchanged)
				updates["conflict_reason"] = "kept existing configuration"
				updates["exclusion_reason"] = ""
			default:
				updates["state"] = string(types.ConfigImportItemStateChanged)
				updates["conflict_reason"] = ""
				updates["exclusion_reason"] = ""
			}
			if err := tx.Model(&model.ConfigImportItem{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
				return err
			}
			issueState := "resolved"
			if resolution.Action == types.ConfigImportResolutionActionExclude {
				issueState = "excluded"
			}
			if err := tx.Model(&model.ConfigImportIssue{}).
				Where("batch_id = ? AND business_id = ? AND severity = ?", batchID, resolution.ItemBusinessID, string(types.ConfigImportIssueSeverityWarning)).
				Updates(map[string]any{"resolution_status": issueState, "updated_at": common.GetTimestamp()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetConfigImportBatch(ctx, batchID)
}

// applyConfigImportResolutions materializes the selected conflict decisions
// into the same canonical proposal rows consumed by staging. Keeping this
// step in the staging transaction makes a decision effective on every retry,
// while the original unresolved item and the resolution audit row remain
// available for review.
func applyConfigImportResolutions(tx *gorm.DB, batchID int64, items []model.ConfigImportItem) error {
	var stored []model.ConfigImportResolution
	if err := tx.Where("batch_id = ?", batchID).Order("id ASC").Find(&stored).Error; err != nil {
		return err
	}
	latest := make(map[string]configImportResolutionDecision, len(stored))
	for _, resolution := range stored {
		var decision configImportResolutionDecision
		if err := common.UnmarshalJsonStr(resolution.DecisionJSON, &decision); err != nil {
			return fmt.Errorf("decode config import resolution %q: %w", resolution.ItemBusinessID, err)
		}
		latest[resolution.ItemBusinessID] = decision
	}
	for index := range items {
		item := &items[index]
		decision, ok := latest[item.BusinessID]
		if !ok || item.EntityType != "unresolved_variants" {
			continue
		}
		if item.State != string(types.ConfigImportItemStateConflict) && item.State != string(types.ConfigImportItemStateChanged) && item.State != string(types.ConfigImportItemStateExcluded) && item.State != string(types.ConfigImportItemStateUnchanged) {
			return configImportError("RESOLUTION_ITEM_STATE", "item %q is not a conflict", item.BusinessID)
		}
		var variant types.ConfigImportUnresolvedVariant
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &variant); err != nil {
			return err
		}
		originalLine := variant.LineRef
		switch decision.Action {
		case types.ConfigImportResolutionActionSplitLine:
			if err := applyConfigImportSplitLine(tx, items, variant, originalLine, decision.LineRef); err != nil {
				return err
			}
			variant.LineRef = decision.LineRef
			item.State = string(types.ConfigImportItemStateChanged)
			item.ConflictReason = ""
			item.ExclusionReason = ""
		case types.ConfigImportResolutionActionBindVariant:
			if err := applyConfigImportVariantBinding(tx, items, variant, decision); err != nil {
				return err
			}
			variant.CostVariantKey = decision.CostVariantKey
			item.State = string(types.ConfigImportItemStateChanged)
			item.ConflictReason = ""
			item.ExclusionReason = ""
		case types.ConfigImportResolutionActionExclude:
			item.State = string(types.ConfigImportItemStateExcluded)
			item.ExclusionReason = decision.Reason
			item.ConflictReason = ""
		case types.ConfigImportResolutionActionKeepExisting:
			item.State = string(types.ConfigImportItemStateUnchanged)
			item.ExclusionReason = ""
			item.ConflictReason = "kept existing configuration"
		case types.ConfigImportResolutionActionUseImport:
			item.State = string(types.ConfigImportItemStateChanged)
			item.ConflictReason = ""
			item.ExclusionReason = ""
		default:
			return configImportError("SCHEMA_RESOLUTION_ACTION", "resolution action %q is invalid", decision.Action)
		}
		encoded, err := common.Marshal(variant)
		if err != nil {
			return err
		}
		item.CanonicalJSON = string(encoded)
		if err := persistConfigImportItemState(tx, item); err != nil {
			return err
		}
	}
	return nil
}

func configImportRefSet(refs []string) map[string]struct{} {
	result := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			result[trimmed] = struct{}{}
		}
	}
	return result
}

func applyConfigImportSplitLine(tx *gorm.DB, items []model.ConfigImportItem, variant types.ConfigImportUnresolvedVariant, originalLine, targetLine string) error {
	if originalLine == "" || targetLine == "" || originalLine == targetLine {
		return nil
	}
	costRefs := configImportRefSet(variant.CostRuleRefs)
	routeRefs := configImportRefSet(variant.RouteTargetRefs)
	for index := range items {
		item := &items[index]
		switch item.EntityType {
		case "cost_rule_drafts":
			var draft types.ConfigImportCostRuleDraft
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
				return err
			}
			_, explicit := costRefs[item.BusinessID]
			matches := explicit || (len(costRefs) == 0 && draft.LineRef == originalLine && (variant.UpstreamModel == "" || draft.UpstreamModel == variant.UpstreamModel) && (variant.CostVariantKey == "" || draft.CostVariantKey == variant.CostVariantKey))
			if !matches {
				continue
			}
			draft.LineRef = targetLine
			encoded, err := common.Marshal(draft)
			if err != nil {
				return err
			}
			item.CanonicalJSON = string(encoded)
			if err := persistConfigImportItemState(tx, item); err != nil {
				return err
			}
		case "route_blueprints":
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
				return err
			}
			changed := false
			for targetIndex := range blueprint.Targets {
				target := &blueprint.Targets[targetIndex]
				_, explicit := routeRefs[target.RouteTargetRef]
				matches := explicit || (len(routeRefs) == 0 && target.LineRef == originalLine && (variant.UpstreamModel == "" || target.UpstreamModel == variant.UpstreamModel) && (variant.CostVariantKey == "" || target.CostVariantKey == variant.CostVariantKey))
				if matches {
					target.LineRef = targetLine
					changed = true
				}
			}
			if !changed {
				continue
			}
			encoded, err := common.Marshal(blueprint)
			if err != nil {
				return err
			}
			item.CanonicalJSON = string(encoded)
			if err := persistConfigImportItemState(tx, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyConfigImportVariantBinding(tx *gorm.DB, items []model.ConfigImportItem, variant types.ConfigImportUnresolvedVariant, decision configImportResolutionDecision) error {
	routeRefs := configImportRefSet(variant.RouteTargetRefs)
	routeRefs[decision.RouteTargetRef] = struct{}{}
	costRefs := configImportRefSet(variant.CostRuleRefs)
	for index := range items {
		item := &items[index]
		switch item.EntityType {
		case "cost_rule_drafts":
			var draft types.ConfigImportCostRuleDraft
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
				return err
			}
			_, explicit := costRefs[item.BusinessID]
			if !explicit && draft.RouteTargetRef != decision.RouteTargetRef {
				continue
			}
			draft.CostVariantKey = decision.CostVariantKey
			encoded, err := common.Marshal(draft)
			if err != nil {
				return err
			}
			item.CanonicalJSON = string(encoded)
			if err := persistConfigImportItemState(tx, item); err != nil {
				return err
			}
		case "route_blueprints":
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
				return err
			}
			changed := false
			for targetIndex := range blueprint.Targets {
				target := &blueprint.Targets[targetIndex]
				if _, ok := routeRefs[target.RouteTargetRef]; !ok {
					continue
				}
				target.CostVariantKey = decision.CostVariantKey
				changed = true
			}
			if !changed {
				continue
			}
			encoded, err := common.Marshal(blueprint)
			if err != nil {
				return err
			}
			item.CanonicalJSON = string(encoded)
			if err := persistConfigImportItemState(tx, item); err != nil {
				return err
			}
		}
	}
	return nil
}

// ConfigImportBaseline is a deterministic optimistic-concurrency snapshot.
// The hash covers only active configuration that this batch will publish.
type ConfigImportBaseline struct {
	Hash            string            `json:"hash"`
	Channels        map[string]string `json:"channels"`
	CostRules       map[string]string `json:"cost_rules"`
	Options         map[string]string `json:"options"`
	ModelMappings   map[string]string `json:"model_mappings"`
	RoutingPolicies map[string]string `json:"routing_policies"`
}

type configImportBaselineScope struct {
	costRules       map[string]struct{}
	optionFields    map[string]map[string]struct{}
	modelMappings   map[int]map[string]struct{}
	routingPolicies map[string]struct{}
}

type configImportBaselineFieldValue struct {
	Exists bool `json:"exists"`
	Value  any  `json:"value,omitempty"`
}

func configImportBaselineScopeForBatch(db *gorm.DB, batchID int64) (*configImportBaselineScope, error) {
	scope := &configImportBaselineScope{
		costRules:       make(map[string]struct{}),
		optionFields:    make(map[string]map[string]struct{}),
		modelMappings:   make(map[int]map[string]struct{}),
		routingPolicies: make(map[string]struct{}),
	}
	var items []model.ConfigImportItem
	if err := db.Where("batch_id = ?", batchID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	lineChannels, err := configImportPublishedLineChannels(db, items)
	if err != nil {
		return nil, err
	}
	for _, channelID := range lineChannels {
		if channelID > 0 && scope.modelMappings[channelID] == nil {
			scope.modelMappings[channelID] = make(map[string]struct{})
		}
	}
	for _, item := range items {
		disabledCost, err := configImportDisabledCostRuleRetiresActive(item)
		if err != nil {
			return nil, err
		}
		if item.State == string(types.ConfigImportItemStateExcluded) && !disabledCost ||
			item.State == string(types.ConfigImportItemStateUnchanged) && item.EntityType != "model_mappings" && item.EntityType != "sale_proposals" {
			continue
		}
		switch item.EntityType {
		case "cost_rule_drafts":
			var draft types.ConfigImportCostRuleDraft
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
				return nil, err
			}
			if channelID := lineChannels[draft.LineRef]; channelID > 0 {
				scope.costRules[fmt.Sprintf("%d|%s|%s", channelID, draft.UpstreamModel, draft.CostVariantKey)] = struct{}{}
			}
		case "sale_proposals":
			var stored struct {
				StagedProposal struct {
					OptionPatches map[string]map[string]any `json:"option_patches"`
				} `json:"staged_proposal"`
			}
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &stored); err != nil {
				return nil, err
			}
			for optionKey, modelValues := range stored.StagedProposal.OptionPatches {
				if scope.optionFields[optionKey] == nil {
					scope.optionFields[optionKey] = make(map[string]struct{})
				}
				for modelName := range modelValues {
					scope.optionFields[optionKey][modelName] = struct{}{}
				}
			}
		case "model_mappings":
			var mapping types.ConfigImportModelMapping
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
				return nil, err
			}
			if channelID := lineChannels[mapping.LineRef]; channelID > 0 {
				if scope.modelMappings[channelID] == nil {
					scope.modelMappings[channelID] = make(map[string]struct{})
				}
				scope.modelMappings[channelID][configImportRuntimeCanonicalModel(mapping.CanonicalModel)] = struct{}{}
			}
		case "route_blueprints":
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
				return nil, err
			}
			if blueprint.MergeMode != types.ConfigImportRouteMergeModeSkip {
				scope.routingPolicies[fmt.Sprintf("default|%s", configImportRuntimeCanonicalModel(blueprint.CanonicalModel))] = struct{}{}
			}
		}
	}
	seedanceOfficialSale, seedanceSaleModels, err := configImportSeedanceSaleCleanupScope(db, items)
	if err != nil {
		return nil, err
	}
	if seedanceOfficialSale {
		for _, optionKey := range configImportSeedanceSaleOptionKeys {
			if scope.optionFields[optionKey] == nil {
				scope.optionFields[optionKey] = make(map[string]struct{})
			}
			for modelName := range seedanceSaleModels {
				scope.optionFields[optionKey][modelName] = struct{}{}
			}
		}
		if db.Migrator().HasTable(&model.Option{}) {
			var options []model.Option
			optionValues := make([]any, 0, len(configImportSeedanceSaleOptionKeys))
			for _, optionKey := range configImportSeedanceSaleOptionKeys {
				optionValues = append(optionValues, optionKey)
			}
			if err := db.Where(clause.IN{Column: clause.Column{Name: "key"}, Values: optionValues}).Find(&options).Error; err != nil {
				return nil, err
			}
			for _, option := range options {
				current := make(map[string]any)
				if strings.TrimSpace(option.Value) != "" {
					if err := common.UnmarshalJsonStr(option.Value, &current); err != nil {
						return nil, configImportError("BASELINE_PRICING_OPTION", "option %q is not a JSON object", option.Key)
					}
				}
				for modelName := range current {
					if seedancepricing.Family(modelName) != "" {
						scope.optionFields[option.Key][modelName] = struct{}{}
					}
				}
			}
		}
	}
	return scope, nil
}

func configImportDisabledCostRuleDraft(item model.ConfigImportItem) (bool, error) {
	if item.EntityType != "cost_rule_drafts" {
		return false, nil
	}
	var draft types.ConfigImportCostRuleDraft
	if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
		return false, err
	}
	return draft.Enabled != nil && !*draft.Enabled, nil
}

func configImportDisabledCostRuleRetiresActive(item model.ConfigImportItem) (bool, error) {
	disabled, err := configImportDisabledCostRuleDraft(item)
	if err != nil {
		return false, err
	}
	return disabled &&
		item.State == string(types.ConfigImportItemStateExcluded) &&
		item.ExclusionReason == "disabled by import document", nil
}

func captureConfigImportBaseline(db *gorm.DB, batchID int64) (*ConfigImportBaseline, error) {
	if db == nil {
		return nil, errors.New("config import baseline database is required")
	}
	scope, err := configImportBaselineScopeForBatch(db, batchID)
	if err != nil {
		return nil, err
	}
	baseline := &ConfigImportBaseline{
		Channels:        map[string]string{},
		CostRules:       map[string]string{},
		Options:         map[string]string{},
		ModelMappings:   map[string]string{},
		RoutingPolicies: map[string]string{},
	}
	if db.Migrator().HasTable(&model.ChannelModelCostRule{}) {
		var rules []model.ChannelModelCostRule
		if err := db.Where("status = ?", types.CostRuleActive).
			Order("channel_id ASC, billable_upstream_model ASC, cost_variant_key ASC, version ASC, id ASC").Find(&rules).Error; err != nil {
			return nil, err
		}
		for _, rule := range rules {
			businessKey := fmt.Sprintf("%d|%s|%s", rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey)
			if _, included := scope.costRules[businessKey]; !included {
				continue
			}
			key := fmt.Sprintf("%s|%d", businessKey, rule.Version)
			encoded, err := common.Marshal(rule)
			if err != nil {
				return nil, err
			}
			baseline.CostRules[key] = string(encoded)
		}
	}
	if db.Migrator().HasTable(&model.Option{}) && len(scope.optionFields) > 0 {
		var options []model.Option
		optionKeys := make([]string, 0, len(scope.optionFields))
		for key := range scope.optionFields {
			optionKeys = append(optionKeys, key)
		}
		sort.Strings(optionKeys)
		optionValues := make([]interface{}, len(optionKeys))
		for index, optionKey := range optionKeys {
			optionValues[index] = optionKey
		}
		if err := db.Where(clause.IN{
			Column: clause.Column{Name: "key"},
			Values: optionValues,
		}).Order(clause.OrderByColumn{
			Column: clause.Column{Name: "key"},
		}).Find(&options).Error; err != nil {
			return nil, err
		}
		byKey := make(map[string]model.Option, len(options))
		for _, option := range options {
			byKey[option.Key] = option
		}
		for _, optionKey := range optionKeys {
			values := make(map[string]any)
			if option, found := byKey[optionKey]; found && strings.TrimSpace(option.Value) != "" {
				if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
					return nil, configImportError("BASELINE_OPTION_JSON", "option %q is not a JSON object", optionKey)
				}
			}
			modelNames := make([]string, 0, len(scope.optionFields[optionKey]))
			for modelName := range scope.optionFields[optionKey] {
				modelNames = append(modelNames, modelName)
			}
			sort.Strings(modelNames)
			for _, modelName := range modelNames {
				value, exists := values[modelName]
				encoded, err := common.Marshal(configImportBaselineFieldValue{Exists: exists, Value: value})
				if err != nil {
					return nil, err
				}
				baseline.Options[fmt.Sprintf("%s|%s", optionKey, modelName)] = string(encoded)
			}
		}
	}
	if db.Migrator().HasTable(&model.Channel{}) && len(scope.modelMappings) > 0 {
		channelIDs := make([]int, 0, len(scope.modelMappings))
		for channelID := range scope.modelMappings {
			channelIDs = append(channelIDs, channelID)
		}
		sort.Ints(channelIDs)
		var channels []model.Channel
		if err := model.LockChannelsForUpdate(db).Where("id IN ?", channelIDs).Order("id ASC").Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			mapping := make(map[string]string)
			if raw := strings.TrimSpace(channel.GetModelMapping()); raw != "" {
				if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
					return nil, configImportError("BASELINE_MODEL_MAPPING_JSON", "channel %d has invalid model mapping", channel.Id)
				}
			}
			models := make([]string, 0)
			seenModels := make(map[string]struct{})
			for _, modelName := range channel.GetModels() {
				modelName = strings.TrimSpace(modelName)
				if modelName == "" {
					continue
				}
				if _, exists := seenModels[modelName]; exists {
					continue
				}
				seenModels[modelName] = struct{}{}
				models = append(models, modelName)
			}
			sort.Strings(models)
			encoded, err := common.Marshal(struct {
				Models       []string          `json:"models"`
				ModelMapping map[string]string `json:"model_mapping"`
			}{Models: models, ModelMapping: mapping})
			if err != nil {
				return nil, err
			}
			baseline.Channels[fmt.Sprintf("%d", channel.Id)] = string(encoded)
			modelNames := make([]string, 0, len(scope.modelMappings[channel.Id]))
			for modelName := range scope.modelMappings[channel.Id] {
				modelNames = append(modelNames, modelName)
			}
			sort.Strings(modelNames)
			for _, modelName := range modelNames {
				value, exists := mapping[modelName]
				encoded, err := common.Marshal(configImportBaselineFieldValue{Exists: exists, Value: value})
				if err != nil {
					return nil, err
				}
				baseline.ModelMappings[fmt.Sprintf("%d|%s", channel.Id, modelName)] = string(encoded)
			}
		}
	}
	if db.Migrator().HasTable(&model.RoutingPolicy{}) && len(scope.routingPolicies) > 0 {
		models := make([]string, 0, len(scope.routingPolicies))
		for key := range scope.routingPolicies {
			_, modelName, found := strings.Cut(key, "|")
			if found {
				models = append(models, modelName)
			}
		}
		sort.Strings(models)
		var policies []model.RoutingPolicy
		if err := db.Where("group_name = ? AND model IN ?", "default", models).Order("group_name ASC, model ASC, id ASC").Find(&policies).Error; err != nil {
			return nil, err
		}
		if db.Migrator().HasTable(&model.RouteTarget{}) && len(policies) > 0 {
			policyIDs := make([]int, 0, len(policies))
			for _, policy := range policies {
				policyIDs = append(policyIDs, policy.ID)
			}
			var targets []model.RouteTarget
			if err := db.Where("policy_id IN ?", policyIDs).Order("policy_id ASC, target_priority ASC, id ASC").Find(&targets).Error; err != nil {
				return nil, err
			}
			byPolicy := make(map[int][]model.RouteTarget, len(policies))
			for _, target := range targets {
				byPolicy[target.PolicyID] = append(byPolicy[target.PolicyID], target)
			}
			for index := range policies {
				policies[index].Targets = byPolicy[policies[index].ID]
			}
		}
		for _, policy := range policies {
			key := fmt.Sprintf("%s|%s", policy.GroupName, policy.Model)
			if _, included := scope.routingPolicies[key]; !included {
				continue
			}
			encoded, err := common.Marshal(policy)
			if err != nil {
				return nil, err
			}
			baseline.RoutingPolicies[key] = string(encoded)
		}
	}
	encoded, err := common.Marshal(struct {
		Channels        map[string]string `json:"channels"`
		CostRules       map[string]string `json:"cost_rules"`
		Options         map[string]string `json:"options"`
		ModelMappings   map[string]string `json:"model_mappings"`
		RoutingPolicies map[string]string `json:"routing_policies"`
	}{baseline.Channels, baseline.CostRules, baseline.Options, baseline.ModelMappings, baseline.RoutingPolicies})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	baseline.Hash = fmt.Sprintf("%x", digest)
	return baseline, nil
}

// CaptureConfigImportBaseline is the exported form used by publish-time
// stale checks. It intentionally returns the same deterministic snapshot that
// staging persisted on the batch.
func CaptureConfigImportBaseline(db *gorm.DB, batchID int64) (*ConfigImportBaseline, error) {
	return captureConfigImportBaseline(db, batchID)
}

type configImportStageIssue struct {
	Code       string
	Severity   types.ConfigImportIssueSeverity
	Message    string
	BusinessID string
}

// StageConfigImportBatch materializes only inactive cost drafts. Sale,
// mapping, and routing entities remain canonical proposals in import items
// until the publish transaction reviews and applies them.
func StageConfigImportBatch(ctx context.Context, adminID int, batchID int64) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := tx.Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		if status := types.ConfigImportBatchStatus(batch.Status); status != types.ConfigImportBatchStatusBinding && status != types.ConfigImportBatchStatusStaged && status != types.ConfigImportBatchStatusPublishFailed {
			return configImportError("STAGE_BATCH_STATUS", "batch %d is not ready for staging", batchID)
		}

		var items []model.ConfigImportItem
		if err := tx.Where("batch_id = ?", batchID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
			return err
		}
		if err := applyConfigImportResolutions(tx, batchID, items); err != nil {
			return err
		}
		var bindings []model.ConfigImportBinding
		if tx.Migrator().HasTable(&model.ConfigImportBinding{}) {
			if err := tx.Where("batch_id = ?", batchID).Order("line_ref ASC").Find(&bindings).Error; err != nil {
				return err
			}
		}
		issues := make([]configImportStageIssue, 0)
		lineChannels := make(map[string]int, len(bindings))
		skippedLines := make(map[string]bool, len(bindings))
		unconfirmedLines := make(map[string]bool, len(bindings))
		hasUnconfirmedBinding := false
		for _, binding := range bindings {
			if binding.Action == string(types.ConfigImportBindingActionSkip) {
				skippedLines[binding.LineRef] = true
			} else if binding.ChannelID != nil && (binding.CredentialsConfirmedAt == nil || binding.CredentialsConfirmedBy <= 0) {
				message := fmt.Sprintf("line %q requires credential confirmation before staging", binding.LineRef)
				issues = append(issues, configImportStageIssue{Code: "BINDING_CREDENTIALS_UNCONFIRMED", Severity: types.ConfigImportIssueSeverityError, BusinessID: binding.LineRef, Message: message})
				unconfirmedLines[binding.LineRef] = true
				hasUnconfirmedBinding = true
				if err := tx.Model(&model.ConfigImportIssue{}).
					Where("batch_id = ? AND code = ? AND business_id = ?", batchID, "BINDING_CREDENTIALS_UNCONFIRMED", binding.LineRef).
					Updates(map[string]any{"severity": string(types.ConfigImportIssueSeverityError), "message": message, "resolution_status": "open", "updated_at": common.GetTimestamp()}).Error; err != nil {
					return err
				}
				continue
			} else if binding.ChannelID != nil {
				lineChannels[binding.LineRef] = *binding.ChannelID
			}
			if err := tx.Model(&model.ConfigImportIssue{}).
				Where("batch_id = ? AND code = ? AND business_id = ? AND resolution_status = ?", batchID, "BINDING_CREDENTIALS_UNCONFIRMED", binding.LineRef, "open").
				Updates(map[string]any{"resolution_status": "resolved", "updated_at": common.GetTimestamp()}).Error; err != nil {
				return err
			}
		}

		for _, item := range items {
			if item.EntityType == "channel_lines" && item.State != string(types.ConfigImportItemStateExcluded) {
				var line types.ConfigImportChannelLine
				if err := common.UnmarshalJsonStr(item.CanonicalJSON, &line); err != nil {
					return err
				}
				if !skippedLines[line.LineRef] && !unconfirmedLines[line.LineRef] && lineChannels[line.LineRef] <= 0 {
					issues = append(issues, configImportStageIssue{Code: "CHANNEL_LINE_UNBOUND", Severity: types.ConfigImportIssueSeverityWarning, BusinessID: item.BusinessID, Message: fmt.Sprintf("line %q is not bound", line.LineRef)})
				}
			}
		}
		generatedIssues, err := stageConfigImportUnresolvedVariants(tx, items)
		if err != nil {
			return err
		}
		issues = append(issues, generatedIssues...)
		if err := stageConfigImportDisabledCostRules(tx, items); err != nil {
			return err
		}
		generatedIssues, err = stageConfigImportCostRules(tx, items, lineChannels, unconfirmedLines, adminID)
		if err != nil {
			return err
		}
		issues = append(issues, generatedIssues...)
		if err := tx.Model(&model.ConfigImportIssue{}).
			Where("batch_id = ? AND code = ? AND resolution_status = ?", batchID, "PRICING_GROUP_SCOPE_UNREPRESENTABLE", "open").
			Updates(map[string]any{"resolution_status": "resolved", "updated_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
		generatedIssues, err = stageConfigImportProposals(tx, items)
		if err != nil {
			return err
		}
		issues = append(issues, generatedIssues...)
		if err := persistConfigImportStageIssues(tx, batchID, issues); err != nil {
			return err
		}
		baseline, err := captureConfigImportBaseline(tx, batchID)
		if err != nil {
			return err
		}
		baselineJSON, err := common.Marshal(baseline)
		if err != nil {
			return err
		}

		var allIssues []model.ConfigImportIssue
		if err := tx.Where("batch_id = ?", batchID).Find(&allIssues).Error; err != nil {
			return err
		}
		ready := true
		for _, issue := range allIssues {
			if issue.ResolutionStatus == "resolved" || issue.ResolutionStatus == "excluded" {
				continue
			}
			if issue.Severity == string(types.ConfigImportIssueSeverityError) || issue.Severity == string(types.ConfigImportIssueSeverityWarning) {
				ready = false
				break
			}
		}
		for _, item := range items {
			if item.State == string(types.ConfigImportItemStateConflict) {
				ready = false
			}
		}
		nextStatus := types.ConfigImportBatchStatusStaged
		if hasUnconfirmedBinding {
			nextStatus = types.ConfigImportBatchStatusBinding
		} else if ready {
			nextStatus = types.ConfigImportBatchStatusReady
		}
		channelModelSnapshots, err := configImportChannelModelSnapshotDiffs(tx, items)
		if err != nil {
			return err
		}
		var storedSummary configImportBatchSummaryStorage
		if err := common.UnmarshalJsonStr(string(batch.SummaryJSON), &storedSummary); err != nil {
			return err
		}
		storedSummary.ChannelModelSnapshots = channelModelSnapshots
		summaryJSON, err := common.Marshal(storedSummary)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"status": string(nextStatus), "baseline_json": string(baselineJSON), "summary_json": string(summaryJSON), "updated_at": common.GetTimestamp(),
		}
		if types.ConfigImportBatchStatus(batch.Status) == types.ConfigImportBatchStatusPublishFailed {
			updates["failure_code"] = ""
			updates["failure_message"] = ""
			updates["failed_at"] = nil
		}
		return tx.Model(&model.ConfigImportBatch{}).Where("id = ?", batchID).Updates(updates).Error
	}); err != nil {
		return nil, err
	}
	return GetConfigImportBatch(ctx, batchID)
}

func stageConfigImportDisabledCostRules(db *gorm.DB, items []model.ConfigImportItem) error {
	for index := range items {
		item := &items[index]
		if item.EntityType != "cost_rule_drafts" || item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		var draft types.ConfigImportCostRuleDraft
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
			return err
		}
		if draft.Enabled == nil || *draft.Enabled {
			continue
		}
		item.State = string(types.ConfigImportItemStateExcluded)
		item.ExclusionReason = "disabled by import document"
		if err := persistConfigImportItemState(db, item); err != nil {
			return err
		}
	}
	return nil
}

type configImportChannelModelSnapshotTarget struct {
	Models            map[string]struct{}
	Mapping           map[string]string
	AmbiguousMappings map[string]struct{}
	LineRefs          map[string]struct{}
}

func configImportChannelModelSnapshotTargets(db *gorm.DB, items []model.ConfigImportItem) (map[int]configImportChannelModelSnapshotTarget, error) {
	lineChannels, err := configImportPublishedLineChannels(db, items)
	if err != nil {
		return nil, err
	}
	targetsByChannel := make(map[int]configImportChannelModelSnapshotTarget)
	for lineRef, channelID := range lineChannels {
		if channelID <= 0 {
			continue
		}
		target, exists := targetsByChannel[channelID]
		if !exists {
			target = configImportChannelModelSnapshotTarget{
				Models: make(map[string]struct{}), Mapping: make(map[string]string), AmbiguousMappings: make(map[string]struct{}), LineRefs: make(map[string]struct{}),
			}
		}
		target.LineRefs[lineRef] = struct{}{}
		targetsByChannel[channelID] = target
	}
	for _, item := range items {
		if item.EntityType != "model_mappings" || item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		var mapping types.ConfigImportModelMapping
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
			return nil, err
		}
		channelID := lineChannels[mapping.LineRef]
		if channelID <= 0 {
			continue
		}
		target, exists := targetsByChannel[channelID]
		if !exists {
			target = configImportChannelModelSnapshotTarget{
				Models: make(map[string]struct{}), Mapping: make(map[string]string), AmbiguousMappings: make(map[string]struct{}), LineRefs: make(map[string]struct{}),
			}
		}
		canonicalModel := configImportRuntimeCanonicalModel(mapping.CanonicalModel)
		upstreamModel := strings.TrimSpace(mapping.UpstreamModel)
		if canonicalModel == "" || upstreamModel == "" {
			continue
		}
		if _, ambiguous := target.AmbiguousMappings[canonicalModel]; !ambiguous {
			if existing, found := target.Mapping[canonicalModel]; found && existing != upstreamModel {
				delete(target.Mapping, canonicalModel)
				target.AmbiguousMappings[canonicalModel] = struct{}{}
			} else {
				target.Mapping[canonicalModel] = upstreamModel
			}
		}
		target.Models[canonicalModel] = struct{}{}
		target.Models[upstreamModel] = struct{}{}
		target.LineRefs[mapping.LineRef] = struct{}{}
		targetsByChannel[channelID] = target
	}
	return targetsByChannel, nil
}

func configImportCurrentChannelModels(channel *model.Channel) (map[string]struct{}, error) {
	currentModels := make(map[string]struct{})
	for _, modelName := range channel.GetModels() {
		if modelName = strings.TrimSpace(modelName); modelName != "" {
			currentModels[modelName] = struct{}{}
		}
	}
	if raw := strings.TrimSpace(channel.GetModelMapping()); raw != "" {
		currentMapping := make(map[string]string)
		if err := common.UnmarshalJsonStr(raw, &currentMapping); err != nil {
			return nil, configImportError("MODEL_SNAPSHOT_MAPPING_JSON", "channel %d has invalid model mapping", channel.Id)
		}
		for canonicalModel, upstreamModel := range currentMapping {
			if canonicalModel = strings.TrimSpace(canonicalModel); canonicalModel != "" {
				currentModels[canonicalModel] = struct{}{}
			}
			if upstreamModel = strings.TrimSpace(upstreamModel); upstreamModel != "" {
				currentModels[upstreamModel] = struct{}{}
			}
		}
	}
	return currentModels, nil
}

func configImportChannelModelSnapshotDiffs(db *gorm.DB, items []model.ConfigImportItem) ([]types.ConfigImportChannelModelSnapshotDiff, error) {
	targetsByChannel, err := configImportChannelModelSnapshotTargets(db, items)
	if err != nil {
		return nil, err
	}
	channelIDs := make([]int, 0, len(targetsByChannel))
	for channelID := range targetsByChannel {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)
	diffs := make([]types.ConfigImportChannelModelSnapshotDiff, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		var channel model.Channel
		if err := db.Where("id = ?", channelID).First(&channel).Error; err != nil {
			return nil, err
		}
		currentModels, err := configImportCurrentChannelModels(&channel)
		if err != nil {
			return nil, err
		}
		target := targetsByChannel[channelID]
		diff := types.ConfigImportChannelModelSnapshotDiff{
			ChannelID: channelID, ChannelName: channel.Name,
			LineRefs: []string{}, AddedModels: []string{}, RetainedModels: []string{}, RemovedModels: []string{},
		}
		for lineRef := range target.LineRefs {
			diff.LineRefs = append(diff.LineRefs, lineRef)
		}
		for modelName := range target.Models {
			if _, exists := currentModels[modelName]; exists {
				diff.RetainedModels = append(diff.RetainedModels, modelName)
			} else {
				diff.AddedModels = append(diff.AddedModels, modelName)
			}
		}
		for modelName := range currentModels {
			if _, exists := target.Models[modelName]; !exists {
				diff.RemovedModels = append(diff.RemovedModels, modelName)
			}
		}
		sort.Strings(diff.LineRefs)
		sort.Strings(diff.AddedModels)
		sort.Strings(diff.RetainedModels)
		sort.Strings(diff.RemovedModels)
		diffs = append(diffs, diff)
	}
	return diffs, nil
}

// StageConfigImport is retained as a concise alias for API/controller code.
func StageConfigImport(ctx context.Context, adminID int, batchID int64) (*dto.ConfigImportBatchDetail, error) {
	return StageConfigImportBatch(ctx, adminID, batchID)
}

func stageConfigImportUnresolvedVariants(db *gorm.DB, items []model.ConfigImportItem) ([]configImportStageIssue, error) {
	issues := make([]configImportStageIssue, 0)
	for index := range items {
		item := &items[index]
		if item.EntityType != "unresolved_variants" || item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateChanged) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		var variant types.ConfigImportUnresolvedVariant
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &variant); err != nil {
			return issues, err
		}
		if variant.Excluded != nil && *variant.Excluded {
			item.State = string(types.ConfigImportItemStateExcluded)
			item.ExclusionReason = "excluded by import document"
			if err := persistConfigImportItemState(db, item); err != nil {
				return issues, err
			}
			continue
		}
		item.State = string(types.ConfigImportItemStateConflict)
		item.ConflictReason = "cost variant requires a structured resolution"
		issues = append(issues, configImportStageIssue{Code: "COST_VARIANT_AMBIGUOUS", Severity: types.ConfigImportIssueSeverityWarning, BusinessID: item.BusinessID, Message: item.ConflictReason})
		if err := persistConfigImportItemState(db, item); err != nil {
			return issues, err
		}
	}
	return issues, nil
}

func stageConfigImportCostRules(db *gorm.DB, items []model.ConfigImportItem, lineChannels map[string]int, unconfirmedLines map[string]bool, adminID int) ([]configImportStageIssue, error) {
	issues := make([]configImportStageIssue, 0)
	sorted := make([]*model.ConfigImportItem, 0)
	for index := range items {
		if items[index].EntityType == "cost_rule_drafts" && items[index].State != string(types.ConfigImportItemStateExcluded) &&
			items[index].State != string(types.ConfigImportItemStateUnchanged) {
			sorted = append(sorted, &items[index])
		}
	}
	sort.Slice(sorted, func(left, right int) bool { return sorted[left].BusinessID < sorted[right].BusinessID })
	merged := make(map[string]struct {
		id   int
		json string
	}, len(sorted))
	for _, item := range sorted {
		if item.MaterializedID != nil && *item.MaterializedID > 0 {
			continue
		}
		var draft types.ConfigImportCostRuleDraft
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
			return issues, err
		}
		channelID := lineChannels[draft.LineRef]
		if channelID <= 0 {
			if unconfirmedLines[draft.LineRef] {
				continue
			}
			item.State = string(types.ConfigImportItemStateConflict)
			item.ConflictReason = "channel line is not bound"
			issues = append(issues, configImportStageIssue{Code: "CHANNEL_LINE_UNBOUND", Severity: types.ConfigImportIssueSeverityWarning, BusinessID: item.BusinessID, Message: item.ConflictReason})
			if err := persistConfigImportItemState(db, item); err != nil {
				return issues, err
			}
			continue
		}
		config, err := configImportCostRuleConfig(draft)
		if err != nil {
			item.State = string(types.ConfigImportItemStateConflict)
			item.ConflictReason = err.Error()
			if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
				return issues, persistErr
			}
			continue
		}
		normalized, err := NormalizeCostRuleConfig(types.CostMode(draft.CostMode), config)
		if err != nil {
			item.State = string(types.ConfigImportItemStateConflict)
			item.ConflictReason = err.Error()
			if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
				return issues, persistErr
			}
			continue
		}
		configJSON, err := common.Marshal(normalized)
		if err != nil {
			return issues, err
		}
		if configImportCostNormalizationMismatch(draft, normalized) {
			issues = append(issues, configImportStageIssue{Code: "COST_NORMALIZATION_MISMATCH", Severity: types.ConfigImportIssueSeverityWarning, BusinessID: item.BusinessID, Message: "provided normalized USD price differs from server recomputation"})
		}
		key := fmt.Sprintf("%d|%s|%s", channelID, draft.UpstreamModel, draft.CostVariantKey)
		if previous, ok := merged[key]; ok {
			if previous.json == string(configJSON) {
				item.MaterializedID = &previous.id
				item.MaterializedType = "cost_rule_draft"
				item.State = string(types.ConfigImportItemStateUnchanged)
				if err := persistConfigImportItemState(db, item); err != nil {
					return issues, err
				}
				continue
			}
			item.State = string(types.ConfigImportItemStateConflict)
			item.ConflictReason = "multiple cost contracts share one channel/model/variant key"
			issues = append(issues, configImportStageIssue{Code: "COST_VARIANT_AMBIGUOUS", Severity: types.ConfigImportIssueSeverityWarning, BusinessID: item.BusinessID, Message: item.ConflictReason})
			if err := persistConfigImportItemState(db, item); err != nil {
				return issues, err
			}
			continue
		}
		var latest model.ChannelModelCostRule
		if err := db.Where("channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ?", channelID, draft.UpstreamModel, draft.CostVariantKey).Order("version DESC").First(&latest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return issues, err
		}
		rule := &model.ChannelModelCostRule{
			ChannelID: channelID, BillableUpstreamModel: strings.TrimSpace(draft.UpstreamModel), CostVariantKey: draft.CostVariantKey,
			Version: latest.Version + 1, Status: string(types.CostRuleDraft), CostMode: draft.CostMode, SchemaVersion: 1,
			ConfigJSON: string(configJSON), Source: "config_import", Note: strings.TrimSpace(draft.Scenario), CreatedBy: adminID,
		}
		if err := model.CreateCostRuleDraftWithTx(db, rule); err != nil {
			return issues, err
		}
		id := int(rule.ID)
		item.MaterializedID = &id
		item.MaterializedType = "cost_rule_draft"
		item.State = string(types.ConfigImportItemStateChanged)
		merged[key] = struct {
			id   int
			json string
		}{id: id, json: string(configJSON)}
		if err := persistConfigImportItemState(db, item); err != nil {
			return issues, err
		}
	}
	return issues, nil
}

func configImportCostNormalizationMismatch(draft types.ConfigImportCostRuleDraft, normalized types.CostRuleConfigV1) bool {
	for _, values := range [][2]*string{
		{draft.NormalizedUSDUnitPrice, normalized.NormalizedUSDPrices.UnitPrice},
		{draft.NormalizedUSDPricePerSecond, normalized.NormalizedUSDPrices.PricePerSecond},
		{draft.NormalizedUSDInputPerMillion, normalized.NormalizedUSDPrices.InputPerMillion},
		{draft.NormalizedUSDOutputPerMillion, normalized.NormalizedUSDPrices.OutputPerMillion},
		{draft.NormalizedUSDCompletionPerMillion, normalized.NormalizedUSDPrices.CompletionPerMillion},
		{draft.NormalizedUSDTotalPerMillion, normalized.NormalizedUSDPrices.TotalPerMillion},
	} {
		if values[0] == nil {
			continue
		}
		if values[1] == nil {
			return true
		}
		provided, providedErr := decimal.NewFromString(strings.TrimSpace(*values[0]))
		recomputed, recomputedErr := decimal.NewFromString(strings.TrimSpace(*values[1]))
		if providedErr != nil || recomputedErr != nil || !provided.Equal(recomputed) {
			return true
		}
	}
	return false
}

func persistConfigImportItemState(db *gorm.DB, item *model.ConfigImportItem) error {
	return db.Model(&model.ConfigImportItem{}).Where("id = ?", item.ID).Updates(map[string]any{
		"state": item.State, "materialized_type": item.MaterializedType, "materialized_id": item.MaterializedID,
		"conflict_reason": item.ConflictReason, "exclusion_reason": item.ExclusionReason, "canonical_json": item.CanonicalJSON,
		"updated_at": common.GetTimestamp(),
	}).Error
}

func configImportCostRuleConfig(draft types.ConfigImportCostRuleDraft) (types.CostRuleConfigV1, error) {
	mode := types.CostMode(strings.TrimSpace(draft.CostMode))
	if mode == "" {
		return types.CostRuleConfigV1{}, errors.New("cost mode is required")
	}
	config := types.CostRuleConfigV1{
		Currency: draft.Currency, BillingMultiplier: pointerStringValue(draft.BillingMultiplier),
		PurchaseDiscountRatio: pointerStringValue(draft.PurchaseDiscountRatio), RechargeExchangeRatio: pointerStringValue(draft.RechargeExchangeRatio),
		FeeRate: pointerStringValue(draft.FeeRate), CurrencyToUSDRate: pointerStringValue(draft.CurrencyToUSDRate),
		UnitPrice: draft.UnitPrice, PricePerSecond: draft.PricePerSecond, InputPerMillion: draft.InputPerMillion,
		OutputPerMillion: draft.OutputPerMillion, CompletionPerMillion: draft.CompletionPerMillion, TotalPerMillion: draft.TotalPerMillion,
		ZeroCostReason: draft.ZeroCostReason, ChargeEvent: types.CostChargeEvent(draft.ChargeEvent), MeterSource: types.CostMeterSource(draft.MeterSource), TokenMode: types.CostTokenMode(draft.TokenMode),
	}
	if mode == types.CostModeFree && config.ZeroCostReason == "" {
		config.ZeroCostReason = "config_import"
	}
	if mode == types.CostModePerDuration && config.ChargeEvent == types.CostChargeResponseSucceeded {
		config.ChargeEvent = types.CostChargeTaskSucceeded
	}
	if config.ChargeEvent == "" {
		if mode == types.CostModePerDuration {
			config.ChargeEvent = types.CostChargeTaskSucceeded
		} else {
			config.ChargeEvent = types.CostChargeResponseSucceeded
		}
	}
	if config.MeterSource == "" {
		if mode == types.CostModePerDuration {
			config.MeterSource = types.CostMeterValidatedRequest
		} else if mode == types.CostModePerToken {
			config.MeterSource = types.CostMeterUpstreamUsage
		}
	}
	if mode == types.CostModePerToken && config.TokenMode == "" {
		switch {
		case config.InputPerMillion != nil || config.OutputPerMillion != nil:
			config.TokenMode = types.CostTokenModeInputOutput
		case config.CompletionPerMillion != nil:
			config.TokenMode = types.CostTokenModeCompletion
		default:
			config.TokenMode = types.CostTokenModeTotal
		}
	}
	return config, nil
}

func pointerStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stageConfigImportProposals(db *gorm.DB, items []model.ConfigImportItem) ([]configImportStageIssue, error) {
	issues := make([]configImportStageIssue, 0)
	costBySKU, err := configImportCostBySKU(items)
	if err != nil {
		return issues, err
	}
	canonicalModelsBySKU, err := configImportCanonicalModelsBySKU(items)
	if err != nil {
		return issues, err
	}
	for index := range items {
		item := &items[index]
		if item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		switch item.EntityType {
		case "sale_proposals":
			var proposal types.ConfigImportSaleProposal
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &proposal); err != nil {
				return issues, err
			}
			cost := costBySKU[proposal.ModelSKURef]
			if cost == "" {
				cost = "0"
			}
			recomputed, saleIssues, err := recomputeConfigImportSaleProposal(proposal, cost)
			if err != nil {
				item.State = string(types.ConfigImportItemStateConflict)
				item.ConflictReason = err.Error()
				if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
					return issues, persistErr
				}
				continue
			}
			for _, issue := range saleIssues {
				issues = append(issues, configImportStageIssue{Code: issue.Code, Severity: types.ConfigImportIssueSeverity(issue.Severity), BusinessID: item.BusinessID, Message: issue.Message})
			}
			if recomputed.BillingExpr != "" {
				if _, err := billingexpr.CompileFromCache(recomputed.BillingExpr); err != nil {
					item.State = string(types.ConfigImportItemStateConflict)
					item.ConflictReason = "PRICING_BILLING_EXPR_INVALID: " + err.Error()
					if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
						return issues, persistErr
					}
					continue
				}
				if err := billing_setting.SmokeTestExpr(recomputed.BillingExpr); err != nil {
					item.State = string(types.ConfigImportItemStateConflict)
					item.ConflictReason = "PRICING_BILLING_EXPR_INVALID: " + err.Error()
					if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
						return issues, persistErr
					}
					continue
				}
			}
			if recomputed.DurationPrice != nil {
				if _, err := configImportDurationPrice(*recomputed.DurationPrice); err != nil {
					item.State = string(types.ConfigImportItemStateConflict)
					item.ConflictReason = "PRICING_DURATION_INVALID"
					if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
						return issues, persistErr
					}
					continue
				}
			}
			if recomputed.SeedanceTokenPrice != nil {
				if _, err := configImportSeedanceScenarioTokenPrice(recomputed); err != nil {
					item.State = string(types.ConfigImportItemStateConflict)
					item.ConflictReason = "PRICING_SEEDANCE_TOKEN_INVALID"
					if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
						return issues, persistErr
					}
					continue
				}
			}
			canonicalModel := canonicalModelsBySKU[recomputed.ModelSKURef]
			if canonicalModel == "" {
				canonicalModel = recomputed.ModelSKURef
			}
			if scopeIssue := validateConfigImportPricingGroupScope(db, canonicalModel, recomputed); scopeIssue != nil {
				issues = append(issues, configImportStageIssue{Code: scopeIssue.Code, Severity: types.ConfigImportIssueSeverityWarning, BusinessID: item.BusinessID, Message: scopeIssue.Message})
			}
			optionPatches, err := configImportSaleOptionPatches(recomputed, canonicalModelsBySKU[recomputed.ModelSKURef])
			if err != nil {
				item.State = string(types.ConfigImportItemStateConflict)
				item.ConflictReason = err.Error()
				if persistErr := persistConfigImportItemState(db, item); persistErr != nil {
					return issues, persistErr
				}
				continue
			}
			if err := updateConfigImportItemProposal(db, item, configImportStagedSaleProposal{Proposal: recomputed, OptionPatches: optionPatches}, map[string]any{"kind": "sale", "status": "recomputed"}); err != nil {
				return issues, err
			}
		case "model_mappings":
			if err := updateConfigImportItemProposal(db, item, nil, map[string]any{"kind": "model_mapping", "status": "proposal"}); err != nil {
				return issues, err
			}
		case "route_blueprints":
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
				return issues, err
			}
			if blueprint.MergeMode == types.ConfigImportRouteMergeModeSkip {
				item.State = string(types.ConfigImportItemStateExcluded)
				item.ExclusionReason = "route merge mode skip"
				if err := persistConfigImportItemState(db, item); err != nil {
					return issues, err
				}
				continue
			}
			if blueprint.MergeMode == "" {
				blueprint.MergeMode = types.ConfigImportRouteMergeModeMerge
			}
			disabled := false
			for targetIndex := range blueprint.Targets {
				blueprint.Targets[targetIndex].Enabled = &disabled
			}
			if err := updateConfigImportItemProposal(db, item, blueprint, map[string]any{"kind": "route", "merge_mode": blueprint.MergeMode, "enabled": false}); err != nil {
				return issues, err
			}
		}
	}
	return issues, nil
}

type configImportStagedSaleProposal struct {
	Proposal      types.ConfigImportSaleProposal `json:"proposal"`
	OptionPatches map[string]any                 `json:"option_patches"`
}

func configImportCanonicalModelsBySKU(items []model.ConfigImportItem) (map[string]string, error) {
	models := make(map[string]string)
	for _, item := range items {
		if item.EntityType != "model_mappings" {
			continue
		}
		var mapping types.ConfigImportModelMapping
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
			return nil, err
		}
		canonicalModel := configImportRuntimeCanonicalModel(mapping.CanonicalModel)
		if existing, found := models[mapping.SKURef]; !found || canonicalModel < existing {
			models[mapping.SKURef] = canonicalModel
		}
	}
	return models, nil
}

func configImportSaleOptionPatches(proposal types.ConfigImportSaleProposal, canonicalModel string) (map[string]any, error) {
	canonicalModel = strings.TrimSpace(canonicalModel)
	if canonicalModel == "" {
		canonicalModel = proposal.ModelSKURef
	}
	patches := make(map[string]any)
	isSeedance := seedancepricing.Family(canonicalModel) != ""
	if isSeedance {
		if proposal.SeedanceTokenPrice == nil || strings.TrimSpace(proposal.Scenario) == "" || strings.TrimSpace(proposal.Resolution) == "" {
			return nil, configImportError("PRICING_SEEDANCE_SCENARIO_REQUIRED", "Seedance sale proposal requires resolution, scenario, and official token price")
		}
		tokenPrice, err := configImportSeedanceScenarioTokenPrice(proposal)
		if err != nil {
			return nil, err
		}
		models := configImportSeedancePricingModels(canonicalModel)
		modes := make(map[string]string, len(models))
		prices := make(map[string]types.SeedanceTokenPrice, len(models))
		for _, modelName := range models {
			modes[modelName] = billing_setting.BillingModeSeedanceTokens
			prices[modelName] = tokenPrice
		}
		expressions := make(map[string]string, len(models))
		for _, modelName := range models {
			expressions[modelName] = ""
		}
		patches["billing_setting.billing_mode"] = modes
		patches["billing_setting.billing_expr"] = expressions
		patches["billing_setting.seedance_token_price"] = prices
		durationCleanup := make(map[string]any, len(models))
		for _, modelName := range models {
			durationCleanup[modelName] = nil
		}
		patches["billing_setting.duration_price"] = durationCleanup
		for _, key := range []string{"ModelPrice", "ModelRatio", "CompletionRatio"} {
			cleanup := make(map[string]any, len(models))
			for _, modelName := range models {
				cleanup[modelName] = nil
			}
			patches[key] = cleanup
		}
		return patches, nil
	}
	if proposal.BillingExpr != "" {
		patches["billing_setting.billing_mode"] = map[string]string{canonicalModel: billing_setting.BillingModeTieredExpr}
		patches["billing_setting.billing_expr"] = map[string]string{canonicalModel: proposal.BillingExpr}
		return patches, nil
	}
	if proposal.DurationPrice != nil {
		durationPrice, err := configImportDurationPrice(*proposal.DurationPrice)
		if err != nil {
			return nil, err
		}
		patches["billing_setting.billing_mode"] = map[string]string{canonicalModel: billing_setting.BillingModePerDuration}
		patches["billing_setting.duration_price"] = map[string]types.DurationPrice{canonicalModel: durationPrice}
		return patches, nil
	}
	price := proposal.UnitPrice
	if price == nil {
		price = proposal.PricePerUnit
	}
	if price == nil {
		for _, candidate := range []*string{proposal.InputPerMillion, proposal.OutputPerMillion, proposal.CompletionPerMillion, proposal.TotalPerMillion} {
			if candidate != nil {
				price = candidate
				break
			}
		}
	}
	if price != nil {
		parsed, err := decimal.NewFromString(*price)
		if err != nil {
			return nil, configImportError("PRICING_DECIMAL", "price is not a decimal")
		}
		asFloat, _ := parsed.Float64()
		patches["ModelPrice"] = map[string]float64{canonicalModel: asFloat}
	}
	return patches, nil
}

func configImportSeedanceScenarioTokenPrice(proposal types.ConfigImportSaleProposal) (types.SeedanceTokenPrice, error) {
	if proposal.SeedanceTokenPrice == nil {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_SCENARIO_REQUIRED", "Seedance official token price is required")
	}
	pricingVersion := strings.TrimSpace(proposal.SeedanceTokenPrice.PricingVersion)
	source := strings.TrimSpace(proposal.SeedanceTokenPrice.Source)
	if pricingVersion == "" || source == "" {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_AUDIT_REQUIRED", "Seedance official token price requires pricing_version and source")
	}
	if !strings.EqualFold(strings.TrimSpace(proposal.Currency), "USD") {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_OFFICIAL_CURRENCY", "Seedance official sale price currency must be USD")
	}
	sourceRef := strings.TrimSpace(proposal.SourceRef)
	if pricingVersion != "official-token-v1" || strings.TrimSpace(proposal.Sheet) != "官方售价" ||
		!strings.HasPrefix(sourceRef, "SRC-OFFICIAL-SEEDANCE-") || proposal.Row == nil || *proposal.Row <= 0 ||
		source != fmt.Sprintf("%s!%d", sourceRef, *proposal.Row) {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_OFFICIAL_SOURCE", "Seedance sale price must reference an official pricing sheet row")
	}
	scenario := strings.ToLower(strings.TrimSpace(proposal.Scenario))
	if scenario != types.SeedanceTokenScenarioNoVideo && scenario != types.SeedanceTokenScenarioWithVideo {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_SCENARIO_INVALID", "unsupported Seedance pricing scenario %q", proposal.Scenario)
	}
	resolution := strings.ToLower(strings.TrimSpace(proposal.Resolution))
	if resolution == "" {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_RESOLUTION_REQUIRED", "Seedance pricing resolution is required")
	}
	price, err := decimal.NewFromString(strings.TrimSpace(proposal.SeedanceTokenPrice.PricePerMillion))
	if err != nil || !price.IsPositive() {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_DECIMAL", "Seedance price_per_million is not a positive decimal")
	}
	tokenPrice := types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
		types.SeedanceTokenScenarioKey(resolution, scenario): {
			PricePerMillion: price.String(),
			Width:           proposal.SeedanceTokenPrice.Width,
			Height:          proposal.SeedanceTokenPrice.Height,
			FrameRate:       proposal.SeedanceTokenPrice.FrameRate,
			PricingVersion:  pricingVersion,
			Source:          source,
		},
	}}
	if err := tokenPrice.Validate(relaycommon.MaxTokensLimit); err != nil {
		return types.SeedanceTokenPrice{}, configImportError("PRICING_SEEDANCE_TOKEN_INVALID", "%v", err)
	}
	return tokenPrice, nil
}

func configImportSeedancePricingModels(canonicalModel string) []string {
	models := []string{canonicalModel}
	if seedancepricing.Family(canonicalModel) != seedancepricing.Family20Mini {
		return models
	}
	for _, alias := range []string{modelrouting.Seedance20Mini, "doubao-seedance-2-0-mini-260128"} {
		if !common.StringsContains(models, alias) {
			models = append(models, alias)
		}
	}
	return models
}

func configImportDurationPrice(proposal types.DurationPriceProposal) (types.DurationPrice, error) {
	price, err := decimal.NewFromString(strings.TrimSpace(proposal.Price))
	if err != nil || price.IsNegative() {
		return types.DurationPrice{}, configImportError("PRICING_DECIMAL", "duration price is not a non-negative decimal")
	}
	asFloat, _ := price.Float64()
	durationPrice := types.DurationPrice{
		Price:                  asFloat,
		Unit:                   strings.TrimSpace(proposal.Unit),
		RoundingStepSeconds:    proposal.RoundingStepSeconds,
		MinimumDurationSeconds: proposal.MinimumDurationSeconds,
	}
	if err := durationPrice.Validate(relaycommon.MaxTaskDurationSeconds); err != nil {
		return types.DurationPrice{}, configImportError("PRICING_DURATION_INVALID", "%v", err)
	}
	return durationPrice, nil
}

func configImportCostBySKU(items []model.ConfigImportItem) (map[string]string, error) {
	byLineModel := make(map[string]string)
	for _, item := range items {
		if item.EntityType != "cost_rule_drafts" || item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		var draft types.ConfigImportCostRuleDraft
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
			return nil, err
		}
		config, err := configImportCostRuleConfig(draft)
		if err != nil {
			continue
		}
		normalized, err := NormalizeCostRuleConfig(types.CostMode(draft.CostMode), config)
		if err != nil {
			continue
		}
		price := normalized.NormalizedUSDPrices.UnitPrice
		if price == nil {
			price = normalized.NormalizedUSDPrices.PricePerSecond
		}
		if price == nil {
			price = normalized.NormalizedUSDPrices.TotalPerMillion
		}
		if price == nil {
			price = normalized.NormalizedUSDPrices.CompletionPerMillion
		}
		if price == nil {
			price = normalized.NormalizedUSDPrices.InputPerMillion
		}
		if price != nil {
			byLineModel[draft.LineRef+"|"+draft.UpstreamModel] = *price
		}
	}
	bySKU := make(map[string]string)
	for _, item := range items {
		if item.EntityType != "model_skus" {
			continue
		}
		var sku types.ConfigImportModelSKU
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &sku); err != nil {
			return nil, err
		}
		bySKU[sku.BusinessID] = byLineModel[sku.LineRef+"|"+sku.UpstreamModel]
	}
	return bySKU, nil
}

func validateConfigImportPricingGroupScope(db *gorm.DB, canonicalModel string, proposal types.ConfigImportSaleProposal) *configImportStageIssue {
	groups := proposal.SelectedGroups
	if len(groups) == 0 {
		groups = []string{"default"}
	}
	selected := make(map[string]struct{}, len(groups))
	base := decimal.Zero
	baseSet := false
	if len(proposal.GroupPrices) > 0 {
		for _, group := range groups {
			selected[group] = struct{}{}
			price, ok := proposal.GroupPrices[group]
			if !ok {
				return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: fmt.Sprintf("selected group %q has no price", group)}
			}
			parsed, err := decimal.NewFromString(price)
			if err != nil {
				return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: "group price is not a decimal"}
			}
			ratio := decimal.NewFromFloat(ratio_setting.GetGroupRatio(group))
			if ratio.IsZero() {
				return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: "group ratio cannot represent a positive group price"}
			}
			if !baseSet {
				base = parsed.Div(ratio)
				baseSet = true
				continue
			}
			if !parsed.Equal(base.Mul(ratio)) {
				return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: "selected group prices cannot be represented by one supported base price"}
			}
		}
	} else {
		for _, group := range groups {
			selected[group] = struct{}{}
		}
		for _, candidate := range []*string{proposal.UnitPrice, proposal.PricePerUnit} {
			if candidate == nil {
				continue
			}
			parsed, err := decimal.NewFromString(*candidate)
			if err != nil {
				return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: "base price is not a decimal"}
			}
			base = parsed
			baseSet = true
			break
		}
	}
	if db == nil || strings.TrimSpace(canonicalModel) == "" || !db.Migrator().HasTable(&model.Ability{}) {
		return nil
	}
	var abilities []model.Ability
	if err := db.Where("model = ? AND enabled = ?", canonicalModel, true).Find(&abilities).Error; err != nil {
		return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: "cannot verify existing group access"}
	}
	unselectedGroups := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		if _, found := selected[ability.Group]; !found {
			unselectedGroups = append(unselectedGroups, ability.Group)
		}
	}
	sort.Strings(unselectedGroups)
	for _, group := range unselectedGroups {
		currentPrice, found := ratio_setting.GetModelPrice(canonicalModel, false)
		if !found || !baseSet || !base.Equal(decimal.NewFromFloat(currentPrice)) {
			return &configImportStageIssue{Code: "PRICING_GROUP_SCOPE_UNREPRESENTABLE", Message: fmt.Sprintf("unselected group %q would receive a changed global price", group)}
		}
	}
	return nil
}

func recomputeConfigImportSaleProposal(proposal types.ConfigImportSaleProposal, costUSD string) (types.ConfigImportSaleProposal, []model.ConfigImportIssue, error) {
	issues := make([]model.ConfigImportIssue, 0)
	proposal.Enabled = nil
	for name, value := range map[string]*string{
		"unit_price": proposal.UnitPrice, "price_per_unit": proposal.PricePerUnit, "margin_ratio": proposal.MarginRatio,
		"input_per_million": proposal.InputPerMillion, "output_per_million": proposal.OutputPerMillion,
		"completion_per_million": proposal.CompletionPerMillion, "total_per_million": proposal.TotalPerMillion,
	} {
		if value == nil {
			continue
		}
		parsed, err := decimal.NewFromString(strings.TrimSpace(*value))
		if err != nil || parsed.IsNegative() {
			return proposal, nil, configImportError("PRICING_DECIMAL", "%s is not a non-negative decimal", name)
		}
		normalized := parsed.String()
		*value = normalized
	}
	if proposal.DurationPrice != nil {
		price, err := decimal.NewFromString(strings.TrimSpace(proposal.DurationPrice.Price))
		if err != nil || price.IsNegative() {
			return proposal, nil, configImportError("PRICING_DECIMAL", "duration price is not a non-negative decimal")
		}
		proposal.DurationPrice.Price = price.String()
	}
	if proposal.SeedanceTokenPrice != nil {
		price, err := decimal.NewFromString(strings.TrimSpace(proposal.SeedanceTokenPrice.PricePerMillion))
		if err != nil || !price.IsPositive() {
			return proposal, nil, configImportError("PRICING_DECIMAL", "Seedance price_per_million is not a positive decimal")
		}
		proposal.SeedanceTokenPrice.PricePerMillion = price.String()
	}
	hasExpression := strings.TrimSpace(proposal.BillingExpr) != ""
	hasDuration := proposal.DurationPrice != nil
	hasSeedanceToken := proposal.SeedanceTokenPrice != nil
	hasToken := proposal.InputPerMillion != nil || proposal.OutputPerMillion != nil || proposal.CompletionPerMillion != nil || proposal.TotalPerMillion != nil
	hasFixedPrice := proposal.UnitPrice != nil || proposal.PricePerUnit != nil
	modeCount := 0
	for _, present := range []bool{hasExpression, hasDuration, hasSeedanceToken, hasToken, hasFixedPrice} {
		if present {
			modeCount++
		}
	}
	if modeCount > 1 {
		return proposal, nil, configImportError("PRICING_MODE_CONFLICT", "sale proposal contains more than one pricing mode")
	}
	if proposal.UnitPrice != nil || proposal.PricePerUnit != nil {
		cost, err := decimal.NewFromString(strings.TrimSpace(costUSD))
		if err != nil {
			return proposal, nil, configImportError("PRICING_DECIMAL", "cost is not a decimal")
		}
		sale := proposal.UnitPrice
		if sale == nil {
			sale = proposal.PricePerUnit
		}
		if sale != nil && decimal.RequireFromString(*sale).LessThan(cost) {
			issues = append(issues, model.ConfigImportIssue{Severity: string(types.ConfigImportIssueSeverityWarning), Code: "PRICING_NEGATIVE_MARGIN", BusinessID: proposal.BusinessID, Message: "sale price is below recomputed cost", ResolutionStatus: "open"})
		}
	}
	if !hasExpression && !hasSeedanceToken {
		tokenExpr, err := configImportTokenPricingExpression(proposal)
		if err != nil {
			return proposal, nil, err
		}
		if tokenExpr != "" {
			proposal.BillingExpr = tokenExpr
		}
	}
	expectedMode := billing_setting.BillingModeRatio
	if proposal.BillingExpr != "" {
		expectedMode = billing_setting.BillingModeTieredExpr
	} else if proposal.SeedanceTokenPrice != nil {
		expectedMode = billing_setting.BillingModeSeedanceTokens
	} else if proposal.DurationPrice != nil {
		expectedMode = billing_setting.BillingModePerDuration
	}
	// Legacy V1 sale rows use the source metering term "per_token". The
	// runtime represents the same token pricing as a compiled tier expression.
	// Keep rejecting every other disagreement between authoritative inputs and
	// runtime mode so incompatible price configurations cannot be staged.
	legacyTokenMode := hasToken && proposal.BillingMode == string(types.CostModePerToken)
	if proposal.BillingMode != "" && proposal.BillingMode != expectedMode && !legacyTokenMode {
		return proposal, nil, configImportError("PRICING_MODE_CONFLICT", "billing_mode %q does not match sale pricing inputs", proposal.BillingMode)
	}
	proposal.BillingMode = expectedMode
	if len(proposal.SelectedGroups) == 0 {
		proposal.SelectedGroups = []string{"default"}
	}
	return proposal, issues, nil
}

func configImportTokenPricingExpression(proposal types.ConfigImportSaleProposal) (string, error) {
	hasInputOutput := proposal.InputPerMillion != nil || proposal.OutputPerMillion != nil
	hasCompletion := proposal.CompletionPerMillion != nil
	hasTotal := proposal.TotalPerMillion != nil
	if !hasInputOutput && !hasCompletion && !hasTotal {
		return "", nil
	}
	if (hasInputOutput && (hasCompletion || hasTotal)) || (hasCompletion && hasTotal) {
		return "", configImportError("PRICING_TOKEN_MODE_INVALID", "token sale proposal contains multiple price modes")
	}
	if hasInputOutput {
		if proposal.InputPerMillion == nil || proposal.OutputPerMillion == nil {
			return "", configImportError("PRICING_TOKEN_MODE_INVALID", "input/output token pricing requires both prices")
		}
		if proposal.TokenMode != "" && proposal.TokenMode != string(types.CostTokenModeInputOutput) {
			return "", configImportError("PRICING_TOKEN_MODE_INVALID", "token mode does not match input/output prices")
		}
		return fmt.Sprintf(`v1:tier("base", p * %s + c * %s)`, *proposal.InputPerMillion, *proposal.OutputPerMillion), nil
	}
	if hasCompletion {
		if proposal.TokenMode != "" && proposal.TokenMode != string(types.CostTokenModeCompletion) {
			return "", configImportError("PRICING_TOKEN_MODE_INVALID", "token mode does not match completion price")
		}
		return fmt.Sprintf(`v1:tier("base", c * %s)`, *proposal.CompletionPerMillion), nil
	}
	if proposal.TokenMode != "" && proposal.TokenMode != string(types.CostTokenModeTotal) {
		return "", configImportError("PRICING_TOKEN_MODE_INVALID", "token mode does not match total price")
	}
	return fmt.Sprintf(`v1:tier("base", (p + c) * %s)`, *proposal.TotalPerMillion), nil
}

func updateConfigImportItemProposal(db *gorm.DB, item *model.ConfigImportItem, proposal any, diff map[string]any) error {
	var document map[string]any
	if err := common.UnmarshalJsonStr(item.CanonicalJSON, &document); err != nil {
		return err
	}
	if proposal != nil {
		encoded, err := common.Marshal(proposal)
		if err != nil {
			return err
		}
		var proposalMap map[string]any
		if err := common.Unmarshal(encoded, &proposalMap); err != nil {
			return err
		}
		document["staged_proposal"] = proposalMap
	}
	document["staged_diff"] = diff
	encoded, err := common.Marshal(document)
	if err != nil {
		return err
	}
	item.CanonicalJSON = string(encoded)
	item.State = string(types.ConfigImportItemStateChanged)
	return db.Model(&model.ConfigImportItem{}).Where("id = ?", item.ID).Updates(map[string]any{
		"canonical_json": item.CanonicalJSON, "state": item.State, "updated_at": common.GetTimestamp(),
	}).Error
}

func persistConfigImportStageIssues(db *gorm.DB, batchID int64, stageIssues []configImportStageIssue) error {
	for _, issue := range stageIssues {
		if issue.Code == "" {
			continue
		}
		var count int64
		if err := db.Model(&model.ConfigImportIssue{}).Where("batch_id = ? AND code = ? AND business_id = ?", batchID, issue.Code, issue.BusinessID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			if issue.Code == "PRICING_GROUP_SCOPE_UNREPRESENTABLE" {
				if err := db.Model(&model.ConfigImportIssue{}).
					Where("batch_id = ? AND code = ? AND business_id = ?", batchID, issue.Code, issue.BusinessID).
					Updates(map[string]any{
						"severity": string(issue.Severity), "message": issue.Message, "resolution_status": "open", "updated_at": common.GetTimestamp(),
					}).Error; err != nil {
					return err
				}
			}
			continue
		}
		if err := db.Create(&model.ConfigImportIssue{BatchID: batchID, Severity: string(issue.Severity), Code: issue.Code, BusinessID: issue.BusinessID, Message: issue.Message, ResolutionStatus: "open"}).Error; err != nil {
			return err
		}
	}
	return nil
}

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
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
				skipStateJSON, err := excludeConfigImportLineDependents(tx, batchID, currentItems, input.LineRef, input.Reason)
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
			if input.Action == types.ConfigImportBindingActionCreate && channel.Status != common.ChannelStatusManuallyDisabled {
				return configImportError("BINDING_NEW_CHANNEL_STATUS", "new channel %d must be manually disabled", channel.Id)
			}
			if err := rejectConfigImportChannelLineConflict(tx, batchID, input.LineRef, channel.Id); err != nil {
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

func validateConfigImportBindingInputs(bindings []dto.ConfigImportBindingInput) error {
	if len(bindings) == 0 {
		return configImportError("SCHEMA_BINDING_REQUEST", "bindings are required")
	}
	seenLines := make(map[string]struct{}, len(bindings))
	for index := range bindings {
		input := &bindings[index]
		input.LineRef = strings.TrimSpace(input.LineRef)
		input.Reason = strings.TrimSpace(input.Reason)
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
			if input.Reason != "" {
				return configImportError("SCHEMA_BINDING_REASON", "bindings[%d].reason is only valid for skip", index)
			}
		case types.ConfigImportBindingActionSkip:
			if input.ChannelID != nil {
				return configImportError("SCHEMA_BINDING_CHANNEL", "bindings[%d].channel_id is not valid for skip", index)
			}
			if input.CredentialsConfirmed {
				return configImportError("SCHEMA_BINDING_CREDENTIAL_CONFIRMATION", "bindings[%d].credentials_confirmed is not valid for skip", index)
			}
			if input.Reason == "" {
				return configImportError("SCHEMA_BINDING_REASON", "bindings[%d].reason is required for skip", index)
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
	Reason string                         `json:"reason"`
	Items  []configImportSkippedItemState `json:"items"`
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
		case "model_skus":
			var sku types.ConfigImportModelSKU
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &sku); err != nil {
				return nil, fmt.Errorf("decode config import model SKU %q: %w", item.BusinessID, err)
			}
			if modelName := strings.TrimSpace(sku.UpstreamModel); modelName != "" {
				catalog.models[sku.LineRef] = append(catalog.models[sku.LineRef], modelName)
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
	if channel.Type != *master.ChannelType {
		return configImportError("BINDING_CHANNEL_TYPE", "channel %d type does not match line_ref %q", channel.Id, line.LineRef)
	}
	requiredModels := catalog.models[line.LineRef]
	if len(requiredModels) == 0 {
		return configImportError("BINDING_CHANNEL_MODEL", "line_ref %q has no declared upstream models", line.LineRef)
	}
	declaredModels := make(map[string]struct{}, len(channel.GetModels()))
	for _, modelName := range channel.GetModels() {
		if modelName = strings.TrimSpace(modelName); modelName != "" {
			declaredModels[modelName] = struct{}{}
		}
	}
	for _, modelName := range requiredModels {
		if _, found := declaredModels[modelName]; !found {
			return configImportError("BINDING_CHANNEL_MODEL", "channel %d does not declare model %q for line_ref %q", channel.Id, modelName, line.LineRef)
		}
	}
	if err := validateConfigImportLineCapability(line, channel); err != nil {
		return err
	}
	return nil
}

func validateConfigImportLineCapability(line types.ConfigImportChannelLine, channel *model.Channel) error {
	secureGroups := map[string]dto.SecureVideoGroup{
		"secure-discount":   dto.SecureVideoGroupDiscount,
		"secure-overseas":   dto.SecureVideoGroupOverseas,
		"secure-enterprise": dto.SecureVideoGroupEnterprise,
	}
	if expectedGroup, secureLine := secureGroups[line.LineRef]; secureLine {
		if channel.Type != constant.ChannelTypeSecure {
			return configImportError("BINDING_LINE_CAPABILITY", "line_ref %q requires a Secure channel", line.LineRef)
		}
		settings := dto.ChannelOtherSettings{}
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

func rejectConfigImportChannelLineConflict(tx *gorm.DB, batchID int64, lineRef string, channelID int) error {
	var count int64
	if err := tx.Model(&model.ConfigImportBinding{}).
		Where("batch_id = ? AND channel_id = ? AND line_ref <> ? AND action IN ?", batchID, channelID, lineRef,
			[]string{string(types.ConfigImportBindingActionBind), string(types.ConfigImportBindingActionCreate)}).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return configImportError("BINDING_CHANNEL_LINE_CONFLICT", "channel %d is already bound to another line in batch %d", channelID, batchID)
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
		err := tx.Create(&model.ConfigImportBinding{
			BatchID: batchID, LineRef: input.LineRef, Action: string(input.Action), ChannelID: channelID,
			CredentialsConfirmedBy: confirmedBy, CredentialsConfirmedAt: confirmedAt, SkipStateJSON: skipStateJSON,
		}).Error
		return mapConfigImportBindingChannelConflict(err)
	}
	if err != nil {
		return err
	}
	if input.Action == types.ConfigImportBindingActionSkip && skipStateJSON == "" {
		skipStateJSON = existing.SkipStateJSON
	}
	err = tx.Model(&existing).Updates(map[string]any{
		"action":                   string(input.Action),
		"channel_id":               channelID,
		"credentials_confirmed_by": confirmedBy,
		"credentials_confirmed_at": confirmedAt,
		"skip_state_json":          skipStateJSON,
		"updated_at":               common.GetTimestamp(),
	}).Error
	return mapConfigImportBindingChannelConflict(err)
}

func excludeConfigImportLineDependents(
	tx *gorm.DB,
	batchID int64,
	items []model.ConfigImportItem,
	lineRef string,
	reason string,
) (string, error) {
	owners, err := configImportActiveSkipOwners(tx, batchID, "")
	if err != nil {
		return "", err
	}
	managedStates := configImportSkippedItemStatesByID(owners)
	modelSKURefs := make(map[string]struct{})
	mappingRefs := make(map[string]struct{})
	for _, item := range items {
		switch item.EntityType {
		case "model_skus":
			var sku types.ConfigImportModelSKU
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &sku); err != nil {
				return "", fmt.Errorf("decode config import model SKU %q: %w", item.BusinessID, err)
			}
			if sku.LineRef == lineRef {
				modelSKURefs[item.BusinessID] = struct{}{}
			}
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

	snapshot := configImportSkipStateSnapshot{
		Reason: reason,
		Items:  make([]configImportSkippedItemState, 0),
	}
	managedIDs := make([]int64, 0)
	for _, item := range items {
		excluded, err := configImportItemDependsOnLine(item, lineRef, modelSKURefs, mappingRefs)
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
		"exclusion_reason": reason,
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
					"exclusion_reason": configImportSkipReasonForItem(owners, item.ID),
					"updated_at":       common.GetTimestamp(),
				}).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Model(&model.ConfigImportItem{}).
			Where("id = ? AND batch_id = ? AND state = ? AND exclusion_reason = ?", item.ID, batchID,
				string(types.ConfigImportItemStateExcluded), snapshot.Reason).
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

func configImportSkipReasonForItem(owners []configImportSkipOwner, itemID int64) string {
	for _, owner := range owners {
		for _, item := range owner.Snapshot.Items {
			if item.ID == itemID {
				return owner.Snapshot.Reason
			}
		}
	}
	return ""
}

func mapConfigImportBindingChannelConflict(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "idx_config_import_binding_channel") ||
		strings.Contains(message, "config_import_bindings.batch_id, config_import_bindings.channel_id") ||
		strings.Contains(message, "config_import_bindings(batch_id,channel_id)") {
		return configImportError("BINDING_CHANNEL_LINE_CONFLICT", "channel is already bound to another line in this batch")
	}
	return err
}

func configImportItemDependsOnLine(
	item model.ConfigImportItem,
	lineRef string,
	modelSKURefs map[string]struct{},
	mappingRefs map[string]struct{},
) (bool, error) {
	switch item.EntityType {
	case "channel_lines":
		var line types.ConfigImportChannelLine
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &line); err != nil {
			return false, fmt.Errorf("decode config import line %q: %w", item.BusinessID, err)
		}
		return line.LineRef == lineRef, nil
	case "model_skus":
		var sku types.ConfigImportModelSKU
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &sku); err != nil {
			return false, fmt.Errorf("decode config import model SKU %q: %w", item.BusinessID, err)
		}
		return sku.LineRef == lineRef, nil
	case "sale_proposals":
		var proposal types.ConfigImportSaleProposal
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &proposal); err != nil {
			return false, fmt.Errorf("decode config import sale proposal %q: %w", item.BusinessID, err)
		}
		_, found := modelSKURefs[proposal.ModelSKURef]
		return found, nil
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

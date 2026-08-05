package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func ProjectPublicLog(log *model.Log, displayID int) *dto.PublicLog {
	if log == nil {
		return nil
	}

	other := dto.PublicLogOther{}
	if log.Other != "" {
		if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
			other = dto.PublicLogOther{}
		}
	}
	content := ""
	switch log.Type {
	case model.LogTypeTopup, model.LogTypeManage, model.LogTypeSystem, model.LogTypeLogin:
		content = common.MaskSensitiveInfo(log.Content)
	}

	return &dto.PublicLog{
		ID:               displayID,
		CreatedAt:        log.CreatedAt,
		Type:             log.Type,
		Content:          content,
		TokenName:        log.TokenName,
		ModelName:        log.ModelName,
		Quota:            log.Quota,
		PromptTokens:     log.PromptTokens,
		CompletionTokens: log.CompletionTokens,
		UseTime:          log.UseTime,
		IsStream:         log.IsStream,
		RequestID:        log.RequestId,
		Other:            other,
	}
}

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
	return &dto.PublicLog{
		ID:               displayID,
		CreatedAt:        log.CreatedAt,
		Type:             log.Type,
		Content:          "",
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

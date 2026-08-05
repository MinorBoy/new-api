package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectPublicLogDropsSupplierFactsByWhitelist(t *testing.T) {
	source := &model.Log{
		Id:                91,
		UserId:            10,
		CreatedAt:         1710000000,
		Type:              model.LogTypeConsume,
		Content:           "Authorization: Bearer secret-token",
		Username:          "alice",
		TokenName:         "user-token",
		ModelName:         "public-model",
		Quota:             123,
		PromptTokens:      11,
		CompletionTokens:  22,
		UseTime:           3,
		IsStream:          true,
		ChannelId:         40,
		ChannelName:       "supplier-secret",
		TokenId:           17,
		Group:             "internal-group",
		Ip:                "10.0.0.8",
		RequestId:         "req-public",
		UpstreamRequestId: "upstream-secret",
		Other: common.MapToJsonStr(map[string]interface{}{
			"login_method":          "password",
			"user_agent":            "public-browser",
			"ws":                    false,
			"cache_tokens":          7,
			"cache_creation_tokens": 0,
			"audio":                 true,
			"audio_input":           0,
			"subscription_consumed": 12,
			"subscription_remain":   88,
			"subscription_total":    100,
			"model_price":           0.2,
			"group_ratio":           1.25,
			"upstream_model_name":   "provider-model",
			"request_path":          "/v1/internal",
			"admin_info": map[string]interface{}{
				"cost_accounting_request_id": 88,
				"use_channel":                []int{40, 41},
				"routing":                    "provider-route",
			},
			"unknown_supplier_fact": "never-public",
		}),
	}

	projected := ProjectPublicLog(source, 1)
	require.NotNil(t, projected)

	assert.Equal(t, 1, projected.ID)
	assert.Equal(t, source.CreatedAt, projected.CreatedAt)
	assert.Equal(t, source.Type, projected.Type)
	assert.Equal(t, common.MaskSensitiveInfo(source.Content), projected.Content)
	assert.Equal(t, source.TokenName, projected.TokenName)
	assert.Equal(t, source.ModelName, projected.ModelName)
	assert.Equal(t, source.Quota, projected.Quota)
	assert.Equal(t, source.PromptTokens, projected.PromptTokens)
	assert.Equal(t, source.CompletionTokens, projected.CompletionTokens)
	assert.Equal(t, source.UseTime, projected.UseTime)
	assert.Equal(t, source.IsStream, projected.IsStream)
	assert.Equal(t, source.RequestId, projected.RequestID)
	assert.Equal(t, "password", projected.Other.LoginMethod)
	assert.Equal(t, "public-browser", projected.Other.UserAgent)
	require.NotNil(t, projected.Other.WebSocket)
	assert.False(t, *projected.Other.WebSocket)
	require.NotNil(t, projected.Other.CacheTokens)
	assert.Equal(t, 7, *projected.Other.CacheTokens)
	require.NotNil(t, projected.Other.CacheCreationTokens)
	assert.Zero(t, *projected.Other.CacheCreationTokens)
	require.NotNil(t, projected.Other.Audio)
	assert.True(t, *projected.Other.Audio)
	require.NotNil(t, projected.Other.AudioInput)
	assert.Zero(t, *projected.Other.AudioInput)
	require.NotNil(t, projected.Other.SubscriptionConsumed)
	assert.Equal(t, 12, *projected.Other.SubscriptionConsumed)

	payload, err := common.Marshal(projected)
	require.NoError(t, err)
	body := string(payload)
	assert.Contains(t, body, "req-public")
	assert.Contains(t, body, "public-model")
	assert.Contains(t, body, "cache_tokens")
	assert.NotContains(t, body, "secret-token")
	for _, forbidden := range []string{
		"91", "alice", "user_id", "username", "supplier-secret", "internal-group",
		"10.0.0.8", "token_id", "upstream-secret", "provider-model", "model_price",
		"group_ratio", "request_path", "cost_accounting_request_id", "use_channel",
		"provider-route", "unknown_supplier_fact", "never-public",
	} {
		assert.NotContains(t, body, forbidden)
	}
}

func TestProjectPublicLogReturnsNilForNilInput(t *testing.T) {
	assert.Nil(t, ProjectPublicLog(nil, 1))
}

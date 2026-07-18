package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestTaskBillingTokensPrefersPresentCompletionValue(t *testing.T) {
	tokens, ok, clamp := taskBillingTokensChecked(&relaycommon.TaskInfo{CompletionTokens: 0, TotalTokens: 1200, CompletionTokensPresent: true})
	assert.True(t, ok)
	assert.Equal(t, 0, tokens)
	assert.Nil(t, clamp)

	tokens, ok, clamp = taskBillingTokensChecked(&relaycommon.TaskInfo{CompletionTokens: 1000, TotalTokens: 1200, CompletionTokensPresent: true})
	assert.True(t, ok)
	assert.Equal(t, 1000, tokens)
	assert.Nil(t, clamp)

	tokens, ok, clamp = taskBillingTokensChecked(&relaycommon.TaskInfo{TotalTokens: 1200})
	assert.True(t, ok)
	assert.Equal(t, 1200, tokens)
	assert.Nil(t, clamp)

	tokens, ok, clamp = taskBillingTokensChecked(&relaycommon.TaskInfo{CompletionTokens: 800})
	assert.True(t, ok)
	assert.Equal(t, 800, tokens)
	assert.Nil(t, clamp)

	tokens, ok, clamp = taskBillingTokensChecked(&relaycommon.TaskInfo{CompletionTokens: -1, TotalTokens: 1200, CompletionTokensPresent: true})
	assert.True(t, ok)
	assert.Equal(t, 0, tokens)
	assert.NotNil(t, clamp)

	taskResult := &relaycommon.TaskInfo{TotalTokens: common.MaxQuota + 1}
	tokens, ok, clamp = taskBillingTokensChecked(taskResult)
	assert.True(t, ok)
	assert.Equal(t, common.MaxQuota, tokens)
	assert.NotNil(t, clamp)
	assert.Same(t, clamp, taskResult.BillingClamp)
	assert.Equal(t, common.MaxQuota, taskBillingTokens(taskResult))

	tokens, ok, clamp = taskBillingTokensChecked(&relaycommon.TaskInfo{TotalTokens: 0})
	assert.False(t, ok)
	assert.Equal(t, 0, tokens)
	assert.Nil(t, clamp)
}

package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestSecureAssetChannelMutationGuardRejectsBoundCredentialChanges(t *testing.T) {
	original := &model.Channel{Id: 7, Type: constant.ChannelTypeSecure, Key: "old-key"}
	effective := *original
	effective.Key = "new-key"

	err := secureAssetChannelMutationError(original, &effective, true)
	assert.Error(t, err)
}

func TestSecureAssetChannelMutationGuardAllowsNonCredentialChanges(t *testing.T) {
	original := &model.Channel{Id: 7, Type: constant.ChannelTypeSecure, Key: "old-key", Name: "before"}
	effective := *original
	effective.Name = "after"

	err := secureAssetChannelMutationError(original, &effective, true)
	assert.NoError(t, err)
}

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskSensitiveInfoMasksBearerTokensCaseInsensitively(t *testing.T) {
	masked := MaskSensitiveInfo("Authorization: Bearer secret-token; proxy bearer second-secret")

	assert.NotContains(t, masked, "secret-token")
	assert.NotContains(t, masked, "second-secret")
	assert.Equal(t, "Authorization: Bearer ***; proxy Bearer ***", masked)
}

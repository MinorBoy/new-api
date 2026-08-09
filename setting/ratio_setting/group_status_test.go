package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupStatusDefaultsMissingGroupsToEnabled(t *testing.T) {
	original := GroupStatus2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupStatusByJSONString(original))
	})

	require.NoError(t, UpdateGroupStatusByJSONString(`{"paused":false,"active":true}`))
	assert.False(t, IsGroupEnabled("paused"))
	assert.True(t, IsGroupEnabled("active"))
	assert.True(t, IsGroupEnabled("legacy"))
	assert.True(t, IsGroupEnabled("auto"))
}

func TestParseGroupStatusRejectsInvalidValues(t *testing.T) {
	_, err := ParseGroupStatusJSONString(`{"default":"yes"}`)
	require.Error(t, err)
}

func TestParseGroupStatusRejectsBlankNames(t *testing.T) {
	_, err := ParseGroupStatusJSONString(`{"  ":true}`)
	require.ErrorContains(t, err, "must not be empty")
}

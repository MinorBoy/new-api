package types

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRWMapUnmarshalInvalidJSONPreservesValues(t *testing.T) {
	values := NewRWMap[string, int]()
	values.Set("stable", 1)

	err := common.Unmarshal([]byte(`[]`), values)
	require.Error(t, err)

	value, ok := values.Get("stable")
	require.True(t, ok)
	assert.Equal(t, 1, value)
}

func TestRWMapUnmarshalNullLeavesEmptyUsableMap(t *testing.T) {
	values := NewRWMap[string, int]()
	values.Set("old", 1)

	require.NoError(t, common.Unmarshal([]byte(`null`), values))
	assert.Empty(t, values.ReadAll())
	_, ok := values.Get("old")
	assert.False(t, ok)

	encoded, err := common.Marshal(values)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(encoded))

	values.Set("new", 2)
	value, ok := values.Get("new")
	require.True(t, ok)
	assert.Equal(t, 2, value)
}

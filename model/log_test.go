package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDistinctModelLogs inserts a fixed set of log rows covering the
// behaviors GetDistinctLogModelNames must enforce:
//   - only type=LogTypeConsume rows are considered (logType default)
//   - empty model_name rows are excluded
//   - duplicate model names collapse to one entry
//   - user/time scoping applies
//
// It returns a cleanup that removes the seeded rows.
func seedDistinctModelLogs(t *testing.T) func() {
	t.Helper()
	rows := []*Log{
		// User 1, consume, distinct models within [100, 200].
		{UserId: 1, Type: LogTypeConsume, ModelName: "gpt-5.6", CreatedAt: 100},
		{UserId: 1, Type: LogTypeConsume, ModelName: "gpt-5.6", CreatedAt: 110},
		{UserId: 1, Type: LogTypeConsume, ModelName: "seedance-2", CreatedAt: 150},
		// User 2, consume — must be excluded for a user-1-scoped query.
		{UserId: 2, Type: LogTypeConsume, ModelName: "claude-opus", CreatedAt: 120},
		// User 1, non-consume (top-up) — must be excluded by the consume default.
		{UserId: 1, Type: LogTypeTopup, ModelName: "topup-only-model", CreatedAt: 130},
		// User 1, consume but empty model_name — must be excluded.
		{UserId: 1, Type: LogTypeConsume, ModelName: "", CreatedAt: 140},
		// User 1, consume but outside the default time window used below.
		{UserId: 1, Type: LogTypeConsume, ModelName: "out-of-range", CreatedAt: 300},
	}
	require.NoError(t, LOG_DB.Create(&rows).Error)
	return func() {
		ids := make([]int, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.Id)
		}
		LOG_DB.Where("id IN ?", ids).Delete(&Log{})
	}
}

func TestGetDistinctLogModelNames_DefaultScope(t *testing.T) {
	cleanup := seedDistinctModelLogs(t)
	defer cleanup()

	// Admin view (userId=0), default logType=consume, window [100,200].
	models, err := GetDistinctLogModelNames(0, 0, 100, 200, "", "", "", 0, "")
	require.NoError(t, err)

	// Includes user 2's in-window consume model; excludes top-up, empty, and
	// out-of-range rows. Sorted by model_name.
	assert.Equal(t, []string{"claude-opus", "gpt-5.6", "seedance-2"}, models)
}

func TestGetDistinctLogModelNames_UserScoped(t *testing.T) {
	cleanup := seedDistinctModelLogs(t)
	defer cleanup()

	// User 1 only: claude-opus (user 2) must drop out.
	models, err := GetDistinctLogModelNames(1, 0, 100, 200, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-5.6", "seedance-2"}, models)
}

func TestGetDistinctLogModelNames_RespectsExplicitLogType(t *testing.T) {
	cleanup := seedDistinctModelLogs(t)
	defer cleanup()

	// Explicitly ask for top-up rows: only the top-up model survives.
	models, err := GetDistinctLogModelNames(1, LogTypeTopup, 100, 200, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"topup-only-model"}, models)
}

func TestGetDistinctLogModelNames_NoTimeWindowReturnsAllConsume(t *testing.T) {
	cleanup := seedDistinctModelLogs(t)
	defer cleanup()

	// No time bounds: out-of-range consume model is now included too.
	models, err := GetDistinctLogModelNames(1, 0, 0, 0, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-5.6", "out-of-range", "seedance-2"}, models)
}

func TestGetDistinctLogModelNames_EmptyResult(t *testing.T) {
	cleanup := seedDistinctModelLogs(t)
	defer cleanup()

	// Window with no matching rows returns an empty (non-nil) slice.
	models, err := GetDistinctLogModelNames(1, 0, 1000, 2000, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Empty(t, models)
}

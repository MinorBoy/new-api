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

// seedTokenUsageLogs inserts consume rows for several token_ids so we can
// verify SumUsedQuotaByTokens sums per token, ignores non-consume types, and
// respects the time window. Returns a cleanup that removes the seeded rows.
func seedTokenUsageLogs(t *testing.T) func() {
	t.Helper()
	rows := []*Log{
		// Token 10: 100 + 50 = 150 within [100,200], 30 outside the window.
		{TokenId: 10, Type: LogTypeConsume, Quota: 100, CreatedAt: 110},
		{TokenId: 10, Type: LogTypeConsume, Quota: 50, CreatedAt: 150},
		{TokenId: 10, Type: LogTypeConsume, Quota: 30, CreatedAt: 300},
		// Token 11: single consume row.
		{TokenId: 11, Type: LogTypeConsume, Quota: 7, CreatedAt: 120},
		// Token 12: a top-up row that must be excluded.
		{TokenId: 12, Type: LogTypeTopup, Quota: 999, CreatedAt: 120},
		// Token 13: consume but outside the window — excluded when bounded.
		{TokenId: 13, Type: LogTypeConsume, Quota: 5, CreatedAt: 400},
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

func TestSumUsedQuotaByTokens_BoundedWindow(t *testing.T) {
	cleanup := seedTokenUsageLogs(t)
	defer cleanup()

	// Window [100, 200]: token 10 sums only the in-window rows (150),
	// token 11 included (7), token 12 excluded (top-up), token 13 out of range.
	usage, err := SumUsedQuotaByTokens([]int{10, 11, 12, 13}, 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 150, usage[10])
	assert.Equal(t, 7, usage[11])
	assert.NotContains(t, usage, 12, "top-up rows must be excluded")
	assert.NotContains(t, usage, 13, "out-of-window rows must be excluded")
}

func TestSumUsedQuotaByTokens_NoWindowIncludesAllConsume(t *testing.T) {
	cleanup := seedTokenUsageLogs(t)
	defer cleanup()

	// No window: token 10 = 100+50+30 = 180, token 13 = 5 now included.
	usage, err := SumUsedQuotaByTokens([]int{10, 13}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 180, usage[10])
	assert.Equal(t, 5, usage[13])
}

func TestSumUsedQuotaByTokens_EmptyInput(t *testing.T) {
	usage, err := SumUsedQuotaByTokens(nil, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, usage)
}

func TestSumUsedQuotaByTokens_NoMatchingRows(t *testing.T) {
	cleanup := seedTokenUsageLogs(t)
	defer cleanup()

	// Token ids that exist in no consume row: result map is empty (missing key = 0).
	usage, err := SumUsedQuotaByTokens([]int{9999}, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, usage)
}

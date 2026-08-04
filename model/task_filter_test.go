package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTaskFilterTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Task{}))

	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
	})
}

func TestTaskQueryFiltersRequestModelAndUserBeforePagination(t *testing.T) {
	setupTaskFilterTestDB(t)

	tasks := []Task{
		{TaskID: "model-a-new", UserId: 10, SubmitTime: 300, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-a"}},
		{TaskID: "model-b", UserId: 10, SubmitTime: 250, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-b"}},
		{TaskID: "other-user", UserId: 11, SubmitTime: 200, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-a"}},
		{TaskID: "model-a-old", UserId: 10, SubmitTime: 100, Status: TaskStatusFailure, Properties: Properties{OriginModelName: "model-a"}},
	}
	require.NoError(t, DB.Create(&tasks).Error)

	query := SyncTaskQueryParams{UserID: "10", RequestModel: "model-a"}
	page := TaskGetAllTasks(1, 1, query)
	require.Len(t, page, 1)
	assert.Equal(t, "model-a-new", page[0].TaskID)
	assert.Equal(t, int64(2), TaskCountAllTasks(query))
}

func TestUserTaskQueryFiltersRequestModelBeforePagination(t *testing.T) {
	setupTaskFilterTestDB(t)

	tasks := []Task{
		{TaskID: "model-a-new", UserId: 10, SubmitTime: 300, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-a"}},
		{TaskID: "model-b", UserId: 10, SubmitTime: 250, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-b"}},
		{TaskID: "model-a-old", UserId: 10, SubmitTime: 100, Status: TaskStatusFailure, Properties: Properties{OriginModelName: "model-a"}},
	}
	require.NoError(t, DB.Create(&tasks).Error)

	query := SyncTaskQueryParams{RequestModel: "model-a"}
	page := TaskGetAllUserTask(10, 1, 1, query)
	require.Len(t, page, 1)
	assert.Equal(t, "model-a-new", page[0].TaskID)
	assert.Equal(t, int64(2), TaskCountAllUserTask(10, query))
}

func TestTaskFilterOptionsUseAllTasksInTimeRange(t *testing.T) {
	setupTaskFilterTestDB(t)

	tasks := []Task{
		{TaskID: "range-a", UserId: 10, ChannelId: 40, SubmitTime: 200, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-b"}},
		{TaskID: "range-b", UserId: 11, ChannelId: 29, SubmitTime: 250, Status: TaskStatusFailure, Properties: Properties{OriginModelName: "model-a"}},
		{TaskID: "duplicate", UserId: 10, ChannelId: 40, SubmitTime: 300, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-b"}},
		{TaskID: "legacy-empty-status", UserId: 10, ChannelId: 40, SubmitTime: 350, Status: "", Properties: Properties{OriginModelName: "model-b"}},
		{TaskID: "before-range", UserId: 12, ChannelId: 99, SubmitTime: 99, Status: TaskStatusQueued, Properties: Properties{OriginModelName: "model-z"}},
		{TaskID: "after-range", UserId: 13, ChannelId: 98, SubmitTime: 401, Status: TaskStatusInProgress, Properties: Properties{OriginModelName: "model-y"}},
	}
	require.NoError(t, DB.Create(&tasks).Error)

	options, err := GetTaskFilterOptions(0, 100, 400, true)
	require.NoError(t, err)
	assert.Equal(t, []int{29, 40}, options.ChannelIDs)
	assert.Equal(t, []TaskStatus{TaskStatusFailure, TaskStatusSuccess}, options.Statuses)
	assert.Equal(t, []string{"model-a", "model-b"}, options.RequestModels)
	assert.Equal(t, []int{10, 11}, options.UserIDs)
}

func TestTaskFilterOptionsRespectUserScope(t *testing.T) {
	setupTaskFilterTestDB(t)

	tasks := []Task{
		{TaskID: "mine", UserId: 10, ChannelId: 40, SubmitTime: 200, Status: TaskStatusSuccess, Properties: Properties{OriginModelName: "model-a"}},
		{TaskID: "other", UserId: 11, ChannelId: 29, SubmitTime: 250, Status: TaskStatusFailure, Properties: Properties{OriginModelName: "model-b"}},
	}
	require.NoError(t, DB.Create(&tasks).Error)

	options, err := GetTaskFilterOptions(10, 100, 300, false)
	require.NoError(t, err)
	assert.Empty(t, options.ChannelIDs)
	assert.Equal(t, []TaskStatus{TaskStatusSuccess}, options.Statuses)
	assert.Equal(t, []string{"model-a"}, options.RequestModels)
	assert.Empty(t, options.UserIDs)
}

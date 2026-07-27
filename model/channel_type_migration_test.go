package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateSecondaryChannelTypeIDsPreservesExistingChannelAndTaskSemantics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Task{}, &Option{}))

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	oldTypes := []int{59, 60, 61, 62, 63, 64, 65, 66}
	for index, oldType := range oldTypes {
		channel := &Channel{Id: index + 1, Type: oldType, Name: "secondary", Status: common.ChannelStatusManuallyDisabled}
		require.NoError(t, db.Create(channel).Error)
		task := &Task{TaskID: "task-" + strconv.Itoa(oldType), Platform: constant.TaskPlatform(strconv.Itoa(oldType))}
		require.NoError(t, db.Create(task).Error)
	}

	require.NoError(t, migrateSecondaryChannelTypeIDs())

	var channels []Channel
	require.NoError(t, db.Order("id").Find(&channels).Error)
	assert.Equal(t, []int{61, 62, 63, 64, 65, 66, 67, 68}, []int{
		channels[0].Type,
		channels[1].Type,
		channels[2].Type,
		channels[3].Type,
		channels[4].Type,
		channels[5].Type,
		channels[6].Type,
		channels[7].Type,
	})

	var tasks []Task
	require.NoError(t, db.Order("id").Find(&tasks).Error)
	assert.Equal(t, []constant.TaskPlatform{
		"61", "62", "63", "64", "65", "66", "67", "68",
	}, []constant.TaskPlatform{
		tasks[0].Platform,
		tasks[1].Platform,
		tasks[2].Platform,
		tasks[3].Platform,
		tasks[4].Platform,
		tasks[5].Platform,
		tasks[6].Platform,
		tasks[7].Platform,
	})

	var marker Option
	require.NoError(t, db.Where(&Option{Key: secondaryChannelTypeMigrationMarker}).First(&marker).Error)
	assert.Equal(t, "complete", marker.Value)

	require.NoError(t, migrateChannelTypeIDs())
	require.NoError(t, migrateChannelTypeIDs())

	require.NoError(t, db.Order("id").Find(&channels).Error)
	assert.Equal(t, []int{200, 201, 202, 203, 204, 205, 206, 207}, []int{
		channels[0].Type,
		channels[1].Type,
		channels[2].Type,
		channels[3].Type,
		channels[4].Type,
		channels[5].Type,
		channels[6].Type,
		channels[7].Type,
	})

	require.NoError(t, db.Order("id").Find(&tasks).Error)
	assert.Equal(t, []constant.TaskPlatform{
		"200", "201", "202", "203", "204", "205", "206", "207",
	}, []constant.TaskPlatform{
		tasks[0].Platform,
		tasks[1].Platform,
		tasks[2].Platform,
		tasks[3].Platform,
		tasks[4].Platform,
		tasks[5].Platform,
		tasks[6].Platform,
		tasks[7].Platform,
	})

	var ysrMarker Option
	require.NoError(t, db.Where(&Option{Key: ysrChannelTypeMigrationMarker}).First(&ysrMarker).Error)
	assert.Equal(t, "complete", ysrMarker.Value)
}

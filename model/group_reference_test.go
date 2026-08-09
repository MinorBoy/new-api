package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFindGroupReferencesCountsUsersAndTokens(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, db.Create(&User{Username: "paused-user", Group: "paused"}).Error)
	require.NoError(t, db.Create(&Token{UserId: 1, Key: "paused-key", Name: "paused-token", Group: "paused"}).Error)
	require.NoError(t, db.Create(&Token{UserId: 1, Key: "active-key", Name: "active-token", Group: "active"}).Error)

	references, err := FindGroupReferences(db, []string{"paused"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), references.Users)
	assert.Equal(t, int64(1), references.Tokens)
}

func TestFindGroupReferencesHandlesEmptyGroups(t *testing.T) {
	references, err := FindGroupReferences(nil, nil)
	require.NoError(t, err)
	assert.Zero(t, references.Users)
	assert.Zero(t, references.Tokens)
}

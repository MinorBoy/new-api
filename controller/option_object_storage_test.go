package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOptionsFiltersObjectStorageSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := common.OptionMap
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		"object_storage.secret_access_key": "super-secret",
		"object_storage.access_key_id":     "public-id",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = original
		common.OptionMapRWMutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	GetOptions(ctx)

	require.Equal(t, 200, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "object_storage.secret_access_key")
	assert.Contains(t, recorder.Body.String(), "object_storage.access_key_id")
}

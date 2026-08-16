package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	object_storage "github.com/QuantumNous/new-api/setting/object_storage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureObjectStorageControllerTest(t *testing.T, values map[string]string) {
	t.Helper()
	cfg := config.GlobalConfig.Get(object_storage.ConfigName)
	require.NotNil(t, cfg)
	original, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, original))
		object_storage.UpdateAndSync()
	})
	require.NoError(t, config.UpdateConfigFromMap(cfg, values))
	object_storage.UpdateAndSync()
}

func objectStorageRequest(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if method == http.MethodGet {
		GetObjectStorageSettings(ctx)
	} else if method == http.MethodPut {
		UpdateObjectStorageSettings(ctx)
	} else {
		TestObjectStorageSettings(ctx)
	}
	return recorder
}

func TestGetObjectStorageSettingsOmitsSecret(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":           "true",
		"endpoint":          "https://s3.example.com",
		"public_endpoint":   "https://cdn.example.com",
		"region":            "us-east-1",
		"bucket":            "videos",
		"access_key_id":     "access",
		"secret_access_key": "super-secret",
	})

	recorder := objectStorageRequest(t, http.MethodGet, "/api/object-storage/settings", "")
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "super-secret")
	assert.Contains(t, recorder.Body.String(), "secret_configured")
	assert.Contains(t, recorder.Body.String(), "access")
}

func TestGetObjectStorageSettingsReturnsEmptyDomainListsAsArrays(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":           "false",
		"region":            "us-east-1",
		"max_video_size_mb": "512",
		"expires_seconds":   "86400",
	})

	recorder := objectStorageRequest(t, http.MethodGet, "/api/object-storage/settings", "")
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"transfer_domain_whitelist":[]`)
	assert.Contains(t, recorder.Body.String(), `"no_transfer_domain_blacklist":[]`)
}

func TestGetObjectStorageSettingsReturnsTransferControls(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":                      "false",
		"region":                       "us-east-1",
		"max_video_size_mb":            "512",
		"expires_seconds":              "86400",
		"transfer_mode":                object_storage.TransferModeRules,
		"whitelist_enabled":            "true",
		"blacklist_enabled":            "false",
		"rules_default_transfer":       "true",
		"transfer_domain_whitelist":    `["provider.example.com"]`,
		"no_transfer_domain_blacklist": `[]`,
	})

	recorder := objectStorageRequest(t, http.MethodGet, "/api/object-storage/settings", "")
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"transfer_mode":"rules"`)
	assert.Contains(t, recorder.Body.String(), `"whitelist_enabled":true`)
	assert.Contains(t, recorder.Body.String(), `"blacklist_enabled":false`)
	assert.Contains(t, recorder.Body.String(), `"rules_default_transfer":true`)
}

func TestUpdateObjectStorageSettingsKeepsSecretWhenInputIsBlank(t *testing.T) {
	db := setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":           "true",
		"endpoint":          "https://s3.example.com",
		"public_endpoint":   "https://cdn.example.com",
		"region":            "us-east-1",
		"bucket":            "videos",
		"access_key_id":     "access",
		"secret_access_key": "original-secret",
	})

	body := `{"enabled":true,"endpoint":"https://s3.example.com","public_endpoint":"https://cdn.example.com","region":"us-east-1","bucket":"videos","access_key_id":"access","secret_access_key":""}`
	recorder := objectStorageRequest(t, http.MethodPut, "/api/object-storage/settings", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, object_storage.Runtime().Enabled)
	assert.Equal(t, "original-secret", object_storage.Runtime().SecretAccessKey)
	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "object_storage.secret_access_key").Error)
	assert.Equal(t, "original-secret", option.Value)
}

func TestUpdateObjectStorageSettingsClearsSecretExplicitly(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":           "true",
		"endpoint":          "https://s3.example.com",
		"public_endpoint":   "https://cdn.example.com",
		"region":            "us-east-1",
		"bucket":            "videos",
		"access_key_id":     "access",
		"secret_access_key": "original-secret",
	})

	body := `{"enabled":false,"endpoint":"https://s3.example.com","public_endpoint":"https://cdn.example.com","region":"us-east-1","bucket":"videos","access_key_id":"access","secret_access_key":"","clear_secret":true}`
	recorder := objectStorageRequest(t, http.MethodPut, "/api/object-storage/settings", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, object_storage.Runtime().SecretAccessKey)
}

func TestUpdateObjectStorageSettingsPersistsTransferControls(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":           "false",
		"region":            "us-east-1",
		"max_video_size_mb": "512",
		"expires_seconds":   "86400",
	})

	body := `{"enabled":false,"region":"us-east-1","max_video_size_mb":512,"expires_seconds":86400,"transfer_mode":"all","whitelist_enabled":true,"blacklist_enabled":false,"rules_default_transfer":true,"transfer_domain_whitelist":["provider.example.com"],"no_transfer_domain_blacklist":[]}`
	recorder := objectStorageRequest(t, http.MethodPut, "/api/object-storage/settings", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, object_storage.TransferModeAll, object_storage.Runtime().TransferMode)
	assert.True(t, object_storage.Runtime().WhitelistEnabled)
	assert.False(t, object_storage.Runtime().BlacklistEnabled)
	assert.True(t, object_storage.Runtime().RulesDefaultTransfer)
	assert.Contains(t, recorder.Body.String(), `"transfer_mode":"all"`)
	assert.Contains(t, recorder.Body.String(), `"whitelist_enabled":true`)
}

func TestUpdateObjectStorageSettingsPreservesTransferControlsWhenOmitted(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	configureObjectStorageControllerTest(t, map[string]string{
		"enabled":                      "false",
		"region":                       "us-east-1",
		"max_video_size_mb":            "512",
		"expires_seconds":              "86400",
		"transfer_mode":                object_storage.TransferModeAll,
		"whitelist_enabled":            "true",
		"blacklist_enabled":            "true",
		"rules_default_transfer":       "true",
		"transfer_domain_whitelist":    `["provider.example.com"]`,
		"no_transfer_domain_blacklist": `["official.example.com"]`,
	})

	body := `{"enabled":false,"endpoint":"https://new-s3.example.com","region":"us-east-1","max_video_size_mb":512,"expires_seconds":86400,"transfer_domain_whitelist":["provider.example.com"],"no_transfer_domain_blacklist":["official.example.com"]}`
	recorder := objectStorageRequest(t, http.MethodPut, "/api/object-storage/settings", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, object_storage.TransferModeAll, object_storage.Runtime().TransferMode)
	assert.True(t, object_storage.Runtime().WhitelistEnabled)
	assert.True(t, object_storage.Runtime().BlacklistEnabled)
	assert.True(t, object_storage.Runtime().RulesDefaultTransfer)
}

func TestUpdateObjectStorageSettingsRejectsInvalidEnabledConfig(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	original := object_storage.Runtime()
	body := `{"enabled":true,"endpoint":"","public_endpoint":"https://cdn.example.com","region":"us-east-1","bucket":"videos","access_key_id":"access","secret_access_key":"secret","expires_seconds":59}`
	recorder := objectStorageRequest(t, http.MethodPut, "/api/object-storage/settings", body)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.False(t, object_storage.Runtime().Enabled)
	assert.Equal(t, original.Endpoint, object_storage.Runtime().Endpoint)
}

func TestTestObjectStorageSettingsProbesWithoutSaving(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	body := fmt.Sprintf(`{"enabled":true,"endpoint":%q,"public_endpoint":%q,"region":"us-east-1","bucket":"videos","access_key_id":"access","secret_access_key":"secret","use_path_style":true}`, server.URL, server.URL)
	recorder := objectStorageRequest(t, http.MethodPost, "/api/object-storage/test", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "success")
	assert.Equal(t, []string{"PUT", "HEAD", "DELETE"}, methods)
}

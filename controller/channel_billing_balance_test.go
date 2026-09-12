package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSub2APIBalanceUsesUsageFallbacks(t *testing.T) {
	setupModelListControllerTestDB(t)

	var gotPath string
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"quota":{"remaining":12.5,"unit":"USD"},"is_active":true}`))
	}))
	defer server.Close()

	channel := &model.Channel{Type: constant.ChannelTypeSub2API, Key: "sub-key", BaseURL: common.GetPointer(server.URL)}
	balance, err := updateChannelBalance(channel)

	require.NoError(t, err)
	assert.Equal(t, 12.5, balance)
	assert.Equal(t, "/v1/usage", gotPath)
	assert.Equal(t, "Bearer sub-key", gotAuthorization)
}

func TestUpdateNewAPIBalanceUsesQuotaConversionAndUserHeader(t *testing.T) {
	setupModelListControllerTestDB(t)

	var gotRequest *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequest = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"group":"pro","quota":1250000,"used_quota":250000}}`))
	}))
	defer server.Close()

	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, Key: "new-key", Other: "42", BaseURL: common.GetPointer(server.URL)}
	balance, err := updateChannelBalance(channel)

	require.NoError(t, err)
	assert.Equal(t, 2.5, balance)
	require.NotNil(t, gotRequest)
	assert.Equal(t, "/api/user/self", gotRequest.URL.Path)
	assert.Equal(t, "Bearer new-key", gotRequest.Header.Get("Authorization"))
	assert.Equal(t, "application/json", gotRequest.Header.Get("Content-Type"))
	assert.Equal(t, "cc-switch/1.0", gotRequest.Header.Get("User-Agent"))
	assert.Equal(t, "42", gotRequest.Header.Get("New-Api-User"))
}

func TestUpdateNewAPIBalanceRejectsInvalidResponse(t *testing.T) {
	setupModelListControllerTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"invalid token"}`))
	}))
	defer server.Close()

	channel := &model.Channel{Type: constant.ChannelTypeNewAPI, Key: "new-key", Other: "42", BaseURL: common.GetPointer(server.URL)}
	_, err := updateChannelNewAPIBalance(channel)

	require.EqualError(t, err, "invalid token")
}

func TestUpdateNewAPIBalanceRequiresUserID(t *testing.T) {
	setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:    constant.ChannelTypeNewAPI,
		Key:     "new-key",
		BaseURL: common.GetPointer("https://upstream.example.com"),
	}

	_, err := updateChannelNewAPIBalance(channel)
	require.EqualError(t, err, "New API 渠道缺少上游用户 ID")
}

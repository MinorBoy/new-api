package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureSSRFTestFetchSetting(t *testing.T) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() {
		*fetchSetting = original
	})

	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = false
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpList = nil
	fetchSetting.AllowedPorts = []string{"80", "443"}
	fetchSetting.ApplyIPFilterForDomain = true
}

func TestGetSSRFProtectedHTTPClientFallsBackToDefaultClientWhenProtectionDisabled(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	originalHTTPClient := httpClient
	originalProtectedClient := ssrfProtectedHTTPClient
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		httpClient = originalHTTPClient
		ssrfProtectedHTTPClient = originalProtectedClient
	})

	fetchSetting.EnableSSRFProtection = false
	expected := &http.Client{}
	httpClient = expected
	ssrfProtectedHTTPClient = &http.Client{}

	assert.Same(t, expected, GetSSRFProtectedHTTPClient())
}

func TestProtectedFetchAdapterRejectsPrivateTargetBeforeProxy(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	proxyURL, err := url.Parse("http://127.0.0.1:3128")
	require.NoError(t, err)
	var dialed []string
	client := newProtectedFetchHTTPClientWithProxy(
		nil,
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return nil, errors.New("proxy should not be dialed")
		},
		func() (*common.SSRFProtection, bool, error) {
			return currentFetchProtection()
		},
		func(*http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
	)
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/resource", nil)
	require.NoError(t, err)

	response, err := client.Do(request)

	require.Error(t, err)
	assert.Nil(t, response)
	assert.Empty(t, dialed)
}

func TestProtectedFetchAdapterUsesConfiguredProxyForPublicTarget(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	proxyURL, err := url.Parse("http://127.0.0.1:3128")
	require.NoError(t, err)
	var dialed []string
	client := newProtectedFetchHTTPClientWithProxy(
		nil,
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return nil, errors.New("stop after proxy dial")
		},
		currentFetchProtection,
		func(*http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
	)
	request, err := http.NewRequest(http.MethodGet, "http://93.184.216.34/resource", nil)
	require.NoError(t, err)

	response, err := client.Do(request)

	require.Error(t, err)
	assert.Nil(t, response)
	assert.Equal(t, []string{"127.0.0.1:3128"}, dialed)
}

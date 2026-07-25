package ssrffetch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticResolver map[string][]net.IPAddr

func (r staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if addresses, ok := r[host]; ok {
		return addresses, nil
	}
	return nil, fmt.Errorf("unexpected lookup for %s", host)
}

func enabledProtection(protection *common.SSRFProtection) func() (*common.SSRFProtection, bool, error) {
	return func() (*common.SSRFProtection, bool, error) {
		return protection, true, nil
	}
}

func publicOnlyProtection() *common.SSRFProtection {
	return &common.SSRFProtection{
		AllowPrivateIp:         false,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		ApplyIPFilterForDomain: true,
	}
}

func pipeConnection(t *testing.T) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		require.NoError(t, server.Close())
	})
	return client
}

func TestProtectedDialerRejectsPrivateReboundAddress(t *testing.T) {
	dialer := &protectedDialer{
		resolver: staticResolver{
			"assets.example": {{IP: net.ParseIP("127.0.0.1")}},
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			require.FailNow(t, "blocked address must not be dialed")
			return nil, nil
		},
		getProtection: enabledProtection(publicOnlyProtection()),
	}

	connection, err := dialer.DialContext(context.Background(), "tcp", "assets.example:443")

	require.Error(t, err)
	assert.Nil(t, connection)
	assert.Contains(t, err.Error(), "private IP address not allowed")
}

func TestProtectedDialerRejectsMixedResolvedAddresses(t *testing.T) {
	var dialed []string
	dialer := &protectedDialer{
		resolver: staticResolver{
			"assets.example": {
				{IP: net.ParseIP("8.8.8.8")},
				{IP: net.ParseIP("10.0.0.1")},
			},
		},
		dialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return pipeConnection(t), nil
		},
		getProtection: enabledProtection(publicOnlyProtection()),
	}

	connection, err := dialer.DialContext(context.Background(), "tcp", "assets.example:443")

	require.Error(t, err)
	assert.Nil(t, connection)
	assert.Empty(t, dialed)
}

func TestProtectedDialerUsesValidatedIPAddress(t *testing.T) {
	var dialed []string
	dialer := &protectedDialer{
		resolver: staticResolver{
			"assets.example": {
				{IP: net.ParseIP("8.8.8.8")},
				{IP: net.ParseIP("1.1.1.1")},
			},
		},
		dialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return pipeConnection(t), nil
		},
		getProtection: enabledProtection(publicOnlyProtection()),
	}

	connection, err := dialer.DialContext(context.Background(), "tcp", "assets.example:443")

	require.NoError(t, err)
	require.NotNil(t, connection)
	assert.Equal(t, []string{"8.8.8.8:443"}, dialed)
}

func TestProtectedDialerUsesOriginalAddressWhenProtectionDisabled(t *testing.T) {
	var dialed []string
	dialer := &protectedDialer{
		resolver: staticResolver{},
		dialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return pipeConnection(t), nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return nil, false, nil
		},
	}

	connection, err := dialer.DialContext(context.Background(), "tcp", "assets.example:443")

	require.NoError(t, err)
	require.NotNil(t, connection)
	assert.Equal(t, []string{"assets.example:443"}, dialed)
}

func TestNewClientRejectsRequestBeforeProxyDial(t *testing.T) {
	var dialed []string
	client, err := NewClient(Options{
		Resolver: staticResolver{},
		DialContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return nil, errors.New("unexpected dial")
		},
		GetProtection: enabledProtection(publicOnlyProtection()),
		ValidateURL: func(string) error {
			return errors.New("target blocked")
		},
		Proxy: func(*http.Request) (*url.URL, error) {
			return url.Parse("http://127.0.0.1:3128")
		},
	})
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodGet, "https://assets.example/video.mp4", nil)
	require.NoError(t, err)

	response, err := client.Do(request)

	require.ErrorContains(t, err, "target blocked")
	assert.Nil(t, response)
	assert.Empty(t, dialed)
}

func TestNewClientRevalidatesEveryRedirect(t *testing.T) {
	var validated []string
	client, err := NewClient(Options{
		GetProtection: enabledProtection(publicOnlyProtection()),
		ValidateURL: func(rawURL string) error {
			validated = append(validated, rawURL)
			if rawURL == "https://blocked.example/private.mp4" {
				return errors.New("redirect blocked")
			}
			return nil
		},
	})
	require.NoError(t, err)
	redirect, err := http.NewRequest(http.MethodGet, "https://blocked.example/private.mp4", nil)
	require.NoError(t, err)
	original, err := http.NewRequest(http.MethodGet, "https://assets.example/video.mp4", nil)
	require.NoError(t, err)

	err = client.CheckRedirect(redirect, []*http.Request{original})

	require.ErrorContains(t, err, "redirect blocked")
	assert.Equal(t, []string{"https://blocked.example/private.mp4"}, validated)
}

func TestProtectedRoundTripperCachesOnlyByProxyAddress(t *testing.T) {
	client, err := NewClient(Options{
		GetProtection: enabledProtection(publicOnlyProtection()),
		ValidateURL:   func(string) error { return nil },
	})
	require.NoError(t, err)
	roundTripper, ok := client.Transport.(*protectedRoundTripper)
	require.True(t, ok)
	proxyURL, err := url.Parse("http://127.0.0.1:3128")
	require.NoError(t, err)

	direct := roundTripper.transportFor(nil)
	directAgain := roundTripper.transportFor(nil)
	proxied := roundTripper.transportFor(proxyURL)
	proxiedAgain := roundTripper.transportFor(proxyURL)

	assert.Same(t, direct, directAgain)
	assert.Same(t, proxied, proxiedAgain)
	assert.NotSame(t, direct, proxied)
}

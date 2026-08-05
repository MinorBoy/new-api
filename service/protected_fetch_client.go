package service

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/ssrffetch"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func currentFetchProtection() (*common.SSRFProtection, bool, error) {
	fetchSetting := system_setting.GetFetchSetting()
	if !fetchSetting.EnableSSRFProtection {
		return nil, false, nil
	}

	protection, err := common.NewSSRFProtectionFromFetchSetting(
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
	if err != nil {
		return nil, true, err
	}
	return protection, true, nil
}

func newProtectedFetchHTTPClient() *http.Client {
	return newProtectedFetchHTTPClientWithDialer(nil, nil, nil)
}

func newDirectProtectedFetchHTTPClient() *http.Client {
	return newDirectProtectedFetchHTTPClientWithDialer(nil, nil, nil)
}

func newDirectProtectedFetchHTTPClientWithDialer(
	resolver ssrffetch.Resolver,
	dialContext func(context.Context, string, string) (net.Conn, error),
	getProtection func() (*common.SSRFProtection, bool, error),
) *http.Client {
	return newProtectedFetchHTTPClientWithProxy(
		resolver,
		dialContext,
		getProtection,
		func(*http.Request) (*url.URL, error) { return nil, nil },
	)
}

func newProtectedFetchHTTPClientWithDialer(
	resolver ssrffetch.Resolver,
	dialContext func(context.Context, string, string) (net.Conn, error),
	getProtection func() (*common.SSRFProtection, bool, error),
) *http.Client {
	return newProtectedFetchHTTPClientWithProxy(resolver, dialContext, getProtection, http.ProxyFromEnvironment)
}

func newProtectedFetchHTTPClientWithProxy(
	resolver ssrffetch.Resolver,
	dialContext func(context.Context, string, string) (net.Conn, error),
	getProtection func() (*common.SSRFProtection, bool, error),
	proxy func(*http.Request) (*url.URL, error),
) *http.Client {
	if getProtection == nil {
		getProtection = currentFetchProtection
	}
	timeout := time.Duration(0)
	if common.RelayTimeout != 0 {
		timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	var tlsConfig = common.InsecureTLSConfig
	if !common.TLSInsecureSkipVerify {
		tlsConfig = nil
	}
	client, err := ssrffetch.NewClient(ssrffetch.Options{
		Resolver:            resolver,
		DialContext:         dialContext,
		GetProtection:       getProtection,
		ValidateURL:         ValidateSSRFProtectedFetchURL,
		Proxy:               proxy,
		Timeout:             timeout,
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(common.RelayIdleConnTimeout) * time.Second,
		TLSConfig:           tlsConfig,
		MaxRedirects:        10,
	})
	if err != nil {
		panic(err)
	}
	return client
}

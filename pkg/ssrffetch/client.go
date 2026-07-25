package ssrffetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type Options struct {
	Resolver            Resolver
	DialContext         func(context.Context, string, string) (net.Conn, error)
	GetProtection       func() (*common.SSRFProtection, bool, error)
	ValidateURL         func(string) error
	Proxy               func(*http.Request) (*url.URL, error)
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	TLSConfig           *tls.Config
	MaxRedirects        int
}

func NewClient(options Options) (*http.Client, error) {
	if options.GetProtection == nil {
		return nil, errors.New("SSRF protection provider is required")
	}
	if options.ValidateURL == nil {
		return nil, errors.New("SSRF URL validator is required")
	}
	if options.Resolver == nil {
		options.Resolver = net.DefaultResolver
	}
	if options.DialContext == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		options.DialContext = dialer.DialContext
	}
	if options.Proxy == nil {
		options.Proxy = http.ProxyFromEnvironment
	}
	if options.MaxIdleConns <= 0 {
		options.MaxIdleConns = 100
	}
	if options.MaxIdleConnsPerHost <= 0 {
		options.MaxIdleConnsPerHost = 10
	}
	if options.IdleConnTimeout <= 0 {
		options.IdleConnTimeout = 90 * time.Second
	}
	if options.MaxRedirects <= 0 {
		options.MaxRedirects = 10
	}

	roundTripper := &protectedRoundTripper{
		resolver:            options.Resolver,
		dialContext:         options.DialContext,
		getProtection:       options.GetProtection,
		validateURL:         options.ValidateURL,
		proxy:               options.Proxy,
		maxIdleConns:        options.MaxIdleConns,
		maxIdleConnsPerHost: options.MaxIdleConnsPerHost,
		idleConnTimeout:     options.IdleConnTimeout,
		tlsConfig:           options.TLSConfig,
		transports:          make(map[string]*http.Transport),
	}
	return &http.Client{
		Transport: roundTripper,
		Timeout:   options.Timeout,
		CheckRedirect: func(request *http.Request, previous []*http.Request) error {
			if request == nil || request.URL == nil {
				return errors.New("redirect request is invalid")
			}
			if err := options.ValidateURL(request.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			if len(previous) >= options.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", options.MaxRedirects)
			}
			return nil
		},
	}, nil
}

type protectedDialer struct {
	resolver      Resolver
	dialContext   func(context.Context, string, string) (net.Conn, error)
	getProtection func() (*common.SSRFProtection, bool, error)
}

func (d *protectedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	protection, enabled, err := d.getProtection()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return d.dialContext(ctx, network, address)
	}
	if protection == nil {
		return nil, errors.New("SSRF protection is enabled without a policy")
	}

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid dial address: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, errors.New("invalid dial port")
	}
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		return d.dialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
	}
	if !protection.ApplyIPFilterForDomain {
		return d.dialContext(ctx, network, address)
	}

	resolved, err := d.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.New("DNS resolution failed")
	}
	candidates := make([]net.IP, 0, len(resolved))
	for _, resolvedAddress := range resolved {
		ip := resolvedAddress.IP
		if ip == nil || !networkAllowsIP(network, ip) {
			continue
		}
		if err := protection.ValidateResolvedIP(host, ip); err != nil {
			return nil, err
		}
		candidates = append(candidates, ip)
	}

	var lastDialError error
	for _, ip := range candidates {
		connection, dialErr := d.dialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
		if dialErr == nil {
			return connection, nil
		}
		lastDialError = dialErr
	}
	if lastDialError != nil {
		return nil, lastDialError
	}
	return nil, errors.New("DNS resolution returned no usable IP addresses")
}

func networkAllowsIP(network string, ip net.IP) bool {
	switch network {
	case "tcp4":
		return ip.To4() != nil
	case "tcp6":
		return ip.To4() == nil
	default:
		return true
	}
}

type protectedRoundTripper struct {
	resolver            Resolver
	dialContext         func(context.Context, string, string) (net.Conn, error)
	getProtection       func() (*common.SSRFProtection, bool, error)
	validateURL         func(string) error
	proxy               func(*http.Request) (*url.URL, error)
	maxIdleConns        int
	maxIdleConnsPerHost int
	idleConnTimeout     time.Duration
	tlsConfig           *tls.Config

	mutex      sync.Mutex
	transports map[string]*http.Transport
}

func (t *protectedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("request is invalid")
	}
	if err := t.validateURL(request.URL.String()); err != nil {
		return nil, err
	}
	proxyURL, err := t.proxy(request)
	if err != nil {
		return nil, err
	}
	return t.transportFor(proxyURL).RoundTrip(request)
}

func (t *protectedRoundTripper) CloseIdleConnections() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for _, transport := range t.transports {
		transport.CloseIdleConnections()
	}
}

func (t *protectedRoundTripper) transportFor(proxyURL *url.URL) *http.Transport {
	key := "direct"
	if proxyURL != nil {
		key = proxyURL.String()
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()
	if transport, ok := t.transports[key]; ok {
		return transport
	}

	dialContext := t.dialContext
	proxyFunction := http.ProxyURL(proxyURL)
	if proxyURL == nil {
		dialContext = (&protectedDialer{
			resolver:      t.resolver,
			dialContext:   t.dialContext,
			getProtection: t.getProtection,
		}).DialContext
		proxyFunction = nil
	}
	transport := &http.Transport{
		Proxy:               proxyFunction,
		DialContext:         dialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        t.maxIdleConns,
		MaxIdleConnsPerHost: t.maxIdleConnsPerHost,
		IdleConnTimeout:     t.idleConnTimeout,
	}
	if t.tlsConfig != nil {
		transport.TLSClientConfig = t.tlsConfig.Clone()
	}
	t.transports[key] = transport
	return transport
}

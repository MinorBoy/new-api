package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/ssrffetch"
	"github.com/QuantumNous/new-api/pkg/videometa"
)

type config struct {
	ListenAddr        string
	Token             string
	MaxBytes          int64
	Timeout           time.Duration
	MaxConcurrency    int
	CacheEntries      int
	CacheTTL          time.Duration
	SignedURLCacheTTL time.Duration
}

func main() {
	configuration, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := run(configuration); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() (config, error) {
	configuration := config{
		ListenAddr:        environmentOrDefault("VIDEO_METADATA_LISTEN_ADDR", ":8090"),
		Token:             strings.TrimSpace(os.Getenv("VIDEO_METADATA_SERVICE_TOKEN")),
		MaxBytes:          videometa.MaxVideoBytes,
		Timeout:           30 * time.Second,
		MaxConcurrency:    16,
		CacheEntries:      10_000,
		CacheTTL:          10 * time.Minute,
		SignedURLCacheTTL: time.Minute,
	}
	if configuration.Token == "" {
		return config{}, errors.New("VIDEO_METADATA_SERVICE_TOKEN is required")
	}
	if _, _, err := net.SplitHostPort(configuration.ListenAddr); err != nil {
		return config{}, fmt.Errorf("VIDEO_METADATA_LISTEN_ADDR is invalid: %w", err)
	}

	maxBytes, err := integerEnvironment("VIDEO_METADATA_MAX_BYTES", configuration.MaxBytes)
	if err != nil || maxBytes <= 0 || maxBytes > videometa.MaxVideoBytes {
		return config{}, errors.New("VIDEO_METADATA_MAX_BYTES must be between 1 and 134217728")
	}
	configuration.MaxBytes = maxBytes
	timeoutSeconds, err := integerEnvironment("VIDEO_METADATA_TIMEOUT_SECONDS", int64(configuration.Timeout/time.Second))
	if err != nil || timeoutSeconds <= 0 || timeoutSeconds > videometa.MaxDeadlineMS/1_000 {
		return config{}, errors.New("VIDEO_METADATA_TIMEOUT_SECONDS must be between 1 and 30")
	}
	configuration.Timeout = time.Duration(timeoutSeconds) * time.Second
	maxConcurrency, err := integerEnvironment("VIDEO_METADATA_MAX_CONCURRENCY", int64(configuration.MaxConcurrency))
	if err != nil || maxConcurrency <= 0 || maxConcurrency > 10_000 {
		return config{}, errors.New("VIDEO_METADATA_MAX_CONCURRENCY must be between 1 and 10000")
	}
	configuration.MaxConcurrency = int(maxConcurrency)
	cacheEntries, err := integerEnvironment("VIDEO_METADATA_CACHE_ENTRIES", int64(configuration.CacheEntries))
	if err != nil || cacheEntries < 0 || cacheEntries > 1_000_000 {
		return config{}, errors.New("VIDEO_METADATA_CACHE_ENTRIES must be between 0 and 1000000")
	}
	configuration.CacheEntries = int(cacheEntries)
	cacheTTLSeconds, err := integerEnvironment("VIDEO_METADATA_CACHE_TTL_SECONDS", int64(configuration.CacheTTL/time.Second))
	if err != nil || cacheTTLSeconds <= 0 || cacheTTLSeconds > 86_400 {
		return config{}, errors.New("VIDEO_METADATA_CACHE_TTL_SECONDS must be between 1 and 86400")
	}
	configuration.CacheTTL = time.Duration(cacheTTLSeconds) * time.Second
	signedTTLSeconds, err := integerEnvironment("VIDEO_METADATA_SIGNED_URL_CACHE_TTL_SECONDS", int64(configuration.SignedURLCacheTTL/time.Second))
	if err != nil || signedTTLSeconds <= 0 || signedTTLSeconds > cacheTTLSeconds {
		return config{}, errors.New("VIDEO_METADATA_SIGNED_URL_CACHE_TTL_SECONDS must be positive and no greater than the default cache TTL")
	}
	configuration.SignedURLCacheTTL = time.Duration(signedTTLSeconds) * time.Second
	return configuration, nil
}

func environmentOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func integerEnvironment(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, nil
}

func newMetadataProtection() (*common.SSRFProtection, error) {
	return common.NewSSRFProtectionFromFetchSetting(
		false,
		false,
		false,
		nil,
		nil,
		[]string{"80", "443"},
		true,
	)
}

func run(configuration config) error {
	protection, err := newMetadataProtection()
	if err != nil {
		return err
	}
	client, err := ssrffetch.NewClient(ssrffetch.Options{
		GetProtection: func() (*common.SSRFProtection, bool, error) {
			return protection, true, nil
		},
		ValidateURL:         protection.ValidateURL,
		Proxy:               http.ProxyFromEnvironment,
		Timeout:             configuration.Timeout,
		MaxIdleConns:        configuration.MaxConcurrency * 2,
		MaxIdleConnsPerHost: configuration.MaxConcurrency,
		IdleConnTimeout:     90 * time.Second,
		MaxRedirects:        10,
	})
	if err != nil {
		return err
	}
	cache := videometa.NewCache(configuration.CacheEntries)
	fetcher := videometa.NewFetcher(videometa.FetcherOptions{
		Client:            client,
		Cache:             cache,
		MaxBytes:          configuration.MaxBytes,
		CacheTTL:          configuration.CacheTTL,
		SignedURLCacheTTL: configuration.SignedURLCacheTTL,
	})
	handler := videometa.NewServer(videometa.ServerOptions{
		Token:          configuration.Token,
		MaxConcurrency: configuration.MaxConcurrency,
		Metadata:       fetcher.Metadata,
		Log: func(message string, fields map[string]any) {
			log.Printf("%s request_id=%v result_code=%v elapsed_ms=%v bytes=%v cache_hit=%v",
				message, fields["request_id"], fields["result_code"], fields["elapsed_ms"], fields["bytes"], fields["cache_hit"])
		},
	})
	server := &http.Server{
		Addr:              configuration.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       configuration.Timeout + 5*time.Second,
		WriteTimeout:      configuration.Timeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdownSignal.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return err
		}
		if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

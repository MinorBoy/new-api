package objectstorage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreUsesPathStyleAndContentType(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	var paths []string
	var contentTypes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
		mu.Unlock()
		if r.Method == http.MethodPut {
			assert.Equal(t, "video/mp4", r.Header.Get("Content-Type"))
			assert.Equal(t, "video-data", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store, err := New(Config{
		Endpoint:        server.URL,
		PublicEndpoint:  server.URL,
		Region:          "us-east-1",
		Bucket:          "videos",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		UsePathStyle:    true,
	})
	require.NoError(t, err)
	require.NoError(t, store.Put(context.Background(), "model/task.mp4", "video/mp4", strings.NewReader("video-data"), int64(len("video-data"))))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{http.MethodPut}, methods)
	assert.Equal(t, []string{"/videos/model/task.mp4"}, paths)
	assert.Equal(t, []string{"video/mp4"}, contentTypes)
}

func TestStoreExistsHandlesNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/missing.mp4" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, PublicEndpoint: server.URL, Region: "us-east-1", Bucket: "videos", AccessKeyID: "access", SecretAccessKey: "secret", UsePathStyle: true})
	require.NoError(t, err)

	exists, err := store.Exists(context.Background(), "present.mp4")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = store.Exists(context.Background(), "missing.mp4")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStoreProbePutsHeadsAndDeletes(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, PublicEndpoint: server.URL, Region: "us-east-1", Bucket: "videos", AccessKeyID: "access", SecretAccessKey: "secret", UsePathStyle: true})
	require.NoError(t, err)
	require.NoError(t, store.Probe(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{http.MethodPut, http.MethodHead, http.MethodDelete}, methods)
}

func TestStorePresignGetUsesConfiguredExpiry(t *testing.T) {
	store, err := New(Config{Endpoint: "https://s3.example.com", PublicEndpoint: "https://cdn.example.com", Region: "us-east-1", Bucket: "videos", AccessKeyID: "access", SecretAccessKey: "secret", UsePathStyle: true})
	require.NoError(t, err)
	url, err := store.PresignGet(context.Background(), "model/task.mp4", 86400*time.Second)
	require.NoError(t, err)
	assert.Contains(t, url, "X-Amz-Expires=86400")
	assert.NotContains(t, url, "secret")
}

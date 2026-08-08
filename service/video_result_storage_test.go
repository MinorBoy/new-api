package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/objectstorage"
	"github.com/QuantumNous/new-api/setting/config"
	object_storage "github.com/QuantumNous/new-api/setting/object_storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeVideoResultStore struct {
	exists      bool
	existsErr   error
	putErr      error
	presignURL  string
	putCalls    int
	deleteCalls int
	lastKey     string
	lastType    string
	lastBody    []byte
	lastSize    int64
	lastExpiry  time.Duration
}

func (s *fakeVideoResultStore) Put(_ context.Context, key, contentType string, body io.Reader, size int64) error {
	s.putCalls++
	s.lastKey = key
	s.lastType = contentType
	s.lastSize = size
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.lastBody = data
	if s.putErr != nil {
		return s.putErr
	}
	s.exists = true
	return nil
}

func (s *fakeVideoResultStore) Exists(context.Context, string) (bool, error) {
	return s.exists, s.existsErr
}

func (s *fakeVideoResultStore) Delete(context.Context, string) error {
	s.deleteCalls++
	return nil
}

func (s *fakeVideoResultStore) PresignGet(_ context.Context, key string, expires time.Duration) (string, error) {
	s.lastKey = key
	s.lastExpiry = expires
	return s.presignURL, nil
}

func configureVideoResultStorage(t *testing.T, values map[string]string) {
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

func installVideoResultDependencies(t *testing.T, store *fakeVideoResultStore, client *http.Client, validator func(string) error) {
	t.Helper()
	originalFactory := videoResultStoreFactory
	originalClient := videoResultHTTPClient
	originalValidator := videoResultURLValidator
	t.Cleanup(func() {
		videoResultStoreFactory = originalFactory
		videoResultHTTPClient = originalClient
		videoResultURLValidator = originalValidator
	})
	videoResultStoreFactory = func(objectstorage.Config) (videoResultObjectStore, error) { return store, nil }
	videoResultHTTPClient = func() *http.Client { return client }
	videoResultURLValidator = validator
}

func enabledVideoStorageValues(host string) map[string]string {
	return map[string]string{
		"enabled":                      "true",
		"endpoint":                     "https://s3.example.com",
		"public_endpoint":              "https://cdn.example.com",
		"region":                       "us-east-1",
		"bucket":                       "videos",
		"access_key_id":                "access",
		"secret_access_key":            "secret",
		"use_path_style":               "true",
		"max_video_size_mb":            "1",
		"expires_seconds":              "86400",
		"transfer_mode":                object_storage.TransferModeRules,
		"whitelist_enabled":            "true",
		"blacklist_enabled":            "false",
		"transfer_domain_whitelist":    fmt.Sprintf(`[%q]`, host),
		"no_transfer_domain_blacklist": `[]`,
	}
}

func sourceHost(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed.Hostname()
}

func testVideoTask() *model.Task {
	return &model.Task{
		TaskID: "task_public",
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-fast",
		},
	}
}

func TestProcessVideoResultTransfersWhitelistedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-data")
	}))
	t.Cleanup(server.Close)
	configureVideoResultStorage(t, enabledVideoStorageValues(sourceHost(t, server.URL)))
	store := &fakeVideoResultStore{}
	installVideoResultDependencies(t, store, server.Client(), func(string) error { return nil })
	task := testVideoTask()

	require.NoError(t, ProcessVideoResultURL(context.Background(), task, server.URL+"/result.mp4"))
	assert.Equal(t, "doubao-seedance-2-0-fast/task_public.mp4", task.PrivateData.ResultObjectKey)
	assert.Equal(t, "video/mp4", task.PrivateData.ResultObjectContentType)
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.Equal(t, 1, store.putCalls)
	assert.Equal(t, []byte("video-data"), store.lastBody)
}

func TestProcessVideoResultTransfersAllModeWithoutDomainMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-data")
	}))
	t.Cleanup(server.Close)
	values := enabledVideoStorageValues("unmatched.example")
	values["transfer_mode"] = object_storage.TransferModeAll
	values["whitelist_enabled"] = "false"
	values["transfer_domain_whitelist"] = `[]`
	configureVideoResultStorage(t, values)
	store := &fakeVideoResultStore{}
	installVideoResultDependencies(t, store, server.Client(), func(string) error { return nil })
	task := testVideoTask()

	require.NoError(t, ProcessVideoResultURL(context.Background(), task, server.URL+"/result.mp4"))
	assert.Equal(t, "doubao-seedance-2-0-fast/task_public.mp4", task.PrivateData.ResultObjectKey)
	assert.Equal(t, 1, store.putCalls)
}

func TestProcessVideoResultLeavesBlacklistedAndDefaultURLs(t *testing.T) {
	for _, tt := range []struct {
		name             string
		mode             string
		whitelistEnabled string
		blacklistEnabled string
		whitelist        string
		blacklist        string
	}{
		{"blacklist wins", object_storage.TransferModeRules, "true", "true", `["provider.example"]`, `["provider.example"]`},
		{"default mode", object_storage.TransferModeDefault, "true", "false", `["provider.example"]`, `[]`},
		{"rules disabled", object_storage.TransferModeRules, "false", "false", `["provider.example"]`, `[]`},
		{"blacklist only unmatched", object_storage.TransferModeRules, "false", "true", `[]`, `["other.example"]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			values := enabledVideoStorageValues("provider.example")
			values["transfer_mode"] = tt.mode
			values["whitelist_enabled"] = tt.whitelistEnabled
			values["blacklist_enabled"] = tt.blacklistEnabled
			values["transfer_domain_whitelist"] = tt.whitelist
			values["no_transfer_domain_blacklist"] = tt.blacklist
			configureVideoResultStorage(t, values)
			store := &fakeVideoResultStore{}
			installVideoResultDependencies(t, store, http.DefaultClient, func(string) error { return nil })
			task := testVideoTask()
			sourceURL := "https://provider.example/video.mp4"

			require.NoError(t, ProcessVideoResultURL(context.Background(), task, sourceURL))
			assert.Equal(t, sourceURL, task.PrivateData.ResultURL)
			assert.Empty(t, task.PrivateData.ResultObjectKey)
			assert.Zero(t, store.putCalls)
		})
	}
}

func TestProcessVideoResultRejectsOversizedVideoWithoutUpstreamFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa((1<<20)+1))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	configureVideoResultStorage(t, enabledVideoStorageValues(sourceHost(t, server.URL)))
	store := &fakeVideoResultStore{}
	installVideoResultDependencies(t, store, server.Client(), func(string) error { return nil })
	task := testVideoTask()

	err := ProcessVideoResultURL(context.Background(), task, server.URL+"/large.mp4")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "video_result_storage_error")
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.Zero(t, store.putCalls)
}

func TestProcessVideoResultTreatsExistingObjectAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "unused")
	}))
	t.Cleanup(server.Close)
	configureVideoResultStorage(t, enabledVideoStorageValues(sourceHost(t, server.URL)))
	store := &fakeVideoResultStore{exists: true}
	installVideoResultDependencies(t, store, server.Client(), func(string) error { return nil })
	task := testVideoTask()

	require.NoError(t, ProcessVideoResultURL(context.Background(), task, server.URL+"/result.mp4"))
	assert.Equal(t, "doubao-seedance-2-0-fast/task_public.mp4", task.PrivateData.ResultObjectKey)
	assert.Zero(t, store.putCalls)
}

func TestProcessVideoResultClassifiesSSRFAndMimeErrorsAsTerminalFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "not-video")
	}))
	t.Cleanup(server.Close)
	for _, tt := range []struct {
		name      string
		validator func(string) error
	}{
		{"ssrf", func(string) error { return errors.New("blocked") }},
		{"mime", func(string) error { return nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configureVideoResultStorage(t, enabledVideoStorageValues(sourceHost(t, server.URL)))
			store := &fakeVideoResultStore{}
			installVideoResultDependencies(t, store, server.Client(), tt.validator)
			task := testVideoTask()
			sourceURL := server.URL + "/result.mp4"

			err := ProcessVideoResultURL(context.Background(), task, sourceURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "video_result_storage_error")
			assert.NotContains(t, err.Error(), sourceHost(t, sourceURL))
			assert.Empty(t, task.PrivateData.ResultURL)
			assert.Zero(t, store.putCalls)
		})
	}
}

func TestProcessVideoResultUploadFailureDoesNotExposeSourceURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-data")
	}))
	t.Cleanup(server.Close)
	configureVideoResultStorage(t, enabledVideoStorageValues(sourceHost(t, server.URL)))
	store := &fakeVideoResultStore{putErr: errors.New("upload failed")}
	installVideoResultDependencies(t, store, server.Client(), func(string) error { return nil })
	task := testVideoTask()

	err := ProcessVideoResultURL(context.Background(), task, server.URL+"/result.mp4")
	require.Error(t, err)
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.Empty(t, task.PrivateData.ResultObjectKey)
}

func TestResolveTaskResultURLPresignsStoredObject(t *testing.T) {
	configureVideoResultStorage(t, enabledVideoStorageValues("provider.example"))
	store := &fakeVideoResultStore{presignURL: "https://cdn.example.com/videos/model/task.mp4?X-Amz-Expires=86400"}
	installVideoResultDependencies(t, store, http.DefaultClient, func(string) error { return nil })
	task := testVideoTask()
	task.PrivateData.ResultObjectKey = "model/task.mp4"

	resolved, err := ResolveTaskResultURL(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, store.presignURL, resolved)
	assert.Equal(t, 86400*time.Second, store.lastExpiry)
}

func TestResolveTaskResultURLKeepsLegacyURL(t *testing.T) {
	task := testVideoTask()
	task.PrivateData.ResultURL = "https://legacy.example/video.mp4"
	resolved, err := ResolveTaskResultURL(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, task.PrivateData.ResultURL, resolved)
}

func TestRewriteVideoResponseURLReplacesMetadataURL(t *testing.T) {
	configureVideoResultStorage(t, enabledVideoStorageValues("provider.example"))
	store := &fakeVideoResultStore{presignURL: "https://cdn.example.com/model/task.mp4?X-Amz-Expires=86400"}
	installVideoResultDependencies(t, store, http.DefaultClient, func(string) error { return nil })
	task := testVideoTask()
	task.PrivateData.ResultObjectKey = "model/task.mp4"

	body, err := RewriteVideoResponseURL(context.Background(), task, []byte(`{"metadata":{"url":"https://provider.example/original.mp4"}}`), VideoResponseFormatMetadataURL)
	require.NoError(t, err)
	assert.JSONEq(t, `{"metadata":{"url":"https://cdn.example.com/model/task.mp4?X-Amz-Expires=86400"}}`, string(body))
}

func TestRewriteVideoResponseURLReplacesVideoURL(t *testing.T) {
	configureVideoResultStorage(t, enabledVideoStorageValues("provider.example"))
	store := &fakeVideoResultStore{presignURL: "https://cdn.example.com/model/task.mp4?X-Amz-Expires=86400"}
	installVideoResultDependencies(t, store, http.DefaultClient, func(string) error { return nil })
	task := testVideoTask()
	task.PrivateData.ResultObjectKey = "model/task.mp4"

	body, err := RewriteVideoResponseURL(context.Background(), task, []byte(`{"content":{"video_url":"https://provider.example/original.mp4"}}`), VideoResponseFormatVideoURL)
	require.NoError(t, err)
	assert.JSONEq(t, `{"content":{"video_url":"https://cdn.example.com/model/task.mp4?X-Amz-Expires=86400"}}`, string(body))
}

var _ videoResultObjectStore = (*fakeVideoResultStore)(nil)

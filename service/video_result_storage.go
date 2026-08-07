package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/objectstorage"
	object_storage "github.com/QuantumNous/new-api/setting/object_storage"
)

type videoResultObjectStore interface {
	Put(context.Context, string, string, io.Reader, int64) error
	Exists(context.Context, string) (bool, error)
	Delete(context.Context, string) error
	PresignGet(context.Context, string, time.Duration) (string, error)
}

var videoResultStoreFactory = func(cfg objectstorage.Config) (videoResultObjectStore, error) {
	return objectstorage.New(cfg)
}

var videoResultHTTPClient = func() *http.Client {
	return GetSSRFProtectedHTTPClient()
}

var videoResultURLValidator = ValidateSSRFProtectedFetchURL

type VideoResponseFormat string

const (
	VideoResponseFormatMetadataURL VideoResponseFormat = "metadata_url"
	VideoResponseFormatVideoURL    VideoResponseFormat = "video_url"
)

type videoResultStorageError struct {
	kind  string
	cause error
}

func (e *videoResultStorageError) Error() string {
	return "video_result_storage_error: " + e.kind
}

func (e *videoResultStorageError) Unwrap() error { return e.cause }

func newVideoResultStorageError(kind string, cause error) error {
	return &videoResultStorageError{kind: kind, cause: cause}
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	return n, err
}

func ProcessVideoResultURL(ctx context.Context, task *model.Task, sourceURL string) error {
	if task == nil {
		return newVideoResultStorageError("invalid_task", fmt.Errorf("task is nil"))
	}
	cfg := object_storage.Runtime().ObjectStorageConfig
	if !cfg.Enabled {
		setDirectVideoResult(task, sourceURL)
		return nil
	}
	if err := object_storage.ValidateConfig(cfg); err != nil {
		clearStoredVideoResult(task)
		return newVideoResultStorageError("invalid_configuration", err)
	}
	transfer, err := objectstorage.ShouldTransfer(sourceURL, cfg.TransferDomainWhitelist, cfg.NoTransferDomainBlacklist)
	if err != nil {
		clearStoredVideoResult(task)
		return newVideoResultStorageError("invalid_source_url", err)
	}
	if !transfer {
		setDirectVideoResult(task, sourceURL)
		return nil
	}

	clearStoredVideoResult(task)
	if err := videoResultURLValidator(sourceURL); err != nil {
		return newVideoResultStorageError("ssrf_rejected", err)
	}
	client := videoResultHTTPClient()
	if client == nil {
		return newVideoResultStorageError("http_client_unavailable", fmt.Errorf("http client is not initialized"))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return newVideoResultStorageError("download_request", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return newVideoResultStorageError("download_request", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return newVideoResultStorageError("download_status", fmt.Errorf("status %d", response.StatusCode))
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if !strings.HasPrefix(contentType, "video/") {
		return newVideoResultStorageError("unsafe_content_type", fmt.Errorf("content type is not video"))
	}
	maxBytes := int64(cfg.MaxVideoSizeMB) << 20
	if response.ContentLength > maxBytes {
		return newVideoResultStorageError("video_too_large", fmt.Errorf("content length exceeds configured limit"))
	}
	key, err := objectstorage.BuildVideoObjectKey(task.Properties.OriginModelName, task.TaskID, contentType, sourceURL)
	if err != nil {
		return newVideoResultStorageError("invalid_object_key", err)
	}
	store, err := videoResultStoreFactory(objectstorage.Config{
		Endpoint:        cfg.Endpoint,
		PublicEndpoint:  cfg.PublicEndpoint,
		Region:          cfg.Region,
		Bucket:          cfg.Bucket,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		UsePathStyle:    cfg.UsePathStyle,
	})
	if err != nil {
		return newVideoResultStorageError("store_unavailable", err)
	}
	exists, err := store.Exists(ctx, key)
	if err != nil {
		return newVideoResultStorageError("store_head", err)
	}
	if exists {
		setStoredVideoResult(task, key, contentType)
		return nil
	}

	reader := &countingReader{reader: io.LimitReader(response.Body, maxBytes+1)}
	if err := store.Put(ctx, key, contentType, reader, response.ContentLength); err != nil {
		if existsAfterFailure, headErr := store.Exists(ctx, key); headErr == nil && existsAfterFailure {
			setStoredVideoResult(task, key, contentType)
			return nil
		}
		return newVideoResultStorageError("store_put", err)
	}
	if reader.count > maxBytes {
		_ = store.Delete(context.Background(), key)
		return newVideoResultStorageError("video_too_large", fmt.Errorf("downloaded content exceeds configured limit"))
	}
	exists, err = store.Exists(ctx, key)
	if err != nil {
		return newVideoResultStorageError("store_head", err)
	}
	if !exists {
		return newVideoResultStorageError("store_verification", fmt.Errorf("uploaded object was not found"))
	}
	setStoredVideoResult(task, key, contentType)
	return nil
}

func ResolveTaskResultURL(ctx context.Context, task *model.Task) (string, error) {
	if task == nil {
		return "", nil
	}
	if strings.TrimSpace(task.PrivateData.ResultObjectKey) == "" {
		return task.GetResultURL(), nil
	}
	cfg := object_storage.Runtime().ObjectStorageConfig
	if err := object_storage.ValidateConfig(cfg); err != nil {
		return "", newVideoResultStorageError("invalid_configuration", err)
	}
	store, err := videoResultStoreFactory(objectstorage.Config{
		Endpoint:        cfg.Endpoint,
		PublicEndpoint:  cfg.PublicEndpoint,
		Region:          cfg.Region,
		Bucket:          cfg.Bucket,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		UsePathStyle:    cfg.UsePathStyle,
	})
	if err != nil {
		return "", newVideoResultStorageError("store_unavailable", err)
	}
	url, err := store.PresignGet(ctx, task.PrivateData.ResultObjectKey, time.Duration(cfg.ExpiresSeconds)*time.Second)
	if err != nil {
		return "", newVideoResultStorageError("presign", err)
	}
	return url, nil
}

func RewriteVideoResponseURL(ctx context.Context, task *model.Task, body []byte, format VideoResponseFormat) ([]byte, error) {
	if task == nil || strings.TrimSpace(task.PrivateData.ResultObjectKey) == "" {
		return body, nil
	}
	resolved, err := ResolveTaskResultURL(ctx, task)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	switch format {
	case VideoResponseFormatMetadataURL:
		metadata, _ := payload["metadata"].(map[string]any)
		if metadata == nil {
			metadata = make(map[string]any)
		}
		metadata["url"] = resolved
		payload["metadata"] = metadata
	case VideoResponseFormatVideoURL:
		content, _ := payload["content"].(map[string]any)
		if content == nil {
			content = make(map[string]any)
		}
		content["video_url"] = resolved
		payload["content"] = content
	default:
		return body, nil
	}
	return common.Marshal(payload)
}

func setDirectVideoResult(task *model.Task, sourceURL string) {
	task.PrivateData.ResultURL = sourceURL
	task.PrivateData.ResultObjectKey = ""
	task.PrivateData.ResultObjectContentType = ""
}

func clearStoredVideoResult(task *model.Task) {
	task.PrivateData.ResultURL = ""
	task.PrivateData.ResultObjectKey = ""
	task.PrivateData.ResultObjectContentType = ""
}

func setStoredVideoResult(task *model.Task, key, contentType string) {
	task.PrivateData.ResultURL = ""
	task.PrivateData.ResultObjectKey = key
	task.PrivateData.ResultObjectContentType = contentType
}

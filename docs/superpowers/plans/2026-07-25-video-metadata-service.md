# 独立视频元数据服务实施计划

> **面向实施代理：** 必须使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 技能，按任务逐项实施。使用复选框（`- [ ]`）跟踪进度。

**目标：** 构建一个可部署到独立服务器的内部视频元数据服务，安全下载 MP4/MOV 素材并返回时长、尺寸、帧率和缓存校验信息，避免 new-api 主进程承担媒体流量和解析负载。

**架构：** 将 SSRF 安全 HTTP 客户端抽成 `pkg/ssrffetch` 共享包，现有 `service` 层保留薄适配器。`pkg/videometa` 负责 API 契约、容器解析、有界下载、缓存和 HTTP 服务，`cmd/video-metadata-service` 只负责读取环境变量、装配依赖与优雅退出；媒体正文始终落临时文件，不进入 new-api，也不完整载入内存。

**技术栈：** Go 1.25.1、标准库 `net/http`、`github.com/abema/go-mp4 v1.4.1`、`shopspring/decimal`、Testify、Docker Compose。

---

## 文件边界

- 新建 `pkg/ssrffetch/client.go`：与业务设置无关的 SSRF 校验、DNS 复核、拨号固定和重定向检查。
- 新建 `pkg/videometa/types.go`：内部 API DTO、限制常量、错误分类和字段校验。
- 新建 `pkg/videometa/parser.go`：只解析 MP4/MOV 视频轨元数据。
- 新建 `pkg/videometa/fetcher.go`：HEAD 探测、128 MiB 流式下载、临时文件生命周期。
- 新建 `pkg/videometa/cache.go`：有容量上限的进程内 LRU+TTL 缓存。
- 新建 `pkg/videometa/server.go`：Bearer 鉴权、并发限制、64 KiB 请求限制和健康检查。
- 新建 `cmd/video-metadata-service/main.go`：独立进程入口和环境变量校验。
- 新建 `Dockerfile.video-metadata`、`docs/deployment/video-metadata-service.md`；修改 `docker-compose.local.yml`。

该计划不实现 new-api 调用端，不加入 Excel/CSV 导入，不支持 WebM/MKV，不使用 ffprobe，也不持久化媒体正文。

### 任务 1：定义稳定的内部 API 契约

**文件：**

- 新建： `pkg/videometa/types.go`
- 新建： `pkg/videometa/types_test.go`

- [ ] **步骤 1：先写 DTO 和边界校验失败测试**

覆盖 HTTP(S) 协议、`media_type=video`、128 MiB、30000 ms、正数时长/尺寸/帧率、最大帧率 240、最大尺寸 16384，并断言错误文本不包含原始 URL。

```go
func TestValidateRequestRejectsUnsafeLimits(t *testing.T) {
	tests := []Request{
		{URL: "file:///etc/passwd", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS},
		{URL: "https://example.com/a.mp4", MediaType: "audio", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS},
		{URL: "https://example.com/a.mp4", MediaType: "video", MaxBytes: MaxVideoBytes + 1, DeadlineMS: MaxDeadlineMS},
		{URL: "https://example.com/a.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS + 1},
	}
	for _, input := range tests {
		assert.Error(t, input.Validate())
	}
}

func TestMetadataValidateRejectsImpossibleFrameRate(t *testing.T) {
	input := Metadata{DurationMS: 5200, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 0, Container: "mp4"}
	assert.Error(t, input.Validate())
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./pkg/videometa -run 'TestValidateRequest|TestMetadataValidate' -count=1`

预期： 编译失败，提示 `Request`、`Metadata` 和限制常量尚未定义。

- [ ] **步骤 3：实现精确契约和类型化错误**

```go
const (
	MaxVideoBytes int64 = 128 * 1024 * 1024
	MaxDeadlineMS int64 = 30_000
	MaxDimension        = 16_384
	MaxFrameRate        = 240
)

type Request struct {
	URL        string `json:"url"`
	MediaType  string `json:"media_type"`
	MaxBytes   int64  `json:"max_bytes"`
	DeadlineMS int64  `json:"deadline_ms"`
}

type Metadata struct {
	DurationMS   int64  `json:"duration_ms"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FrameRateNum int64  `json:"frame_rate_num"`
	FrameRateDen int64  `json:"frame_rate_den"`
	Container    string `json:"container"`
	ContentLength int64 `json:"content_length"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

type ErrorCode string

const (
	ErrorInvalidRequest     ErrorCode = "invalid_request"
	ErrorUnauthorized       ErrorCode = "unauthorized"
	ErrorUnsupportedFormat  ErrorCode = "unsupported_format"
	ErrorMediaTooLarge      ErrorCode = "media_too_large"
	ErrorMetadataInvalid    ErrorCode = "metadata_invalid"
	ErrorFetchUnavailable   ErrorCode = "fetch_unavailable"
	ErrorDeadlineExceeded   ErrorCode = "deadline_exceeded"
	ErrorConcurrencyLimited ErrorCode = "concurrency_limited"
	ErrorInternal           ErrorCode = "internal_error"
)

type ServiceError struct {
	Code       ErrorCode
	HTTPStatus int
	Err        error
}

type ErrorResponse struct {
	Error struct {
		Code    ErrorCode `json:"code"`
		Message string    `json:"message"`
	} `json:"error"`
}
```

`Request.Validate()` 只做结构和协议检查；DNS、IP、端口与重定向由 `pkg/ssrffetch` 权威检查。`Metadata.Validate()` 接受 `mp4` 或 `mov`，要求 `duration_ms > 0`、`content_length >= 0`、尺寸在 1..16384、帧率大于 0 且不超过 240。

- [ ] **步骤 4：运行测试并确认 GREEN**

执行：`go test ./pkg/videometa -run 'TestValidateRequest|TestMetadataValidate' -count=1`

预期： PASS。

- [ ] **步骤 5：提交 API 契约**

```text
git add pkg/videometa/types.go pkg/videometa/types_test.go
git commit -m "feat: define video metadata service contract"
```

### 任务 2：抽取共享 SSRF 安全客户端

**文件：**

- 新建： `pkg/ssrffetch/client.go`
- 新建： `pkg/ssrffetch/client_test.go`
- 修改： `service/protected_fetch_client.go`
- 修改： `service/protected_fetch_client_test.go`

- [ ] **步骤 1：将现有安全语义写成共享包失败测试**

从现有 service 测试迁移 deterministic fake resolver/dialer 用例，断言：拨号解析到私网时拒绝；公网和私网混合结果整体拒绝；每次重定向重新校验；直连实际拨号使用已校验 IP；代理 transport 缓存键只使用代理地址。

```go
func TestDialerRejectsPrivateReboundAddress(t *testing.T) {
	protection := publicOnlyProtection()
	dialer := &protectedDialer{
		resolver: staticResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			require.FailNow(t, "blocked address must not be dialed")
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) { return protection, true, nil },
	}
	_, err := dialer.DialContext(context.Background(), "tcp", "assets.example:443")
	assert.Error(t, err)
}
```

- [ ] **步骤 2：运行共享包和 service 测试并确认 RED**

执行：`go test ./pkg/ssrffetch ./service -run 'SSRF|ProtectedFetch|Dialer|Redirect' -count=1`

预期： `pkg/ssrffetch` 编译失败，service 现有测试保持通过。

- [ ] **步骤 3：实现共享客户端和 service 薄适配器**

```go
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

func NewClient(options Options) (*http.Client, error)
```

`NewClient` 在 `RoundTrip`、`DialContext` 和 `CheckRedirect` 都执行校验。直连 DNS 结果逐个调用 `ValidateResolvedIP`，任一地址不合规就拒绝整个结果；实际拨号必须使用已校验 IP，不能交给 transport 再解析。`service/protected_fetch_client.go` 只保留 `currentFetchProtection()` 和 `ssrffetch.NewClient` 参数装配。

- [ ] **步骤 4：格式化并运行回归测试**

执行：`gofmt -w pkg/ssrffetch/client.go pkg/ssrffetch/client_test.go service/protected_fetch_client.go service/protected_fetch_client_test.go`

执行：`go test ./pkg/ssrffetch ./service -run 'SSRF|ProtectedFetch|Dialer|Redirect' -count=1`

预期： PASS；现有下载、Webhook 和用户通知调用点无需修改。

- [ ] **步骤 5：提交 SSRF 客户端抽取**

```text
git add pkg/ssrffetch/client.go pkg/ssrffetch/client_test.go service/protected_fetch_client.go service/protected_fetch_client_test.go
git commit -m "refactor: share ssrf protected fetch client"
```

### 任务 3：实现 MP4/MOV 视频轨解析器

**文件：**

- 新建： `pkg/videometa/parser.go`
- 新建： `pkg/videometa/parser_test.go`
- 新建： `pkg/videometa/testdata/sample.mp4`
- 新建： `pkg/videometa/testdata/sample_qt.mp4`

- [ ] **步骤 1：固化小型容器样本并写失败测试**

从当前依赖 `github.com/abema/go-mp4 v1.4.1` 的 `testdata/sample.mp4` 和 `testdata/sample_qt.mp4` 复制到本包，并在测试注释记录样本来源和上游 LICENSE。测试不调用网络或 ffmpeg。

```go
func TestParseVideoMetadataMP4(t *testing.T) {
	file, err := os.Open("testdata/sample.mp4")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	metadata, err := Parse(file, 8278)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), metadata.DurationMS)
	assert.Equal(t, 320, metadata.Width)
	assert.Equal(t, 180, metadata.Height)
	assert.Equal(t, int64(10), metadata.FrameRateNum)
	assert.Equal(t, int64(1), metadata.FrameRateDen)
	assert.Equal(t, "mp4", metadata.Container)
}

func TestParseVideoMetadataRejectsBrokenContainer(t *testing.T) {
	reader := bytes.NewReader([]byte("not-an-iso-bmff-file"))
	_, err := Parse(reader, int64(reader.Len()))
	assert.ErrorIs(t, err, ErrUnsupportedContainer)
}
```

- [ ] **步骤 2：运行解析测试并确认 RED**

执行：`go test ./pkg/videometa -run 'TestParseVideoMetadata' -count=1`

预期： 编译失败，提示 `Parse` 和 `ErrUnsupportedContainer` 不存在。

- [ ] **步骤 3：使用轨道 box 实现解析，不依赖 H.264 Probe 字段**

`Parse(io.ReadSeeker, contentLength int64)` 先验证 `ftyp`，再用 `mp4.ExtractBox` 枚举 `moov/trak`。对每个 `trak` 读取 `mdia/hdlr` 识别 `[4]byte{'v','i','d','e'}`、`tkhd` 取宽高、`mdia/mdhd` 取 timescale/duration、`mdia/minf/stbl/stts` 汇总 sample count/sample delta。

```go
func Parse(reader io.ReadSeeker, contentLength int64) (Metadata, error)

func frameRate(stts *mp4.Stts, timescale uint32) (int64, int64, error) {
	var samples uint64
	var units uint64
	for _, entry := range stts.Entries {
		if entry.SampleCount == 0 || entry.SampleDelta == 0 {
			return 0, 0, ErrInvalidVideoTrack
		}
		if uint64(entry.SampleCount) > math.MaxUint64/uint64(entry.SampleDelta) {
			return 0, 0, ErrInvalidVideoTrack
		}
		samples += uint64(entry.SampleCount)
		units += uint64(entry.SampleCount) * uint64(entry.SampleDelta)
	}
	if samples == 0 || units == 0 || timescale == 0 || samples > math.MaxUint64/uint64(timescale) {
		return 0, 0, ErrInvalidVideoTrack
	}
	numerator := samples * uint64(timescale)
	divisor := gcd(numerator, units)
	return int64(numerator / divisor), int64(units / divisor), nil
}
```

时长使用视频轨 `mdhd.GetDuration()/Timescale`，毫秒向上取整：`(duration*1000 + timescale - 1) / timescale`。多视频轨选择像素面积最大的有效轨，面积相同选第一个。容器根据 `ftyp` major/compatible brand 判定 `mov` 或 `mp4`，最后调用 `Metadata.Validate()`。

- [ ] **步骤 4：增加损坏边界并运行 GREEN**

补充 `timescale=0`、`stts` 空、无 `vide` 轨、宽高为零和 QuickTime brand 用例。

执行：`go test ./pkg/videometa -run 'TestParseVideoMetadata|TestFrameRate' -count=1`

预期： PASS；测试证明不依赖 `Probe.Track.AVC` 也能取得宽高。

- [ ] **步骤 5：提交解析器**

```text
git add pkg/videometa/parser.go pkg/videometa/parser_test.go pkg/videometa/testdata/sample.mp4 pkg/videometa/testdata/sample_qt.mp4
git commit -m "feat: parse mp4 and mov video metadata"
```

### 任务 4：实现有界下载和服务端缓存

**文件：**

- 新建： `pkg/videometa/cache.go`
- 新建： `pkg/videometa/cache_test.go`
- 新建： `pkg/videometa/fetcher.go`
- 新建： `pkg/videometa/fetcher_test.go`

- [ ] **步骤 1：写下载限制、缓存键和临时文件失败测试**

使用 `httptest.Server` 与注入的 `http.Client`，覆盖：HEAD 返回 ETag 后缓存命中只执行一次 GET；HEAD 405 时仍可 GET；声明或实际正文超过限制返回 `ErrorMediaTooLarge`；取消 context 后临时文件被删除；查询参数 URL 使用短 TTL；错误文本不含完整 URL。

```go
func TestFetcherStopsAtConfiguredByteLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("x"), 1025))
	}))
	t.Cleanup(server.Close)

	tempDir := t.TempDir()
	fetcher := NewFetcher(FetcherOptions{Client: server.Client(), MaxBytes: 1024, TempDir: tempDir})
	_, err := fetcher.Metadata(context.Background(), Request{URL: server.URL + "/video.mp4", MediaType: "video", MaxBytes: 1024, DeadlineMS: 1000})
	assertServiceErrorCode(t, err, ErrorMediaTooLarge)
	entries, readErr := os.ReadDir(tempDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./pkg/videometa -run 'TestFetcher|TestCache' -count=1`

预期： 编译失败，提示 `Fetcher` 和 `Cache` 尚未定义。

- [ ] **步骤 3：实现有容量上限的 LRU+TTL 缓存**

只用 `container/list`、`map` 和 `sync.Mutex`。缓存键是 SHA-256，不保存可枚举的完整 URL：

```go
type CacheKeyInput struct {
	URL           string
	ETag          string
	LastModified  string
	ContentLength int64
}

func CacheKey(input CacheKeyInput) string {
	sum := sha256.Sum256([]byte(input.URL + "\x00" + input.ETag + "\x00" + input.LastModified + "\x00" + strconv.FormatInt(input.ContentLength, 10)))
	return hex.EncodeToString(sum[:])
}
```

`Cache.Get` 在锁内删除过期项并移动到队首；`Cache.Set` 更新重复键并从队尾淘汰，`capacity <= 0` 时禁止缓存。无查询参数默认 TTL 600 秒，有查询参数默认 TTL 60 秒。

- [ ] **步骤 4：实现 HEAD+GET 流式下载和清理**

```go
type FetcherOptions struct {
	Client            *http.Client
	Cache             *Cache
	MaxBytes          int64
	TempDir           string
	CacheTTL          time.Duration
	SignedURLCacheTTL time.Duration
}

func NewFetcher(options FetcherOptions) *Fetcher
func (f *Fetcher) Metadata(ctx context.Context, request Request) (Metadata, error)
```

先发 HEAD 获取 `ETag`、`Last-Modified`、`Content-Length`；405/501 回退 GET。GET 用 `io.LimitReader(resp.Body, maxBytes+1)` 写入 `os.CreateTemp`；写入字节超过上限立即返回并由 defer 删除文件。只把临时文件句柄传给 `Parse`。Content-Length 用 `strconv.ParseInt` 且拒绝负数；只接受 2xx；deadline 映射 `ErrorDeadlineExceeded`，DNS/连接/5xx 映射 `ErrorFetchUnavailable`，解析错误映射 `ErrorUnsupportedFormat` 或 `ErrorMetadataInvalid`。

- [ ] **步骤 5：运行并提交下载缓存**

执行：`gofmt -w pkg/videometa/cache.go pkg/videometa/cache_test.go pkg/videometa/fetcher.go pkg/videometa/fetcher_test.go`

执行：`go test ./pkg/videometa -run 'TestFetcher|TestCache' -count=1`

预期： PASS；测试临时目录在成功、解析失败、超限和取消路径均为空。

```text
git add pkg/videometa/cache.go pkg/videometa/cache_test.go pkg/videometa/fetcher.go pkg/videometa/fetcher_test.go
git commit -m "feat: add bounded video metadata fetcher"
```

### 任务 5：实现鉴权、并发限制和 HTTP 服务

**文件：**

- 新建： `pkg/videometa/server.go`
- 新建： `pkg/videometa/server_test.go`

- [ ] **步骤 1：写 HTTP 契约和隐私失败测试**

覆盖 `/healthz`、缺失/错误 Bearer 令牌 401、超 64 KiB JSON 413、并发槽耗尽 503、非法素材 400、下载不可用 503、成功 200。捕获测试 logger，断言令牌和 URL 查询参数未出现。

```go
func TestServerReturnsStableMetadataEnvelope(t *testing.T) {
	handler := NewServer(ServerOptions{
		Token: "service-secret",
		MaxConcurrency: 2,
		Metadata: func(context.Context, Request) (Metadata, error) {
			return Metadata{DurationMS: 5200, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1834210}, nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/metadata/video", strings.NewReader(`{"url":"https://assets.example/input.mp4?sig=secret","media_type":"video","max_bytes":134217728,"deadline_ms":30000}`))
	req.Header.Set("Authorization", "Bearer service-secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":1834210}`, recorder.Body.String())
}
```

- [ ] **步骤 2：运行服务测试并确认 RED**

执行：`go test ./pkg/videometa -run 'TestServer' -count=1`

预期： 编译失败，提示 `NewServer` 不存在。

- [ ] **步骤 3：实现标准库 HTTP handler**

`server.go` 不直接调用 `encoding/json`；读取和写出分别使用 `common.DecodeJson`、`common.Marshal`。Bearer 比较使用固定长度 SHA-256 后的 `subtle.ConstantTimeCompare`。

```go
type ServerOptions struct {
	Token          string
	MaxConcurrency int
	Metadata       func(context.Context, Request) (Metadata, error)
	Log            func(message string, fields map[string]any)
}

func NewServer(options ServerOptions) http.Handler
```

`POST /v1/metadata/video` 通过 `http.MaxBytesReader` 限制 64 KiB，拒绝尾随第二个 JSON 值。容量为 `MaxConcurrency` 的 channel 非阻塞获取槽位，失败返回 `concurrency_limited`。从 `deadline_ms` 创建子 context，但不超过服务端 30 秒。日志字段只允许 `request_id`、`result_code`、`elapsed_ms`、`bytes`、`cache_hit`。

- [ ] **步骤 4：运行 HTTP 契约和 race 测试**

执行：`go test -race ./pkg/videometa -run 'TestServer' -count=1`

预期： PASS；并发测试没有 data race，普通错误 message 不回显 URL、validator 或内部网络错误。

- [ ] **步骤 5：提交 HTTP 服务**

```text
git add pkg/videometa/server.go pkg/videometa/server_test.go
git commit -m "feat: serve authenticated video metadata api"
```

### 任务 6：增加独立二进制和本地部署

**文件：**

- 新建： `cmd/video-metadata-service/main.go`
- 新建： `cmd/video-metadata-service/main_test.go`
- 新建： `Dockerfile.video-metadata`
- 修改： `docker-compose.local.yml`
- 新建： `docs/deployment/video-metadata-service.md`

- [ ] **步骤 1：写环境配置失败测试**

将环境读取保持为可测试纯函数，断言空令牌启动失败、无效整数失败、默认限制精确一致。

```go
func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("VIDEO_METADATA_SERVICE_TOKEN", "secret")
	config, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, ":8090", config.ListenAddr)
	assert.Equal(t, int64(134217728), config.MaxBytes)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 16, config.MaxConcurrency)
	assert.Equal(t, 10000, config.CacheEntries)
}
```

- [ ] **步骤 2：运行入口测试并确认 RED**

执行：`go test ./cmd/video-metadata-service -run 'TestLoadConfig' -count=1`

预期： 编译失败，提示 `loadConfig` 不存在。

- [ ] **步骤 3：实现启动、SSRF 默认策略和优雅退出**

支持并校验：

```text
VIDEO_METADATA_LISTEN_ADDR=:8090
VIDEO_METADATA_SERVICE_TOKEN=<required>
VIDEO_METADATA_MAX_BYTES=134217728
VIDEO_METADATA_TIMEOUT_SECONDS=30
VIDEO_METADATA_MAX_CONCURRENCY=16
VIDEO_METADATA_CACHE_ENTRIES=10000
VIDEO_METADATA_CACHE_TTL_SECONDS=600
VIDEO_METADATA_SIGNED_URL_CACHE_TTL_SECONDS=60
```

独立服务的默认 `common.SSRFProtection` 禁止私网、启用域名解析后的 IP 复核、只允许 80/443，并采用空黑名单。用该实例的 `ValidateURL` 和 `ValidateResolvedIP` 装配 `ssrffetch.NewClient`。`http.Server` 设置 `ReadHeaderTimeout=5s`、`ReadTimeout=35s`、`WriteTimeout=35s`、`IdleTimeout=60s`；SIGINT/SIGTERM 最多等待 10 秒。

- [ ] **步骤 4：增加镜像、Compose 和部署文档**

`Dockerfile.video-metadata` 只构建 `./cmd/video-metadata-service`，运行镜像安装 `ca-certificates`、`tzdata`、`wget`，使用非 root 用户和可写 `/tmp`。Compose 增加 `video-metadata`，给 `new-api` 增加：

```yaml
VIDEO_METADATA_SERVICE_URL: http://video-metadata:8090
VIDEO_METADATA_SERVICE_TOKEN: "${VIDEO_METADATA_SERVICE_TOKEN:-local-video-metadata-secret}"
```

部署文档明确：生产 token 至少 32 随机字节；防火墙只允许 new-api 节点访问；基础设施支持时在反向代理层启用 mTLS；监控 `request_count`、`error_code`、`download_bytes`、`parse_latency_ms`、`active_requests`、`cache_hit`；日志不得记录 URL 查询参数。

- [ ] **步骤 5：运行构建与配置检查并提交**

执行：`gofmt -w cmd/video-metadata-service/main.go cmd/video-metadata-service/main_test.go`

执行：`go test ./cmd/video-metadata-service ./pkg/videometa ./pkg/ssrffetch -count=1`

执行：`go build ./cmd/video-metadata-service`

执行：`docker compose -f docker-compose.local.yml config`

预期： Go 测试和构建 PASS；Compose 输出包含 `video-metadata:8090`，没有未解析的必填字段错误。

```text
git add cmd/video-metadata-service/main.go cmd/video-metadata-service/main_test.go Dockerfile.video-metadata docker-compose.local.yml docs/deployment/video-metadata-service.md
git commit -m "feat: deploy standalone video metadata service"
```

### 任务 7：完成安全与全仓回归验证

**文件：**

- 修改： `pkg/videometa/fetcher_test.go`
- 修改： `pkg/videometa/server_test.go`
- 修改： `pkg/ssrffetch/client_test.go`
- 修改： `docs/deployment/video-metadata-service.md`

- [ ] **步骤 1：增加完整链路回归**

用外层 `httptest.Server` 模拟服务 API，内层素材服务分别模拟重定向到私网、超大正文、慢响应、合法 MP4 和损坏 MOV。断言预算覆盖 HEAD+GET+解析总时长，重定向无法绕过 SSRF，同一 validator 第二次命中缓存且不再 GET。

```go
func TestMetadataServiceEndToEndCachesValidatedAsset(t *testing.T) {
	var gets atomic.Int32
	asset := newMP4AssetServer(t, &gets)
	handler := newTestMetadataHandler(t, asset.Client())

	first := postMetadata(t, handler, asset.URL+"/sample.mp4")
	second := postMetadata(t, handler, asset.URL+"/sample.mp4")
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, int32(1), gets.Load())
}
```

- [ ] **步骤 2：执行隐私和危险调用扫描**

执行：`rg -n 'Authorization|RawQuery|RequestURI|\.URL\.String\(\)' pkg/videometa cmd/video-metadata-service`

预期： 只命中鉴权读取和测试断言，不命中日志拼接。

执行：`rg -n 'io\.ReadAll|os\.ReadFile|encoding/json' pkg/videometa pkg/ssrffetch cmd/video-metadata-service`

预期： 不命中媒体下载路径和业务 JSON 编解码。

- [ ] **步骤 3：运行 race、vet 和全量 Go 测试**

执行：`go test -race ./pkg/videometa ./pkg/ssrffetch ./service -count=1`

执行：`go vet ./pkg/videometa ./pkg/ssrffetch ./cmd/video-metadata-service`

执行：`go test ./... -count=1`

预期： 全部 PASS；若全仓存在与本计划无关的既有失败，记录精确包名和测试名，不修改无关文件。

- [ ] **步骤 4：验证容器健康与临时文件清理**

执行：`docker compose -f docker-compose.local.yml up -d --build video-metadata`

执行：`docker compose -f docker-compose.local.yml ps video-metadata`

执行：`docker compose -f docker-compose.local.yml logs --no-color video-metadata`

预期： 状态 healthy；日志没有服务 token、完整素材 URL或查询参数；容器 `/tmp` 在请求结束后没有遗留媒体文件。

- [ ] **步骤 5：提交最终回归补充**

```text
git add pkg/videometa pkg/ssrffetch docs/deployment/video-metadata-service.md
git commit -m "test: verify video metadata service boundaries"
```

## 验收标准

- `POST /v1/metadata/video` 在 30 秒内返回经过二次校验的 MP4/MOV 元数据，或稳定的 400/401/413/503 错误。
- 单视频正文绝不超过 128 MiB，正文不进入 new-api、不完整载入内存、临时文件始终清理。
- SSRF 防护覆盖 URL、DNS 解析、实际拨号和每次重定向，DNS rebinding 不能绕过。
- 服务能作为独立二进制和独立容器部署到专属服务器，new-api 只需通过内部 HTTP JSON 调用。
- 缓存有容量与 TTL 上限，带查询参数 URL 使用短 TTL，日志和错误不泄露 URL、令牌或媒体内容。

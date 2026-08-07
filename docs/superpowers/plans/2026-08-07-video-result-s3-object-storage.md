# 视频生成结果 S3 对象存储 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为标准视频任务增加 S3 兼容对象存储转存、域名转存规则、动态预签名结果 URL，以及管理员单页配置。

**Architecture:** 使用 `setting/object_storage` 保存并原子同步运行时配置；`pkg/objectstorage` 封装 S3 客户端、域名匹配、对象键和预签名；`service` 在定时轮询和 Gemini/Vertex 实时查询的成功终态前统一处理视频 URL。任务私有 JSON 只保存对象键，每次公共响应重新生成预签名 URL。后台通过 RootAuth 专用接口保存/测试配置，Operations 下新增单页表单。

**Tech Stack:** Go 1.22、Gin、GORM v2、AWS SDK v2 S3/uploader/presigner、React 19、TypeScript、React Hook Form、Zod、React Query、i18next、Bun。

---

## 文件映射

| 文件 | 责任 |
| --- | --- |
| `setting/object_storage/config.go` | 配置结构、默认值、范围校验、运行时快照 |
| `setting/object_storage/config_test.go` | 配置默认值、合法性和归一化测试 |
| `model/option.go` | 注册分层配置包并在批量更新后刷新对象存储快照 |
| `controller/option.go` | 旧通用 Option API 过滤对象存储 Secret |
| `controller/option_object_storage_test.go` | 防止 `secret_access_key` 从 `/api/option/` 泄漏 |
| `pkg/objectstorage/rules.go` | Host 归一化、精确/通配域名匹配和黑名单优先决策 |
| `pkg/objectstorage/key.go` | 基于本站模型 ID/公开任务 ID 的对象键生成 |
| `pkg/objectstorage/s3.go` | S3 客户端、流式上传、Head、探针、动态预签名 |
| `pkg/objectstorage/*_test.go` | S3 请求、对象键和域名规则的确定性测试 |
| `model/task.go` | `TaskPrivateData` 增加对象键和 Content-Type 字段 |
| `service/video_result_storage.go` | 下载、大小/MIME 校验、域名决策、上传和错误分类 |
| `service/video_result_storage_test.go` | 转存成功、跳过、失败、SSRF 和大小边界测试 |
| `service/task_polling.go` | 标准视频轮询成功终态接入统一转存处理 |
| `relay/relay_task.go` | Gemini/Vertex 实时查询、Task DTO 和响应转换接入动态 URL |
| `relay/seedance_task.go` | ARK/Seedance 响应结果覆盖为动态对象存储 URL |
| `service/task_response_audit.go` | 终态用户响应审计使用不泄漏上游域名的 URL |
| `service/public_task.go` | 用户任务投影使用动态对象存储 URL |
| `controller/video_proxy.go`、`controller/video_proxy_gemini.go` | 读取任务媒体时解析最新预签名 URL |
| `controller/object_storage.go` | 管理员读取、保存、测试对象存储配置 |
| `controller/object_storage_test.go` | API 权限、校验、掩码和原子保存测试 |
| `router/api-router.go` | 注册 `/api/object-storage/*` RootAuth 路由 |
| `web/src/features/system-settings/api.ts` | 对象存储 GET/PUT/test API 封装 |
| `web/src/features/system-settings/types.ts` | 对象存储配置请求/响应类型 |
| `web/src/features/system-settings/operations/object-storage-section.tsx` | 单页对象存储表单 |
| `web/src/features/system-settings/operations/section-registry.tsx` | Operations 中注册 Object Storage section |
| `web/src/features/system-settings/operations/index.tsx` | Operations 默认配置/section 类型扩展 |
| `web/src/features/system-settings/operations/__tests__/object-storage-section.test.tsx` | 表单行为和错误状态测试 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` | 新增后台文案翻译 |

## Task 1: 配置模块与 OptionMap 注册

**Files:**
- Create: `setting/object_storage/config.go`
- Test: `setting/object_storage/config_test.go`
- Modify: `model/option.go`
- Modify: `controller/option.go`
- Test: `controller/option_object_storage_test.go`

- [ ] **Step 1: Write failing configuration tests**

在 `setting/object_storage/config_test.go` 使用 `require`/`assert` 添加以下表格场景：

```go
func TestValidateConfigRequiresCredentialsWhenEnabled(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ObjectStorageConfig)
		wantErr string
	}{
		{"disabled allows draft", func(c *ObjectStorageConfig) { c.Enabled = false }, ""},
		{"missing endpoint", func(c *ObjectStorageConfig) { c.Endpoint = "" }, "endpoint"},
		{"missing public endpoint", func(c *ObjectStorageConfig) { c.PublicEndpoint = "" }, "public_endpoint"},
		{"missing secret", func(c *ObjectStorageConfig) { c.SecretAccessKey = "" }, "secret_access_key"},
		{"expires too short", func(c *ObjectStorageConfig) { c.ExpiresSeconds = 59 }, "expires_seconds"},
		{"expires too long", func(c *ObjectStorageConfig) { c.ExpiresSeconds = 604801 }, "expires_seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testEnabledConfig()
			tt.mutate(&cfg)
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
```

另加默认值断言：`Enabled=false`、`Region=us-east-1`、`MaxVideoSizeMB=512`、`ExpiresSeconds=86400`，以及边界 `1/2048 MB`、`60/604800 秒`。

- [ ] **Step 2: Run the focused test and verify failure**

Run: `go test ./setting/object_storage -run 'TestValidateConfig|TestDefault' -count=1`

Expected: FAIL because the package and `Config`/`ValidateConfig` are not defined。

- [ ] **Step 3: Implement configuration and runtime snapshot**

实现以下公开 API：

```go
const ConfigName = "object_storage"

type ObjectStorageConfig struct {
	Enabled                     bool     `json:"enabled"`
	Endpoint                    string   `json:"endpoint"`
	PublicEndpoint              string   `json:"public_endpoint"`
	Region                      string   `json:"region"`
	Bucket                      string   `json:"bucket"`
	AccessKeyID                 string   `json:"access_key_id"`
	SecretAccessKey             string   `json:"secret_access_key"`
	UsePathStyle                bool     `json:"use_path_style"`
	MaxVideoSizeMB              int      `json:"max_video_size_mb"`
	ExpiresSeconds              int      `json:"expires_seconds"`
	TransferDomainWhitelist     []string `json:"transfer_domain_whitelist"`
	NoTransferDomainBlacklist   []string `json:"no_transfer_domain_blacklist"`
}

type RuntimeSnapshot struct { ObjectStorageConfig }

func Runtime() RuntimeSnapshot
func UpdateAndSync()
func ValidateConfig(ObjectStorageConfig) error
func DefaultConfig() ObjectStorageConfig
```

注册 `config.GlobalConfig.Register(ConfigName, &objectStorageConfig)`，将列表字段按 JSON 数组解析。归一化时裁剪空白、域名统一小写、补默认 Region/大小/有效期；启用配置缺 Endpoint/PublicEndpoint/Bucket/凭据时返回可展示的字段错误。

- [ ] **Step 4: Register the package in the model option loader**

在 `model/option.go` 引入 `setting/object_storage`，使其注册包在 `config.GlobalConfig.ExportAllConfigs()` 中出现，并在 `handleConfigUpdate` 增加：

```go
} else if configName == object_storage.ConfigName {
	object_storage.UpdateAndSync()
}
```

配置会通过 `config.GlobalConfig.ExportAllConfigs()` 进入 `OptionMap`，因此同一任务必须立即扩展 `controller.GetOptions` 的敏感键判断。

- [ ] **Step 5: Protect the legacy option endpoint before committing**

在 `controller/option.go` 抽取并使用：

```go
func isSensitiveOptionKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.HasSuffix(key, "Token") ||
		strings.HasSuffix(key, "Secret") ||
		strings.HasSuffix(key, "Key") ||
		strings.HasSuffix(lower, "secret") ||
		strings.HasSuffix(lower, "api_key") ||
		strings.HasSuffix(lower, "secret_access_key")
}
```

在 `controller/option_object_storage_test.go` 把 `object_storage.secret_access_key` 放入 `common.OptionMap`，调用 `GetOptions`，断言响应不包含该 key，同时 `object_storage.access_key_id` 仍可返回。

- [ ] **Step 6: Run configuration and secret-filter tests and commit**

Run: `go test ./setting/object_storage ./model ./controller -run 'ObjectStorage|Option' -count=1`

Expected: PASS。

Commit: `feat: add object storage runtime settings`

## Task 2: 域名策略、对象键和 S3 核心客户端

**Files:**
- Create: `pkg/objectstorage/rules.go`
- Create: `pkg/objectstorage/key.go`
- Create: `pkg/objectstorage/s3.go`
- Test: `pkg/objectstorage/rules_test.go`
- Test: `pkg/objectstorage/key_test.go`
- Test: `pkg/objectstorage/s3_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Write domain and key tests**

先写 `rules_test.go`，覆盖精确 Host、`*.example.com` 只匹配子域、大小写/尾部点/端口归一化、黑名单优先和默认不转存：

```go
func TestShouldTransfer(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		whitelist []string
		blacklist []string
		want      bool
	}{
		{"whitelist exact", "https://OWN.Example.com/a.mp4", []string{"own.example.com"}, nil, true},
		{"blacklist wins", "https://own.example.com/a.mp4", []string{"own.example.com"}, []string{"own.example.com"}, false},
		{"wildcard child", "https://cdn.example.com/a.mp4", []string{"*.example.com"}, nil, true},
		{"wildcard excludes root", "https://example.com/a.mp4", []string{"*.example.com"}, nil, false},
		{"default skip", "https://other.example/a.mp4", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShouldTransfer(tt.url, tt.whitelist, tt.blacklist)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

`key_test.go` 断言 `BuildVideoObjectKey("doubao-seedance-2-0-fast", "task_public", "video/mp4", "https://x/result")` 返回 `doubao-seedance-2-0-fast/task_public.mp4`，并断言 `/`, `?`, 空模型等危险输入被归一化。

- [ ] **Step 2: Run the new tests and verify failure**

Run: `go test ./pkg/objectstorage -run 'TestShouldTransfer|TestBuildVideoObjectKey' -count=1`

Expected: FAIL because the package functions are not defined。

- [ ] **Step 3: Implement Host policy and safe key builder**

实现签名：

```go
func ShouldTransfer(rawURL string, whitelist, blacklist []string) (bool, error)
func BuildVideoObjectKey(originModelName, publicTaskID, contentType, sourceURL string) (string, error)
```

`ShouldTransfer` 只接受绝对 HTTP(S) URL；黑名单先查，白名单支持精确匹配或前缀 `*.` 子域匹配。`BuildVideoObjectKey` 使用 `url.Parse`/MIME 类型推导扩展名，不使用字符串拼接解析 URL 路径，模型 ID 和任务 ID 各自清理为单一路径段。

- [ ] **Step 4: Write S3 client contract tests**

在 `s3_test.go` 使用 `httptest.Server` 验证：

- Endpoint 和 `UsePathStyle=true` 时 PUT 路径为 `/<bucket>/<key>`。
- `Content-Type` 被传递到 PutObject。
- `HeadObject` 已存在返回 true，404 返回 false。
- 探针执行 Put、Head、Delete。
- Presign URL 包含 `X-Amz-Expires=86400`（使用测试配置），且不把 Secret 写入日志/错误。

- [ ] **Step 5: Add AWS S3 dependencies and implement the client**

使用 Go modules 增加 `github.com/aws/aws-sdk-go-v2/service/s3` 和 `github.com/aws/aws-sdk-go-v2/feature/s3/manager`，然后执行 `go mod tidy`。在 `pkg/objectstorage/s3.go` 定义与 setting 包解耦的客户端配置：

```go
type Config struct {
	Endpoint        string
	PublicEndpoint  string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}
```

并实现：

```go
type Store struct {
	api      *s3.Client
	presign  *s3.PresignClient
	uploader *manager.Uploader
	bucket   string
}

func New(cfg Config) (*Store, error)
func (s *Store) Put(ctx context.Context, key, contentType string, body io.Reader, size int64) error
func (s *Store) Exists(ctx context.Context, key string) (bool, error)
func (s *Store) Delete(ctx context.Context, key string) error
func (s *Store) PresignGet(ctx context.Context, key string, expires time.Duration) (string, error)
func (s *Store) Probe(ctx context.Context) error
```

客户端构造同时使用 API Endpoint 和 Public Endpoint；预签名客户端使用 Public Endpoint、同一 Region/凭据/Path Style。`Put` 使用 manager uploader 流式上传，设置 `ContentLength`（未知时以限制读取器计数并拒绝超过配置上限）。

- [ ] **Step 6: Run package tests and commit**

Run: `go test ./pkg/objectstorage -count=1`

Expected: PASS。

Commit: `feat: add s3 object storage client and domain policy`

## Task 3: 任务私有元数据与视频转存服务

**Files:**
- Modify: `model/task.go`
- Create: `service/video_result_storage.go`
- Test: `service/video_result_storage_test.go`

- [ ] **Step 1: Write task metadata regression tests**

在 `model/task_private_data_test.go` 增加 Value/Scan 断言，确认 `ResultObjectKey` 和 `ResultObjectContentType` 序列化后仍存在，旧的只含 `ResultURL` 数据仍可读取。

- [ ] **Step 2: Write service transfer tests with a fake store**

在 `service/video_result_storage_test.go` 注入 fake store，覆盖：

```go
func TestProcessVideoResultTransfersWhitelistedURL(t *testing.T) {
	// httptest server returns video/mp4 and a bounded body.
	// configure whitelist with server host, fake store records key/body.
	// assert ResultObjectKey == "doubao-seedance-2-0-fast/task_public.mp4".
	// assert ResultURL is empty for stored objects; public resolution happens later.
}

func TestProcessVideoResultLeavesBlacklistedAndDefaultURLs(t *testing.T)
func TestProcessVideoResultRejectsOversizedVideoWithoutUpstreamFallback(t *testing.T)
func TestProcessVideoResultTreatsExistingObjectAsSuccess(t *testing.T)
func TestProcessVideoResultClassifiesSSRFAndMimeErrorsAsTerminalFailure(t *testing.T)
```

测试必须断言黑名单/默认路径不会调用 fake store，上传失败不会把源 URL 写入结果字段。

- [ ] **Step 3: Add task fields and result resolver API**

在 `TaskPrivateData` 增加：

```go
ResultObjectKey         string `json:"result_object_key,omitempty"`
ResultObjectContentType string `json:"result_object_content_type,omitempty"`
```

更新 `TaskPrivateData.Value` 的空值判断。新增服务层接口：

```go
func ResolveTaskResultURL(ctx context.Context, task *model.Task) (string, error)
func RewriteVideoResponseURL(ctx context.Context, task *model.Task, body []byte, format VideoResponseFormat) ([]byte, error)
```

`ResolveTaskResultURL` 有对象键时从当前运行时配置创建/复用 Store 并按 `ExpiresSeconds` 重新签名，没有对象键时回退 `task.GetResultURL()`。

- [ ] **Step 4: Implement bounded streaming download and transfer orchestration**

在 `service/video_result_storage.go` 实现：

```go
func ProcessVideoResultURL(ctx context.Context, task *model.Task, sourceURL string) error
```

流程固定为：读取快照 -> `pkg/objectstorage.ShouldTransfer` -> 非转存写 `ResultURL` 并清空对象键 -> 转存路径调用 `ValidateSSRFProtectedFetchURL`、`GetSSRFProtectedHTTPClient`、响应状态/MIME/Content-Length 校验、`io.LimitReader` 流式上传、Head 校验、写对象键字段。错误包装为可分类的 `video_result_storage_error`，不在错误文本中保留完整 URL。

- [ ] **Step 5: Run service/model tests and commit**

Run: `go test ./model ./service -run 'Test(TaskPrivateData|ProcessVideoResult|ResolveTaskResultURL)' -count=1`

Expected: PASS。

Commit: `feat: add video result transfer service`

## Task 4: 接入标准视频终态与动态公共响应

**Files:**
- Modify: `service/task_polling.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/seedance_task.go`
- Modify: `service/task_response_audit.go`
- Modify: `service/public_task.go`
- Modify: `controller/video_proxy.go`
- Modify: `controller/video_proxy_gemini.go`
- Test: `service/task_polling_test.go`
- Test: `relay/relay_task_seedance_test.go`
- Test: `service/public_task_test.go`

- [ ] **Step 1: Add failing scheduled-polling regression tests**

在现有 `service/task_polling_test.go` 添加 whitelisted success case：上游返回 `video/mp4` URL，fake store 记录上传，断言任务保存 `ResultObjectKey`、对外任务 DTO 使用新签名 URL，不含上游 Host。再添加黑名单/default case，断言不上传且保持原始 URL；添加上传失败 case，断言任务状态为 `FAILURE`、quota 进入现有退款路径。

- [ ] **Step 2: Add failing realtime and response-rewrite tests**

在 relay 测试中覆盖 `tryRealtimeFetch` 成功时使用同一转存函数。响应重写测试分别断言：

```json
{"metadata":{"url":"https://storage.example/doubao-seedance-2-0/task_public.mp4?X-Amz-Expires=86400"}}
```

和：

```json
{"content":{"video_url":"https://storage.example/doubao-seedance-2-0/task_public.mp4?X-Amz-Expires=86400"}}
```

原始上游 URL 只能存在管理员审计/原始数据，不能出现在普通用户结果字段。

- [ ] **Step 3: Run tests to verify failure**

Run: `go test ./service ./relay -run 'Test.*(ObjectStorage|ResultURL|RealtimeFetch|TaskPolling)' -count=1`

Expected: FAIL until the polling and response paths call the new service functions。

- [ ] **Step 4: Extract and reuse the complete terminal transition**

从 `updateVideoSingleTask` 提取导出的统一终态函数：

```go
func ApplyVideoTaskResult(
	ctx context.Context,
	adaptor TaskPollingAdaptor,
	task *model.Task,
	taskResult *relaycommon.TaskInfo,
) error
```

该函数在成功分支写入 `taskResult.Url` 前调用 `ProcessVideoResultURL`；转存错误时把结果转换成既有失败状态，再执行 `preparePolledTaskCostSettlement`、`UpdateWithStatus`、结算/退款和终态审计。保持 CAS 获胜者才执行结算、退款和审计的现有不变量。

`updateVideoSingleTask` 负责解析 HTTP 响应后调用 `ApplyVideoTaskResult`。`tryRealtimeFetch` 也调用同一函数，避免实时查询提前写成功状态而跳过计费；如果转存失败，返回失败格式且不能返回原始 URL。

- [ ] **Step 5: Rewrite every public result boundary**

实现 `RewriteVideoResponseURL` 并在 `relay/relay_task.go` 的 OpenAI/ARK converter 返回前、`relay/seedance_task.go` 的 Seedance payload 返回前、`service.persistPolledTerminalTaskUserResponse` 审计前调用。将 `TaskModel2Dto`、`ProjectPublicTask`、两个视频代理 controller 的 `task.GetResultURL()` 替换为 `ResolveTaskResultURL`。

JSON 改写只使用 `common.Unmarshal`/`common.Marshal`，不直接调用 `encoding/json` 的 marshal/unmarshal。

- [ ] **Step 6: Run focused and broad backend tests**

Run:

```text
go test ./service ./relay ./controller -run 'Test.*(ObjectStorage|ResultURL|TaskPolling|PublicTask|VideoProxy)' -count=1
go test ./service ./relay ./controller ./model -count=1
```

Expected: PASS，且既有视频响应、审计、CAS 和退款回归测试保持通过。

- [ ] **Step 7: Commit**

Commit: `feat: transfer standard video results before terminal response`

## Task 5: 管理员对象存储 API 与路由

**Files:**
- Create: `controller/object_storage.go`
- Test: `controller/object_storage_test.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: Write API contract tests**

使用现有 RootAuth/controller 测试 fixture 添加：

- 未认证或非 root 请求返回 401/403。
- GET 返回所有非敏感配置和 `secret_configured`，不返回 Secret Access Key。
- PUT 启用但缺 Endpoint/Bucket/凭据/`ExpiresSeconds` 越界返回 400，数据库 Option 不变。
- PUT Secret 留空保留旧值，`clear_secret=true` 清除旧值。
- PUT 合法完整配置调用 `model.UpdateOptionsBulk` 一次提交并更新运行时快照。
- POST test 使用未保存配置，成功时 probe 写入/Head/Presign/Delete，失败时返回脱敏错误且不保存。

- [ ] **Step 2: Run API tests to verify failure**

Run: `go test ./controller -run 'TestObjectStorage' -count=1`

Expected: FAIL because controller handlers and routes are not defined。

- [ ] **Step 3: Implement request/response DTOs and handler validation**

请求结构固定为：

```go
type ObjectStorageSettingsRequest struct {
	Enabled                   bool     `json:"enabled"`
	Endpoint                  string   `json:"endpoint"`
	PublicEndpoint            string   `json:"public_endpoint"`
	Region                    string   `json:"region"`
	Bucket                    string   `json:"bucket"`
	AccessKeyID               string   `json:"access_key_id"`
	SecretAccessKey           string   `json:"secret_access_key"`
	UsePathStyle              bool     `json:"use_path_style"`
	MaxVideoSizeMB            int      `json:"max_video_size_mb"`
	ExpiresSeconds            int      `json:"expires_seconds"`
	TransferDomainWhitelist   []string `json:"transfer_domain_whitelist"`
	NoTransferDomainBlacklist []string `json:"no_transfer_domain_blacklist"`
	ClearSecret               bool     `json:"clear_secret"`
}
```

GET 先从 `object_storage.Runtime()` 构造响应，Secret 只输出 `SecretConfigured`。PUT 读取当前 Secret 处理留空保留/明确清除，再调用 `ValidateConfig`、序列化列表字段为 JSON，最后调用已有 `model.UpdateOptionsBulk`。

- [ ] **Step 4: Implement test endpoint without persistence**

POST test 将请求转换为临时 `pkg/objectstorage.Config`，如果 Secret 留空且当前配置已有 Secret，使用当前 Secret；调用 `New` 和 `Probe`，成功返回 `{success:true, data:{presign_supported:true}}`。错误只返回稳定分类消息和被 `common.MaskSensitiveInfo` 清理后的诊断。

- [ ] **Step 5: Register RootAuth routes and add router tests**

在 `router/api-router.go` 增加：

```go
objectStorageRoute := apiRouter.Group("/object-storage")
objectStorageRoute.Use(middleware.RootAuth())
{
	objectStorageRoute.GET("/settings", controller.GetObjectStorageSettings)
	objectStorageRoute.PUT("/settings", controller.UpdateObjectStorageSettings)
	objectStorageRoute.POST("/test", controller.TestObjectStorageSettings)
}
```

为三个方法增加路由表断言。

- [ ] **Step 6: Run tests and commit**

Run: `go test ./controller ./router -run 'Test.*ObjectStorage' -count=1`

Expected: PASS。

Commit: `feat: add object storage admin API`

## Task 6: Operations 单页配置表单

**Files:**
- Modify: `web/src/features/system-settings/types.ts`
- Modify: `web/src/features/system-settings/api.ts`
- Create: `web/src/features/system-settings/operations/object-storage-section.tsx`
- Modify: `web/src/features/system-settings/operations/section-registry.tsx`
- Modify: `web/src/features/system-settings/operations/index.tsx`
- Create: `web/src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`

- [ ] **Step 1: Define API/domain types and write failing component tests**

在 `types.ts` 增加：

```ts
export type ObjectStorageSettings = {
  enabled: boolean
  endpoint: string
  public_endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_configured: boolean
  use_path_style: boolean
  max_video_size_mb: number
  expires_seconds: number
  transfer_domain_whitelist: string[]
  no_transfer_domain_blacklist: string[]
}
```

在专属 `__tests__` 中先写用户视角测试：页面显示连接/凭据/链接/域名四组字段；启用后空 Endpoint/Bucket/ExpiresSeconds 越界显示错误；Secret 输入留空时 PUT 不带清除标记；点击清除后发送 `clear_secret:true`；Test Connection 成功/失败显示对应 toast。

- [ ] **Step 2: Run frontend test to verify failure**

Run from `web/`: `bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`

Expected: FAIL because API functions and component are not defined。

- [ ] **Step 3: Implement React Query API helpers**

在 `api.ts` 增加 `getObjectStorageSettings`、`updateObjectStorageSettings`、`testObjectStorageSettings`，分别调用 `/api/object-storage/settings` GET/PUT 和 `/api/object-storage/test` POST；使用现有 `api` 实例并返回 `res.data`。

- [ ] **Step 4: Implement the single-page form**

在 `object-storage-section.tsx` 使用 `useQuery`/`useMutation` 和 React Hook Form + Zod。表单内部字段用嵌套对象避免 RHF dotted-name 问题；域名数组用 `Textarea` 逐行解析；`Secret Access Key` 默认空字符串，依据 `secret_configured` 显示状态 badge；清除操作用确认对话框。

校验规则：启用时 Endpoint/Public Endpoint/Bucket/Access Key/Secret（除非已配置）必填；`max_video_size_mb` 为 1-2048 整数；`expires_seconds` 为 60-604800 整数；域名条目只能是精确 Host 或 `*.` 子域模式。保存时发送全量非敏感字段、Secret 输入、`clear_secret`。

按钮使用现有图标库：保存使用 Save、测试使用 Plug、清除密钥使用 Trash2，并为图标按钮提供 `aria-label`/tooltip。使用 `SettingsSection`、`SettingsForm`、`SettingsPageFormActions` 保持 Operations 现有布局。

- [ ] **Step 5: Register the section**

在 `section-registry.tsx` 添加：

```tsx
{
  id: 'object-storage',
  titleKey: 'Object Storage',
  build: () => <ObjectStorageSection />,
},
```

将 `OperationsSectionId` 自动纳入 registry，并在 `OperationsSettings` 保留现有默认配置；该 section 的数据来自专用 API，不把 Secret 放进通用 `/api/option/` settings。

- [ ] **Step 6: Run component tests, typecheck and lint**

Run from `web/`:

```text
bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/system-settings/operations/object-storage-section.tsx src/features/system-settings/operations/__tests__/object-storage-section.test.tsx
```

Expected: PASS，且无 TypeScript/lint error。

- [ ] **Step 7: Commit**

Commit: `feat: add object storage settings page`

## Task 7: 前端 i18n 完整性与翻译同步

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: Invoke the i18n translation skill before editing locale files**

读取并遵循 `i18n-translate/SKILL.md`，先从组件提取英文 source keys，再按项目约定补齐七种 flat JSON locale。

- [ ] **Step 2: Add all Object Storage keys to en and translated locales**

至少覆盖：`Object Storage`、`Enable video result transfer`、`S3 Endpoint`、`Public Endpoint`、`Region`、`Bucket`、`Access Key ID`、`Secret Access Key`、`Path Style`、`ExpiresSeconds`、`Max video size (MB)`、`Transfer domain whitelist`、`No-transfer domain blacklist`、`Test connection`、`Connection test succeeded`、`Connection test failed`、`Secret is configured`、`Clear secret`、字段范围错误、保存成功/失败和“黑名单优先”说明。

- [ ] **Step 3: Run sync and locale tests**

Run from `web/`: `bun run i18n:sync`。

Expected: keys are present in all seven locale files with no missing-key report. Follow with `bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`。

- [ ] **Step 4: Commit**

Commit: `feat: translate object storage settings`

## Task 8: 集成验证、文档和发布前检查

**Files:**
- Modify: `docs/superpowers/specs/2026-08-07-video-result-s3-object-storage-design.md` only if implementation behavior requires a documented correction.
- Test: all affected Go and frontend tests.

- [ ] **Step 1: Run focused backend suites**

```text
go test ./setting/object_storage ./pkg/objectstorage -count=1
go test ./model ./service -run 'Test.*(ObjectStorage|VideoResult|TaskPolling|TaskPrivateData)' -count=1
go test ./relay ./controller ./router -run 'Test.*(ObjectStorage|ResultURL|VideoProxy|TaskModel2Dto)' -count=1
```

Expected: PASS。

- [ ] **Step 2: Run frontend verification**

From `web/`:

```text
bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx
bun run typecheck
bun run lint
bun run format:check
bun run build
```

Expected: all commands exit 0。

- [ ] **Step 3: Run full backend regression**

Run: `go test ./...`

Expected: PASS. If an existing unrelated user change causes a failure, record the exact package/test and do not revert that change。

- [ ] **Step 4: Manual S3-compatible smoke check**

Use a local MinIO or equivalent S3-compatible endpoint with a private bucket and a public endpoint that accepts SigV4. Configure whitelist with the mock upstream host, submit a standard video task, and verify:

1. S3 contains `<OriginModelName>/<PublicTaskID>.mp4`.
2. API response contains `X-Amz-Expires=86400` and no upstream Host.
3. Re-querying the task produces a new signature.
4. Blacklist/default URLs do not create objects.
5. Forced upload error marks task failed and refunds quota.

- [ ] **Step 5: Review diff and commit verification metadata**

Run: `git diff --check` and `git status --short`。不要提交数据库、`.superpowers` 视觉 companion 内容或用户未提交的 Ark 文件。

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-07-video-result-s3-object-storage.md`. Two execution options:

1. **Subagent-Driven (recommended)** - dispatch a fresh subagent per task and review between tasks.
2. **Inline Execution** - execute tasks in this session with checkpointed batches.

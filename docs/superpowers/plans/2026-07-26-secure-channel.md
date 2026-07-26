# Secure Seedance 渠道实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用一个 Secure 渠道类型和一套 task adaptor 接入特价、海外、企业三个 Video 分组；管理员用三枚分组 Key 创建三个普通渠道，用户继续通过 Ark SDK 的创建、单查和列表接口调用。

**Architecture:** `secure_video_group` 是 Secure 渠道专属的渠道级枚举，由 adaptor 初始化时选择不可变 profile。三个 profile 共享鉴权、公开任务 ID、响应投影、轮询和计费；特价/海外只实现各自 multipart 编码，企业实现 `/v1/videos` JSON 编码。后台轮询从渠道记录重新加载该枚举，确保同一渠道提交和查询始终使用同一分组协议。

**Tech Stack:** Go 1.22+、Gin、GORM v2、testify、`mime/multipart`、React 19、TypeScript、Zod、Base UI、i18next、Bun。

**Design:** `docs/superpowers/specs/2026-07-26-secure-channel-design.md`

**Ark Contract:** 用户代码不变，统一覆盖 `POST /api/v3/contents/generations/tasks`、`GET /api/v3/contents/generations/tasks/{id}` 和 `GET /api/v3/contents/generations/tasks`。

**Prerequisite:** 严格依次完成 Lucen、MegaByAI、苍原和派普计划。派普占用类型 65 并把 Dummy 移到 66；本计划使用类型 66，最终 `ChannelTypeDummy = 67`。

---

## 文件结构

### 新增文件

- `relay/channel/task/newapivideo/secure_request.go`：三分组校验、multipart/JSON 编码和海外 `@*_file_N` 引用补齐。
- `relay/channel/task/newapivideo/secure_request_test.go`：三种协议、边界和 Content-Type 测试。
- `web/src/features/channels/lib/secure-video-group.ts`：Secure 分组枚举、可见性和表单选项。
- `web/src/features/channels/lib/secure-video-group.test.ts`：可见性、保存和清理行为测试。
- `e2e/secure_upstream_e2e_test.go`：三分组 Ark 生命周期。
- `e2e/secure_routing_e2e_test.go`：三渠道分组能力和 Key 隔离。
- `docs/superpowers/reports/2026-07-26-secure-channel-acceptance.md`：三枚真实 Key 的脱敏验收报告。

### 修改文件

- `dto/channel_settings.go`、`dto/channel_settings_test.go`：`SecureVideoGroup` 类型、枚举和渠道类型校验。
- `controller/channel.go`、`controller/channel_test_internal_test.go`：新增/编辑保存边界。
- `web/src/features/channels/types.ts`、`lib/channel-form.ts`、`components/drawers/channel-mutate-drawer.tsx`：条件显示、必填、载入、保存和切换清理。
- `relay/channel/task/newapivideo/profile.go`、`adaptor.go`：Secure profile 选择、动态端点、Content-Type 和轮询路径。
- `service/task_polling.go`、`service/task_polling_test.go`：轮询初始化时携带渠道 `ChannelOtherSettings`。
- `constant/channel.go`、`constant/channel_test.go`、`relay/relay_adaptor.go`、`relay/seedance_task.go`、`relay/relay_task.go`、`relay/relay_task_seedance_test.go`、`relay/cost_accounting_adaptor_test.go`：注册类型 66 和 task-only 能力。
- `controller/channel-test.go`、`controller/channel_test_internal_test.go`：通用聊天测试排除。
- `web/src/features/channels/constants.ts`、`lib/channel-type-config.ts`、`lib/channel-utils.ts`、`web/tests/channel-type-config.test.ts`：Secure 渠道目录。
- `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`：渠道和分组文案。
- `relay/relay_task_billing_test.go`、`relay/cost_accounting_adaptor_test.go`：三分组计费覆盖。

---

### Task 1: 定义并强制 Secure 渠道分组配置

**Files:**
- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `dto/channel_settings.go`
- Modify: `dto/channel_settings_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: 写 DTO 枚举失败测试**

先在 `constant/channel_test.go` 断言类型 66、最终 Dummy 67、默认 URL 和名称；该常量必须在本任务落地，因为 DTO 校验会直接引用它：

```go
func TestSecureChannelConstants(t *testing.T) {
	require.Equal(t, 66, constant.ChannelTypeSecure)
	require.Equal(t, 67, constant.ChannelTypeDummy)
	require.Equal(t, "https://token.secure-skill.com", constant.ChannelBaseURLs[constant.ChannelTypeSecure])
	require.Equal(t, "Secure", constant.GetChannelTypeName(constant.ChannelTypeSecure))
}
```

```go
func TestChannelOtherSettingsValidateSecureVideoGroup(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		group       dto.SecureVideoGroup
		wantErr     string
	}{
		{name: "discount", channelType: constant.ChannelTypeSecure, group: dto.SecureVideoGroupDiscount},
		{name: "overseas", channelType: constant.ChannelTypeSecure, group: dto.SecureVideoGroupOverseas},
		{name: "enterprise", channelType: constant.ChannelTypeSecure, group: dto.SecureVideoGroupEnterprise},
		{name: "missing", channelType: constant.ChannelTypeSecure, wantErr: "secure_video_group is required"},
		{name: "unknown", channelType: constant.ChannelTypeSecure, group: "other", wantErr: "secure_video_group must be one of"},
		{name: "leak to other type", channelType: constant.ChannelTypeOpenAI, group: dto.SecureVideoGroupDiscount, wantErr: "secure_video_group is only valid for Secure"},
		{name: "empty on other type", channelType: constant.ChannelTypeOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := dto.ChannelOtherSettings{SecureVideoGroup: tt.group}
			err := settings.ValidateSecureVideoGroup(tt.channelType)
			if tt.wantErr == "" { require.NoError(t, err); return }
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
```

- [ ] **Step 2: 写新增和编辑渠道失败测试**

在 `controller/channel_test_internal_test.go` 对 `validateChannel` 覆盖：Secure 缺失、未知枚举、合法三值和非 Secure 携带该字段。编辑路径使用现有 Secure 渠道改成 OpenAI 但保留 settings，必须失败，证明隐藏配置不能绕过前端清理。

- [ ] **Step 3: 运行测试并确认失败**

```powershell
go test ./dto ./controller -run 'TestChannelOtherSettingsValidateSecure|TestValidateSecure' -count=1
```

Expected: FAIL，枚举、类型 66 和校验方法尚不存在。

- [ ] **Step 4: 预留类型 66 并实现稳定枚举和校验**

在派普后增加 `ChannelTypeSecure = 66`，把 Dummy 调整为 67；给 `ChannelBaseURLs[66]` 增加 `https://token.secure-skill.com`，给 `ChannelTypeNames` 增加 `Secure`。本任务只建立供保存校验使用的身份常量，task adaptor 注册留到 Task 4。

在 `dto/channel_settings.go` 增加：

```go
type SecureVideoGroup string

const (
	SecureVideoGroupDiscount   SecureVideoGroup = "discount"
	SecureVideoGroupOverseas   SecureVideoGroup = "overseas"
	SecureVideoGroupEnterprise SecureVideoGroup = "enterprise"
)

func (s ChannelOtherSettings) ValidateSecureVideoGroup(channelType int) error {
	group := SecureVideoGroup(strings.TrimSpace(string(s.SecureVideoGroup)))
	if channelType != constant.ChannelTypeSecure {
		if group != "" {
			return fmt.Errorf("secure_video_group is only valid for Secure channels")
		}
		return nil
	}
	switch group {
	case SecureVideoGroupDiscount, SecureVideoGroupOverseas, SecureVideoGroupEnterprise:
		return nil
	case "":
		return fmt.Errorf("secure_video_group is required for Secure channels")
	default:
		return fmt.Errorf("secure_video_group must be one of discount, overseas, enterprise")
	}
}
```

并给 `ChannelOtherSettings` 增加：

```go
SecureVideoGroup SecureVideoGroup `json:"secure_video_group,omitempty"`
```

- [ ] **Step 5: 接入服务端保存边界**

在 `validateChannel` 的 `ValidateSettings()` 之后加入：

```go
settings := channel.GetOtherSettings()
if err := settings.ValidateSecureVideoGroup(channel.Type); err != nil {
	return err
}
```

新增和更新都已调用 `validateChannel`，不要只在新增 handler 中校验。不要自动把未知值改成默认分组，否则错误 Key 会被用于错误协议。

- [ ] **Step 6: 格式化、测试并提交**

```powershell
gofmt -w constant/channel.go constant/channel_test.go dto/channel_settings.go dto/channel_settings_test.go controller/channel.go controller/channel_test_internal_test.go
go test ./constant ./dto ./controller -run 'TestSecureChannelConstants|TestChannelOtherSettingsValidateSecure|TestValidateSecure' -count=1
git add constant/channel.go constant/channel_test.go dto/channel_settings.go dto/channel_settings_test.go controller/channel.go controller/channel_test_internal_test.go
git commit -m "feat(channel): validate Secure video groups"
```

Expected: PASS。

---

### Task 2: 仅在 Secure 渠道表单显示分组选择

**Files:**
- Modify: `web/src/features/channels/types.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Create: `web/src/features/channels/lib/secure-video-group.ts`
- Create: `web/src/features/channels/lib/secure-video-group.test.ts`
- Modify: `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] **Step 1: 写可见性、必填和 payload 清理失败测试**

```ts
test('shows the group field only for Secure', () => {
  expect(shouldShowSecureVideoGroup(66)).toBe(true)
  expect(shouldShowSecureVideoGroup(65)).toBe(false)
  expect(shouldShowSecureVideoGroup(1)).toBe(false)
})

test('requires a group for Secure', () => {
  const result = channelFormSchema.safeParse({
    ...CHANNEL_FORM_DEFAULT_VALUES,
    type: 66,
    name: 'Secure', key: 'secret', models: 'video-2.0-pro',
  })
  expect(result.success).toBe(false)
  expect(result.error?.issues).toContainEqual(expect.objectContaining({ path: ['secure_video_group'] }))
})

test('persists the selected group and removes it after changing type', () => {
  const secure = transformFormDataToCreatePayload({
    ...CHANNEL_FORM_DEFAULT_VALUES,
    type: 66, name: 'Secure enterprise', key: 'secret',
    models: 'video-2.0-pro', secure_video_group: 'enterprise',
  })
  expect(JSON.parse(secure.channel.settings).secure_video_group).toBe('enterprise')

  const other = transformFormDataToCreatePayload({
    ...CHANNEL_FORM_DEFAULT_VALUES,
    type: 1, name: 'OpenAI', key: 'secret', models: 'gpt-4o',
    settings: secure.channel.settings, secure_video_group: undefined,
  })
  expect(JSON.parse(other.channel.settings)).not.toHaveProperty('secure_video_group')
})
```

- [ ] **Step 2: 运行测试并确认失败**

```powershell
Set-Location web
bun test src/features/channels/lib/secure-video-group.test.ts
Set-Location ..
```

Expected: FAIL，字段、helper 和保存逻辑尚不存在。

- [ ] **Step 3: 定义前端枚举和表单转换**

在 `types.ts` 给 `ChannelOtherSettings` 增加 `secure_video_group?: 'discount' | 'overseas' | 'enterprise'`。创建：

```ts
export const SECURE_CHANNEL_TYPE = 66
export const SECURE_VIDEO_GROUP_OPTIONS = [
  { value: 'discount', label: 'Discount Video' },
  { value: 'overseas', label: 'Overseas Video' },
  { value: 'enterprise', label: 'Enterprise Video' },
] as const

export function shouldShowSecureVideoGroup(type: number): boolean {
  return type === SECURE_CHANNEL_TYPE
}
```

在 `channelFormSchema` 增加可选枚举，并在 `superRefine` 对类型 66 缺失值添加 `secure_video_group` required issue。默认值为 `undefined`，编辑载入时只对 Secure 读取已保存值。

在 `buildSettingsJSON` 增加：

```ts
if (formData.type === SECURE_CHANNEL_TYPE && formData.secure_video_group) {
  settingsObj.secure_video_group = formData.secure_video_group
} else if ('secure_video_group' in settingsObj) {
  delete settingsObj.secure_video_group
}
```

- [ ] **Step 4: 在抽屉中条件渲染 Select**

在现有渠道类型专属字段区域加入 `FormField name='secure_video_group'`，只在 `shouldShowSecureVideoGroup(currentType)` 时渲染。使用 Base UI `Select` 和三个固定选项；标签为 `Secure video group`，说明为 `Use the API key issued for the selected Secure group`。

渠道类型选择的 `onValueChange` 在新类型不是 66 时执行：

```ts
form.setValue('secure_video_group', undefined, {
  shouldDirty: true,
  shouldValidate: false,
})
```

隐藏字段和最终 payload 都清理，不能只依赖 CSS 隐藏。

- [ ] **Step 5: 添加七语言文案并验证**

新增并翻译 `Secure video group`、`Discount Video`、`Overseas Video`、`Enterprise Video`、`Use the API key issued for the selected Secure group`。简体中文分别为“Secure 视频分组”“特价 Video”“海外 Video”“企业 Video”“使用所选 Secure 分组签发的 API Key”；其余语言不得保留英文占位。

```powershell
Set-Location web
bun test src/features/channels/lib/secure-video-group.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
Set-Location ..
git add web/src/features/channels/types.ts web/src/features/channels/lib/channel-form.ts web/src/features/channels/lib/secure-video-group.ts web/src/features/channels/lib/secure-video-group.test.ts web/src/features/channels/components/drawers/channel-mutate-drawer.tsx web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(web): configure Secure video groups"
```

Expected: PASS。类型 66 显示选择器，其他类型不显示且 payload 不含 `secure_video_group`。

---

### Task 3: 实现三种 Secure 请求 profile

**Files:**
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Create: `relay/channel/task/newapivideo/secure_request.go`
- Create: `relay/channel/task/newapivideo/secure_request_test.go`

- [ ] **Step 1: 写 profile 选择和端点失败测试**

```go
func TestSecureProfiles(t *testing.T) {
	tests := []struct {
		group       dto.SecureVideoGroup
		submitPath  string
		pollPath    string
		dialect     videoRequestDialect
	}{
		{dto.SecureVideoGroupDiscount, "/api/generate-video", "/api/task/{task_id}", videoRequestDialectSecureDiscount},
		{dto.SecureVideoGroupOverseas, "/api/generate-video", "/api/task/{task_id}", videoRequestDialectSecureOverseas},
		{dto.SecureVideoGroupEnterprise, "/v1/videos", "/v1/videos/{task_id}", videoRequestDialectSecureEnterprise},
	}
	for _, tt := range tests {
		t.Run(string(tt.group), func(t *testing.T) {
			profile, err := secureProtocolProfile(tt.group)
			require.NoError(t, err)
			assert.Equal(t, "Secure", profile.channelName)
			assert.Equal(t, tt.submitPath, profile.submitPath)
			assert.Equal(t, tt.pollPath, profile.pollPath)
			assert.Equal(t, tt.dialect, profile.requestDialect)
		})
	}
	_, err := secureProtocolProfile("unknown")
	require.Error(t, err)
}
```

另测 `NewSecureTaskAdaptor().GetModelList()` 精确返回 `video-2.0-fast`、`video-2.0-mini`、`video-2.0-pro`，且调用方修改返回 slice 不会污染后续结果。

- [ ] **Step 2: 写三种精确请求体失败测试**

特价输入包含两图、一视频、一音频，读取 multipart part 后必须得到以下有序键值：

```go
[][2]string{
	{"model", "video-2.0-pro"},
	{"prompt", "产品特写"},
	{"duration", "8"},
	{"ratio", "16:9"},
	{"resolution", "720p"},
	{"files", "https://x/a.jpg"},
	{"files", "https://x/b.jpg"},
	{"video_urls", "https://x/ref.mp4"},
	{"audio_urls", "https://x/ref.mp3"},
}
```

海外 omni 输入断言 `functionMode=omni_reference`，字段依次为 `image_file_1`、`video_file_1`、`audio_file_1`，且原 prompt `按视频节奏剪辑` 变为：

```text
按视频节奏剪辑 @image_file_1 @video_file_1 @audio_file_1
```

若原 prompt 已含 `@video_file_1`，只能补另外两个引用，不能重复。严格首尾帧输入断言 `functionMode=first_last_frames`，首帧和尾帧分别为 `image_file_1`、`image_file_2`，不写 omni 引用。

企业输入断言：

```json
{
  "model":"video-2.0-pro",
  "prompt":"企业多模态",
  "duration":8,
  "aspect_ratio":"16:9",
  "image_url":"https://x/main.jpg",
  "extra_images":["https://x/extra.jpg"],
  "extra_videos":["https://x/ref.mp4"],
  "extra_audios":["https://x/ref.mp3"]
}
```

企业请求不能出现 `ratio`、`resolution`、`files` 或 `functionMode`。

- [ ] **Step 3: 写能力矩阵失败测试**

按下表逐项构造 Ark 请求，断言准确的 `arkRequestError.Code`：

| 分组 | 非法输入 | 错误码 |
| --- | --- | --- |
| 特价 | 纯文生或 0 图 | `InvalidParameter.content` |
| 特价 | `last_frame` | `InvalidParameter.content` |
| 特价 | 视频+音频超过 3 | `InvalidParameter.content` |
| 特价 | 3 秒或 16 秒 | `InvalidParameter.duration` |
| 特价 | `1:1` | `InvalidParameter.ratio` |
| 特价 | fast/mini + 1080p，pro + 非 720p/1080p/4k | `InvalidParameter.resolution` |
| 海外 | 图片超过 9、视频超过 3、音频超过 3或总素材超过 12 | `InvalidParameter.content` |
| 海外 | 首尾帧模式混入视频/音频 | `InvalidParameter.content` |
| 海外 | 3 秒或 16 秒、未知比例、fast/mini + 1080p、任意 4k | 对应参数错误码 |
| 企业 | 非 `video-2.0-pro`、非 720p、缺少 duration、4 秒或 16 秒 | 对应参数错误码 |
| 企业 | `last_frame` | `InvalidParameter.content` |
| 三组 | 显式 `watermark`、`generate_audio`、非默认 service tier、draft、tools | 对应字段错误码 |

所有媒体继续使用 profile-aware `relaycommon.ParseTaskMediaURL`，Secure 只接受可抓取的 HTTP(S) URL；`data:` 与 `asset://` 返回 `InvalidParameter.content`。

请求自身即可判断的非法项通过 `ValidateRequestAndSetAction` 测试；依赖上游模型的模型/分辨率矩阵通过模型映射后的 `ValidateBillingRequest` 测试。两类测试都断言 HTTP 400，且不调用 `BuildRequestBody`、不产生任务记录、不改变用户 quota。

- [ ] **Step 4: 写海外参考视频总时长测试**

复用 MegaByAI Task 1 已增加的 `service.ResolveReferenceVideoDurationMS`，不要再创建第二套 metadata 入口。Secure 校验测试注入 fake `VideoMetadataClient`：两个 URL 的 metadata 时长 9000 ms 和 6000 ms 返回 15000；任一无效媒体返回 `VideoMetadataInvalidMedia`；服务不可用返回 `VideoMetadataUnavailable`。错误和日志不得包含完整签名 URL。

在 Secure 校验测试中把总时长 15000 ms 设为通过、15001 ms 设为 `InvalidParameter.content`。metadata 服务不可用必须转换为 HTTP 503 的 `reference_video_metadata_unavailable`，且上游请求计数为 0，不能把未知时长当成 0。

- [ ] **Step 5: 运行测试并确认失败**

```powershell
go test ./relay/channel/task/newapivideo ./service -run 'TestSecure|TestResolveReferenceVideoDuration' -count=1
```

Expected: FAIL，Secure dialect、编码器和公开 metadata 入口尚不存在。

- [ ] **Step 6: 定义 profile 和初始化选择**

在 `profile.go` 增加三个 dialect 和：

```go
var secureModels = []string{"video-2.0-fast", "video-2.0-mini", "video-2.0-pro"}

func secureProtocolProfile(group dto.SecureVideoGroup) (protocolProfile, error) {
	profile := protocolProfile{
		channelName: "Secure",
		modelList: append([]string(nil), secureModels...),
	}
	switch group {
	case dto.SecureVideoGroupDiscount:
		profile.submitPath = "/api/generate-video"
		profile.pollPath = "/api/task/{task_id}"
		profile.requestDialect = videoRequestDialectSecureDiscount
	case dto.SecureVideoGroupOverseas:
		profile.submitPath = "/api/generate-video"
		profile.pollPath = "/api/task/{task_id}"
		profile.requestDialect = videoRequestDialectSecureOverseas
	case dto.SecureVideoGroupEnterprise:
		profile.submitPath = "/v1/videos"
		profile.pollPath = "/v1/videos/{task_id}"
		profile.contentType = "application/json"
		profile.requestDialect = videoRequestDialectSecureEnterprise
	default:
		return protocolProfile{}, fmt.Errorf("invalid secure_video_group: %s", group)
	}
	return profile.normalized(), nil
}

func NewSecureTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: protocolProfile{
		channelName: "Secure",
		modelList: append([]string(nil), secureModels...),
	}}
}
```

给 `TaskAdaptor` 增加 `profileErr error` 和 `requestContentType string`。`Init` 仍设置 Key/Base URL；当 channel name 为 Secure 时从 `info.ChannelOtherSettings.SecureVideoGroup` 选择 profile，失败保存到 `profileErr`。每次 Init 清空 `requestContentType`，避免实例复用残留 multipart boundary。

- [ ] **Step 7: 实现纯校验和请求 DTO**

`secure_request.go` 定义 `secureRequestProfile`、`secureEnterpriseRequest` 和 `validateSecureRequest(request, profile, upstreamModel)`。校验严格采用 Step 3 矩阵；模型/分辨率组合用显式 switch，不用字符串包含猜测。`upstreamModel` 为空时只校验不依赖模型映射的请求边界，非空时额外执行模型/分辨率矩阵。企业 DTO 使用指针 duration：

```go
type secureEnterpriseRequest struct {
	Model        string   `json:"model"`
	Prompt       string   `json:"prompt"`
	Duration     *int     `json:"duration"`
	AspectRatio  string   `json:"aspect_ratio,omitempty"`
	ImageURL     string   `json:"image_url,omitempty"`
	ExtraImages  []string `json:"extra_images,omitempty"`
	ExtraVideos  []string `json:"extra_videos,omitempty"`
	ExtraAudios  []string `json:"extra_audios,omitempty"`
}
```

特价和海外用 `multipart.NewWriter` 按 Step 2 的稳定顺序调用 `WriteField`。返回 `writer.FormDataContentType()`，不手写 boundary。引用补齐规则为：按图片、视频、音频顺序生成实际字段序号；仅当 `!strings.Contains(prompt, marker)` 时在 prompt 末尾追加一个空格和 marker。

- [ ] **Step 8: 在预扣前完成 profile、请求、映射模型和视频时长校验**

在 `ValidateRequestAndSetAction` 的 Ark 分支中，先检查 `profileErr`；缺失或未知分组返回 500 `invalid_secure_channel_config`，不得继续解析或发请求。随后调用 profile-aware `validateARKRequest`，读取已解析 state，并用空 `upstreamModel` 调用 `validateSecureRequest`，完成分组、素材、控制字段、时长、比例和请求分辨率等校验。海外存在参考视频时调用上述 metadata 入口；总时长超过 15000 ms 返回 400，metadata 不可用返回 503。

再实现 `TaskBillingRequestValidator.ValidateBillingRequest`：该 hook 在 `ModelMappedHelper` 之后、价格计算和 `PreConsumeBilling` 之前执行，从 state 取原请求并用 `info.UpstreamModelName` 再次调用 `validateSecureRequest`，专门闭合 enterprise 仅 pro、fast/mini 分辨率上限等映射后约束。任何 `arkRequestError` 保留原错误码并返回 HTTP 400；只有 profile 内部配置损坏返回 500。

- [ ] **Step 9: 编码请求体和动态 Content-Type**

`BuildRequestBody` 按 dialect 调用对应编码器；编码器以映射后的模型调用同一纯校验函数作为防御性复验，但正常 relay 路径不能到这里才首次发现用户错误。multipart 编码器同时把实际 Content-Type 保存到 `a.requestContentType`。`BuildRequestHeader` 使用：

```go
contentType := a.activeProfile().contentType
if a.requestContentType != "" { contentType = a.requestContentType }
req.Header.Set("Content-Type", contentType)
req.Header.Set("Accept", "application/json")
req.Header.Set("Authorization", "Bearer "+a.apiKey)
```

统一使用 Bearer 鉴权；Secure 文档明确特价也支持该方式。`BuildRequestURL` 和 `FetchTask` 继续使用 profile 的 submit/poll path 与 `url.PathEscape(taskID)`。

- [ ] **Step 10: 格式化、测试并提交**

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/secure_request.go relay/channel/task/newapivideo/secure_request_test.go
go test ./relay/channel/task/newapivideo ./service -run 'TestSecure|TestResolveReferenceVideoDuration|TestMegaByAI|TestCangyuan|TestPaipu|TestLucen' -count=1
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/secure_request.go relay/channel/task/newapivideo/secure_request_test.go
git commit -m "feat(video): add Secure group profiles"
```

Expected: PASS。特价/海外请求头包含真实 multipart boundary，企业为 `application/json`。

---

### Task 4: 注册 task-only 平台并让后台轮询恢复分组 profile

**Files:**
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/cost_accounting_adaptor_test.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`
- Modify: `service/task_polling.go`
- Modify: `service/task_polling_test.go`
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] **Step 1: 写 adaptor、task-only 和管理端失败测试**

relay 测试断言 `GetTaskAdaptor("66")` 返回 Secure、实现 `ArkVideoTaskConverter` 与 `TaskCostAccountingAdaptor`、`seedanceTaskPlatformValues()` 包含 `"66"`，并断言 `ChannelType2APIType` 不包含 Secure。前端测试断言：

```ts
expect(CHANNEL_TYPES[66]).toBe('Secure')
expect(getDefaultBaseUrl(66)).toBe('https://token.secure-skill.com')
expect(getChannelTypeConfig(66).supportedModels).toEqual([
  'video-2.0-fast', 'video-2.0-mini', 'video-2.0-pro',
])
expect(TASK_ONLY_CHANNEL_TYPES.has(66)).toBe(true)
expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(66)).toBe(true)
expect(MODEL_FETCHABLE_TYPES.has(66)).toBe(false)
```

- [ ] **Step 2: 写后台轮询配置恢复失败测试**

在 `service/task_polling_test.go` 的 recording adaptor 中让 `Init` 保存 `info.ChannelOtherSettings.SecureVideoGroup`。创建三个独立 subtest，每个缓存渠道设置一个分组并放入一个待轮询任务，断言：

```go
assert.Equal(t, tt.group, adaptor.initializedGroup)
assert.Equal(t, tt.privateTaskID, adaptor.fetchedTaskID)
```

另测缺失分组时 adaptor 返回配置错误且不发 HTTP 请求；该情况用于防御历史脏数据，正常保存已由 Task 1 阻止。

- [ ] **Step 3: 运行失败测试**

```powershell
go test ./constant ./relay ./controller ./service -run 'TestSecure|TestTaskPollingCarriesSecure' -count=1
Set-Location web
bun test tests/channel-type-config.test.ts
Set-Location ..
```

Expected: FAIL，Secure adaptor、任务平台、管理端目录和轮询 settings 传递尚未注册。

- [ ] **Step 4: 注册 Ark 任务平台**

`GetTaskAdaptor` 返回 `newapivideo.NewSecureTaskAdaptor()`。把类型 66 加入 `isSeedanceTaskPlatform`、`seedanceTaskPlatformValues`、Ark converter 强制分支、task cost accounting 测试矩阵和通用聊天渠道测试禁止列表。

- [ ] **Step 5: 给轮询 adaptor 初始化完整渠道配置**

在 `service/task_polling.go` 获取 `cacheGetChannel` 后构造：

```go
info := &relaycommon.RelayInfo{}
info.ChannelMeta = &relaycommon.ChannelMeta{
	ChannelType:          cacheGetChannel.Type,
	ChannelBaseUrl:       cacheGetChannel.GetBaseURL(),
	ChannelOtherSettings: cacheGetChannel.GetOtherSettings(),
}
info.ApiKey = cacheGetChannel.Key
adaptor.Init(info)
```

不能从任务公开 `Data`、`Action` 或上游 ID 推测分组；渠道记录是唯一权威来源。一个 Secure 渠道只绑定一枚 Key 和一个分组，因此同一轮询批次的 profile 不会变化。

- [ ] **Step 6: 注册管理端 Secure 渠道目录**

类型常量、显示顺序、`TASK_ONLY_CHANNEL_TYPES`、`GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES` 和 Key 提示加入 66；不加入 `MODEL_FETCHABLE_TYPES`。配置为：

```ts
{
  id: 66,
  name: CHANNEL_TYPES[66],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://token.secure-skill.com',
  supportedModels: ['video-2.0-fast', 'video-2.0-mini', 'video-2.0-pro'],
  hints: {
    baseUrl: 'Default: https://token.secure-skill.com',
    key: 'Enter the API key issued for the selected Secure video group',
    models: 'Select only models enabled for this Secure group API key',
  },
}
```

加入 managed default URL，图标复用 `NewAPI`。warning 明确“为特价、海外、企业三枚 Key 分别创建三个 Secure 渠道，不要在一条渠道中混用 Key”。七语言都增加实际翻译。

- [ ] **Step 7: 增加公开响应隔离测试**

对平台 66 构造成功与失败任务，断言 Ark 单查/列表不出现 `secure_video_group`、私有任务 ID、渠道 ID、Key、上游模型或 quota。任务所有者的 `content.video_url` 保留播放所需的完整结果 URL；日志捕获器和路由诊断断言只比较去查询串后的 host/path，且不得记录敏感查询参数。

- [ ] **Step 8: 格式化、验证并提交**

```powershell
gofmt -w relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go service/task_polling.go service/task_polling_test.go
go test ./constant ./relay ./controller ./service -run 'TestSecure|TestSeedanceTask|TestTaskPollingCarriesSecure|TestSupportsGenericChannelTest' -count=1
Set-Location web
bun test tests/channel-type-config.test.ts src/features/channels/lib/secure-video-group.test.ts
bun run i18n:sync
bun run typecheck
bun run build
Set-Location ..
git add relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go service/task_polling.go service/task_polling_test.go web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(secure): register grouped Seedance channels"
```

Expected: PASS，最终 Dummy 为 67。

---

### Task 5: 让能力路由在选渠道前区分输入模式和最小素材数

**Files:**
- Modify: `pkg/modelrouting/types.go`
- Modify: `pkg/modelrouting/match.go`
- Modify: `pkg/modelrouting/match_test.go`
- Modify: `pkg/modelrouting/validate.go`
- Modify: `pkg/modelrouting/validate_test.go`
- Modify: `middleware/model_routing.go`
- Modify: `middleware/model_routing_test.go`
- Modify: `service/routing_policy.go`
- Modify: `service/model_routing_test.go`
- Modify: `web/src/features/model-routing/types.ts`
- Modify: `web/tests/model-routing-types.test.ts`
- Modify: `web/src/features/model-routing/components/route-target-editor.tsx`
- Modify: `web/src/features/model-routing/components/route-target-editor-client.test.tsx`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] **Step 1: 写输入模式解析失败测试**

在 middleware 表驱动测试中使用标准 Ark content，断言：

| content | `FactsInput.InputMode` |
| --- | --- |
| 仅 text | `text` |
| 一张 `first_frame` | `first_frame` |
| `first_frame` + `last_frame` | `first_last_frames` |
| 任意 `reference_image`、`reference_video` 或 `reference_audio` | `omni_reference` |

现有互斥校验保持不变，因此混合严格首尾帧与 omni 素材仍在路由前返回 400。

- [ ] **Step 2: 写最小素材和模式匹配失败测试**

```go
discount := modelrouting.Constraints{
	OutputResolutions: []string{"720p", "1080p", "4k"},
	Durations: modelrouting.DurationConstraint{Min: ptr(4), Max: ptr(15)},
	AspectRatios: []string{"16:9", "9:16"},
	InputModes: []modelrouting.InputMode{modelrouting.InputModeFirstFrame, modelrouting.InputModeOmniReference},
	ReferenceMinimums: modelrouting.ReferenceLimits{Images: 1},
	ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
}
```

断言纯文生因 `input_mode` 和 `reference_images` 不匹配；video-only omni 因最小图片数不匹配；一张图片通过。另断言 `input_modes` 为空、minimums 为零的旧目标继续匹配所有已有请求。

- [ ] **Step 3: 写策略校验和重叠失败测试**

覆盖未知 input mode、负 minimum、minimum 大于 maximum。相同渠道/优先级的两个目标只有在 resolution、duration、ratio、input mode 和每类引用数量范围都相交时才算 overlap；例如 text-only 与 first-last-only 不重叠，图片范围 `[1,9]` 与 `[0,0]` 不重叠。

增加两个策略可达性回归：只有一个 discount target、`input_modes=[first_frame,omni_reference]`、`reference_minimums.images=1` 且默认输出为 720p/8s/16:9 时，`ValidatePolicy` 必须成功；`input_modes=[first_last_frames]` 但 `reference_limits.images=1` 时必须失败。前者证明校验不能只用“纯文生、零素材”的旧 synthetic facts，后者证明不能为了放行特价策略而跳过真实可达性检查。

- [ ] **Step 4: 运行后端测试并确认失败**

```powershell
go test ./pkg/modelrouting ./middleware ./service -run 'Test.*InputMode|Test.*ReferenceMinimum|Test.*Routing' -count=1
```

Expected: FAIL，新字段和枚举尚不存在。

- [ ] **Step 5: 定义向后兼容的路由类型**

在 `pkg/modelrouting/types.go` 增加：

```go
type InputMode string

const (
	InputModeText            InputMode = "text"
	InputModeFirstFrame      InputMode = "first_frame"
	InputModeFirstLastFrames InputMode = "first_last_frames"
	InputModeOmniReference   InputMode = "omni_reference"
)
```

`FactsInput` 和 `Facts` 增加 `InputMode`；`Constraints` 增加：

```go
InputModes        []InputMode    `json:"input_modes,omitempty"`
ReferenceMinimums ReferenceLimits `json:"reference_minimums,omitempty"`
```

`ResolveFacts` 在输入 mode 为空时根据零引用默认成 `text`，以兼容策略默认值校验。

- [ ] **Step 6: 实现匹配、校验、规范化和 overlap**

`Match` 在非空 `InputModes` 不包含 facts mode 时增加 `MismatchInputMode`；每类引用数小于 `ReferenceMinimums` 或大于 `ReferenceLimits` 时使用对应 mismatch reason。

`validateConstraints` 只接受四个枚举值，并要求每类满足 `0 <= minimum <= maximum <= Seedance 公共上限`。`normalizeRoutingPolicyWriteRequest` 去空白、去重、排序 input modes。`constraintsOverlap` 同时比较 input mode 集合和三类 `[minimum, maximum]` 闭区间；空 input modes 表示全集。

在 `validate.go` 增加可直接单测的领域 helper `representativeFactsInputs(constraints Constraints) []FactsInput`。输入模式为空时按四种模式处理；每种模式用 minimums 构造一个最小合法代表：`text` 必须允许零素材，`first_frame` 必须恰好允许 1 图且视频/音频为 0，`first_last_frames` 必须恰好允许 2 图且视频/音频为 0，`omni_reference` 至少有一种参考素材，纯音频 minimum 则补 1 张图片以满足 Ark 的“音频必须配图片或视频”规则。任何代表违反 minimum 或超过 maximum 就丢弃。

`validateConstraints` 在基础范围校验后要求该 helper 至少返回一个代表，否则返回 `ValidationInvalidReferenceLimit`，字段为 `targets.constraints.reference_minimums`。`ValidatePolicy` 的默认路由检查改为遍历所有 enabled target 的代表输入，把 policy 的 canonical model 和 defaults 交给 `ResolveFacts`，再调用 `Evaluate`；只要存在一个代表能到达任一渠道即通过，否则仍返回 `ValidationDefaultRouteUnavailable`。不要固定构造纯文生 facts，也不要把 minimums 临时清零。

核心实现形状固定为：

```go
func representativeFactsInputs(constraints Constraints) []FactsInput {
	modes := constraints.InputModes
	if len(modes) == 0 {
		modes = []InputMode{InputModeText, InputModeFirstFrame, InputModeFirstLastFrames, InputModeOmniReference}
	}
	inputs := make([]FactsInput, 0, len(modes))
	for _, mode := range modes {
		refs := constraints.ReferenceMinimums
		switch mode {
		case InputModeText:
			if refs.Images > 0 || refs.Videos > 0 || refs.Audios > 0 { continue }
			refs = ReferenceLimits{}
		case InputModeFirstFrame:
			if refs.Images > 1 || refs.Videos > 0 || refs.Audios > 0 { continue }
			refs.Images = 1
		case InputModeFirstLastFrames:
			if refs.Images > 2 || refs.Videos > 0 || refs.Audios > 0 { continue }
			refs.Images = 2
		case InputModeOmniReference:
			if refs.Images+refs.Videos+refs.Audios == 0 { refs.Images = 1 }
			if refs.Audios > 0 && refs.Images+refs.Videos == 0 { refs.Images = 1 }
		default:
			continue
		}
		if refs.Images > constraints.ReferenceLimits.Images || refs.Videos > constraints.ReferenceLimits.Videos || refs.Audios > constraints.ReferenceLimits.Audios { continue }
		inputs = append(inputs, FactsInput{InputMode: mode, ReferenceImages: refs.Images, ReferenceVideos: refs.Videos, ReferenceAudios: refs.Audios})
	}
	return inputs
}
```

- [ ] **Step 7: 从 Ark content 生成 input mode**

在 `parseSeedanceRoutingFields` 使用已解析的 `seedanceRoutingContentFacts`：

```go
switch {
case contentFacts.images == 0 && contentFacts.videos == 0 && contentFacts.audios == 0:
	input.InputMode = modelrouting.InputModeText
case contentFacts.lastFrames > 0:
	input.InputMode = modelrouting.InputModeFirstLastFrames
case contentFacts.firstFrames > 0:
	input.InputMode = modelrouting.InputModeFirstFrame
default:
	input.InputMode = modelrouting.InputModeOmniReference
}
```

签名 URL 仍只保存在 `ReferenceVideoURLs json:"-"`，input mode 不包含 URL 或角色原文。

- [ ] **Step 8: 扩展管理端路由编辑器**

前端 schema 增加四值 `input_modes` 和 `reference_minimums`，默认分别为四种全选和 `{images:0,videos:0,audios:0}`；API 读取旧策略缺失字段时应用同样默认。

`route-target-editor.tsx` 使用 ToggleGroup 编辑四种模式，并在现有三列 maximum 输入上方增加三列 minimum 输入。minimum 的 `max` 动态等于对应 maximum；maximum 的 `min` 动态等于 minimum。提交 schema 拒绝 minimum > maximum。

client test 操作模式按钮和图片 minimum 输入，断言 form 值与提交 payload 精确包含：

```json
{
  "input_modes":["first_frame","omni_reference"],
  "reference_minimums":{"images":1,"videos":0,"audios":0}
}
```

- [ ] **Step 9: 添加七语言文案并验证提交**

翻译 `Input modes`、`Text to video`、`First frame`、`First and last frames`、`Omni reference`、`Minimum reference images/videos/audios`。简体中文分别使用“输入模式”“文生视频”“首帧”“首尾帧”“全能参考”“最少参考图片/视频/音频”。

```powershell
gofmt -w pkg/modelrouting/types.go pkg/modelrouting/match.go pkg/modelrouting/match_test.go pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go middleware/model_routing.go middleware/model_routing_test.go service/routing_policy.go service/model_routing_test.go
go test ./pkg/modelrouting ./middleware ./service -run 'Test.*InputMode|Test.*ReferenceMinimum|Test.*Routing' -count=1
Set-Location web
bun test tests/model-routing-types.test.ts src/features/model-routing/components/route-target-editor-client.test.tsx
bun run i18n:sync
bun run typecheck
Set-Location ..
git add pkg/modelrouting/types.go pkg/modelrouting/match.go pkg/modelrouting/match_test.go pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go middleware/model_routing.go middleware/model_routing_test.go service/routing_policy.go service/model_routing_test.go web/src/features/model-routing/types.ts web/tests/model-routing-types.test.ts web/src/features/model-routing/components/route-target-editor.tsx web/src/features/model-routing/components/route-target-editor-client.test.tsx web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(routing): match Seedance input modes and minimum references"
```

Expected: PASS，旧策略 JSON 不需要迁移，读取后保持原有“任意输入模式、最小素材为 0”语义。

---

### Task 6: 证明三渠道 Ark 生命周期、路由和计费

**Files:**
- Create: `e2e/secure_upstream_e2e_test.go`
- Create: `e2e/secure_routing_e2e_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Create after real success: `docs/superpowers/reports/2026-07-26-secure-channel-acceptance.md`

- [ ] **Step 1: 写三分组 mock 生命周期**

表驱动创建三个普通 Secure 渠道，每条渠道只有一枚 Key 和一个 settings 枚举：

| 分组 | Key | Ark 场景 | 创建/查询路径 |
| --- | --- | --- | --- |
| discount | `discount-key` | pro、两张图片、8s、720p | `/api/generate-video` / `/api/task/{id}` |
| overseas | `overseas-key` | fast、图片+视频+音频 omni、8s、720p | `/api/generate-video` / `/api/task/{id}` |
| enterprise | `enterprise-key` | pro 文生、8s、720p | `/v1/videos` / `/v1/videos/{id}` |

每个 mock 精确断言 Authorization、Content-Type、请求体和私有任务 ID。创建响应只返回公开 ID；轮询依次返回 queued/in_progress/completed；Ark 单查和列表返回用户原始模型、成功状态和 `content.video_url`。

- [ ] **Step 2: 写三渠道能力路由测试**

同一策略配置三个 route target：

```text
discount:   modes=[first_frame,omni_reference], minimum images=1
overseas:   modes=[text,first_frame,first_last_frames,omni_reference], minimums=0
enterprise: modes=[text,first_frame,omni_reference], minimums=0, model=video-2.0-pro, resolution=720p
```

断言：纯文生永不使用 discount Key；严格首尾帧只进入 overseas；4k 图片请求只进入 discount/pro；video-only omni 永不进入 discount；720p 普通图片 omni 可由成本/利润规则在合法候选中选择。每次只允许一个 recording server 收到 POST，公开响应不包含选中的分组、渠道 ID 或上游模型。

- [ ] **Step 3: 写失败、重试和隐私测试**

三个分组分别返回业务失败，断言公共退款只执行一次。5xx 和网络错误允许现有重试机制切换到另一个能力匹配渠道，但不得切到 capability 不匹配的 Secure 分组。日志捕获器断言不包含三枚 Key、私有任务 ID或结果 URL 查询串；任务所有者的 `content.video_url` 保留播放所需的完整 URL。

- [ ] **Step 4: 扩展计费矩阵**

把 `ChannelTypeSecure` 的 discount/overseas/enterprise 三种 settings 加入共享 task billing fixture。按次计费在 submit accepted 后结算，按时长计费使用已校验的 duration，失败退款。上游无权威 usage 时不生成 token meter；所有 quota 转换继续走 `common.Quota*Checked` 和饱和审计。

- [ ] **Step 5: 运行并提交自动化测试**

```powershell
gofmt -w e2e/secure_upstream_e2e_test.go e2e/secure_routing_e2e_test.go relay/relay_task_billing_test.go
go test ./e2e -run 'TestSecure' -count=1 -v
go test ./relay -run 'TestSecure|TestTaskBilling' -count=1 -v
git add e2e/secure_upstream_e2e_test.go e2e/secure_routing_e2e_test.go relay/relay_task_billing_test.go
git commit -m "test(secure): cover grouped Ark video routing"
```

Expected: PASS。

- [ ] **Step 6: 运行全量验证和安全审计**

```powershell
go test ./relay/channel/task/newapivideo ./pkg/modelrouting ./middleware ./service ./constant ./relay ./controller ./e2e -count=1
go test ./... -count=1
go vet ./...
go build ./...
Set-Location web
bun test tests/channel-type-config.test.ts tests/model-routing-types.test.ts src/features/channels/lib/secure-video-group.test.ts src/features/model-routing/components/route-target-editor-client.test.tsx
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
Set-Location ..
rg -n 'ChannelTypeSecure|secure_video_group|SecureVideoGroup' constant dto controller relay service web/src e2e
rg -n 'discount-key|overseas-key|enterprise-key|private-task|signature=' e2e/secure_* relay/channel/task/newapivideo service
rg -n 'int\(.*quota|int\(math\.|OtherRatios\[' relay/channel/task/newapivideo relay/relay_task_billing_test.go
git diff --check
git status --short
```

Expected: 全部退出 0；测试 fixture 中的假凭据只出现在请求断言，不出现在公开响应或日志断言。

- [ ] **Step 7: 用三枚真实分组 Key 验收**

本机环境变量分别为 `SECURE_DISCOUNT_API_KEY`、`SECURE_OVERSEAS_API_KEY`、`SECURE_ENTERPRISE_API_KEY`，Base URL 为 `https://token.secure-skill.com`。创建三个临时 Secure 渠道，分别选择对应 group，至少完成：特价 pro 双图生成、海外 fast 图片+视频+音频全能参考、企业 pro 文生。

每条验证 Ark 创建、单查、列表、上游路径、媒体可播放、时长/分辨率、结算和公开 ID。额外验证把任一 Key 配到错误分组会在真实上游失败，但系统不会自动改分组或跨渠道复用该 Key。

- [ ] **Step 8: 写脱敏报告并提交**

报告记录三条公开任务 ID、状态序列、分组、上游模型、结果是否可播放和计费结果；不记录 Key、私有 ID、完整签名 URL 或用户素材 URL。

```powershell
git add docs/superpowers/reports/2026-07-26-secure-channel-acceptance.md
git diff --cached --quiet; if ($LASTEXITCODE -eq 0) { throw 'No Secure acceptance report to commit' }
git commit -m "docs(secure): record grouped channel acceptance"
```

---

## 自检

| 设计要求 | 实施任务 |
| --- | --- |
| 一个 Secure 类型、一套 adaptor | Task 3、Task 4 |
| 三枚 Key 建三个普通渠道 | Task 1、Task 2、Task 6 |
| `secure_video_group` 仅 Secure 可见且必填 | Task 1、Task 2 |
| 切换类型清理隐藏配置 | Task 1、Task 2 |
| 特价/海外 multipart，企业 JSON | Task 3 |
| 海外 `functionMode`、编号字段和 `@` 引用 | Task 3 |
| 海外参考视频总时长 15 秒上限 | Task 3 |
| 后台轮询恢复正确分组协议 | Task 4 |
| 路由不会选择能力不匹配的分组 Key | Task 5、Task 6 |
| Ark 创建、单查、列表零代码改动 | Task 4、Task 6 |
| 成功结算、失败退款和隐私隔离 | Task 6 |
| 三枚真实 Key 验收 | Task 6 |

本计划使用类型 66，并将最终 `ChannelTypeDummy` 调整为 67。严格执行 `Lucen -> MegaByAI -> 苍原 -> 派普 -> Secure`，避免常量和前端类型编号冲突。

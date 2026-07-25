# 利润感知渠道路由实施计划

> **面向实施代理：** 必须使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 技能，按任务逐项实施。使用复选框（`- [ ]`）跟踪进度。

**目标：** 在严格成本模式下，按官方用户售价、实际分组倍率、请求事实和活动渠道成本规则预测毛利率，只让达到最低预计毛利率的渠道模型参与选择，并在发送上游前权威复核。

**架构：** 请求进入网关后先形成不含敏感 URL 的路由事实；候选过滤批量取得渠道快照与活动成本规则，必要时通过独立元数据服务一次性补全输入视频时长，再复用现有用户计费与成本金额函数计算 `GrossMarginPPM`。筛选结果只缩小 `AllowedChannelIDs`，不改变优先级、权重和随机逻辑；适配器确认最终上游模型后，在 `PrepareCostAttempt` 和 `DoRequest` 前重新读取当前规则与阈值。

**技术栈：** Go 1.25.1、Gin、GORM v2、SQLite/MySQL/PostgreSQL、`shopspring/decimal`、现有 cost accounting、React 19、TypeScript、Zod、TanStack Query、Bun、i18next。

---

## 文件边界与依赖方向

- 修改 `setting/cost_setting/config.go`：原子保存模式与全局最低预计毛利率。
- 修改 `model/routing_policy.go`、`model/routing_policy_cache.go`、`pkg/modelrouting/types.go`：路由目标可空覆盖值进入数据库、DTO 和缓存快照。
- 新建 `pkg/seedancepricing/profile.go`：Seedance 分辨率像素、24 fps 和官方视频输入价格倍率的共享只读资料。
- 修改 `relay/channel/task/doubao/constants.go`：保留兼容包装，改用共享资料。
- 新建 `service/video_metadata_client.go`：独立元数据服务客户端和响应二次校验。
- 新建 `service/profit_routing.go`：Token 预测、收入/成本预测、阈值比较和候选结果。
- 修改 `service/cost_rule.go`：活动规则批量读取，避免 N+1。
- 修改 `model/ability.go`、`model/channel_cache.go`：列出筛选前候选 ID，不执行随机选择。
- 修改 `service/model_routing.go`、`service/channel_select.go`：普通、auto、亲和、指定渠道和重试共用利润准入。
- 修改 `relay/relay_task.go`、`controller/relay.go`：最终模型发送前复核与本地排除重试。
- 修改 cost accounting 和 model routing 前端模块及七种 locale；普通用户 API 不新增成本/利润字段。

实施前必须再次阅读 `pkg/billingexpr/expr.md`。执行前端任务时必须使用 `i18n-translate`、`shadcn-ui` 和 `vercel-react-best-practices` 技能。

### 任务 1：增加全局阈值、路由目标覆盖值和跨数据库迁移

**文件：**

- 修改： `setting/cost_setting/config.go`
- 修改： `setting/cost_setting/config_test.go`
- 修改： `dto/cost_accounting.go`
- 修改： `controller/cost_accounting.go`
- 修改： `controller/cost_accounting_test.go`
- 修改： `model/routing_policy.go`
- 修改： `model/routing_policy_test.go`
- 修改： `model/routing_policy_cache.go`
- 修改： `model/routing_policy_cache_test.go`
- 修改： `model/cost_accounting_migration_test.go`
- 修改： `pkg/modelrouting/types.go`
- 修改： `pkg/modelrouting/validate.go`
- 修改： `pkg/modelrouting/validate_test.go`
- 修改： `service/routing_policy.go`
- 修改： `service/routing_policy_test.go`

- [ ] **步骤 1：写全局设置和边界失败测试**

```go
func TestValidateMinimumExpectedMarginBPS(t *testing.T) {
	assert.NoError(t, ValidateMinimumExpectedMarginBPS(0))
	assert.NoError(t, ValidateMinimumExpectedMarginBPS(10_000))
	assert.Error(t, ValidateMinimumExpectedMarginBPS(-1))
	assert.Error(t, ValidateMinimumExpectedMarginBPS(10_001))
}

func TestRuntimePreservesExplicitZeroMargin(t *testing.T) {
	original := costSetting
	t.Cleanup(func() { costSetting = original; UpdateAndSync() })
	costSetting = CostSetting{Mode: types.CostAccountingStrict, MinimumExpectedMarginBPS: 0}
	UpdateAndSync()
	assert.Equal(t, 0, Runtime().MinimumExpectedMarginBPS)
}
```

Controller 测试发送 `{"mode":"strict","minimum_expected_margin_bps":0}`，断言 0 不会被 `binding:"required"` 丢弃，响应同时包含两个字段；10001 返回 400 且不更新任一设置。

- [ ] **步骤 2：写路由目标 NULL/0/边界和迁移失败测试**

新增测试断言：`nil` 继承、`&0` 保留显式零、10000 合法、-1/10001 被 `ValidationInvalidMinimumExpectedMargin` 拒绝；缓存刷新后指针值不丢失；SQLite `AutoMigrate` 有 `minimum_expected_margin_bps` 可空列。现有 MySQL/PostgreSQL migration harness 使用同一模型列表，不增加方言 SQL。

```go
zero := 0
	snapshot := PolicySnapshot{TargetsByChannel: map[int][]Target{1: {{
		ID: 1, ChannelID: 1, Enabled: true, MinimumExpectedMarginBPS: &zero,
		Constraints: validConstraints(),
	}}}}
	require.NoError(t, ValidatePolicy(snapshot, relaycommon.MaxTaskDurationSeconds))
	assert.NotNil(t, snapshot.TargetsByChannel[1][0].MinimumExpectedMarginBPS)
```

- [ ] **步骤 3：运行测试并确认 RED**

执行：`go test ./setting/cost_setting ./controller ./model ./pkg/modelrouting ./service -run 'MinimumExpectedMargin|CostAccountingSettings|RoutingPolicy|Migration' -count=1`

预期： 编译失败，提示最低毛利率字段、验证码和 DTO 尚未定义。

- [ ] **步骤 4：实现设置、字段、验证和缓存传播**

```go
const KeyMinimumExpectedMarginBPS = "minimum_expected_margin_bps"

type CostSetting struct {
	Mode                     types.CostAccountingMode `json:"mode"`
	MinimumExpectedMarginBPS int                      `json:"minimum_expected_margin_bps"`
}

type RuntimeSnapshot struct {
	Mode                     types.CostAccountingMode
	MinimumExpectedMarginBPS int
}

func ValidateMinimumExpectedMarginBPS(value int) error {
	if value < 0 || value > 10_000 {
		return fmt.Errorf("minimum expected margin must be between 0 and 10000 basis points")
	}
	return nil
}
```

更新请求使用指针保持 0：

```go
type UpdateCostAccountingSettingsRequest struct {
	Mode                     *types.CostAccountingMode `json:"mode"`
	MinimumExpectedMarginBPS *int                      `json:"minimum_expected_margin_bps"`
}
```

Handler 要求两个指针均非 nil，先完成严格覆盖检查和范围校验，再用 `model.UpdateOptionsBulk` 原子保存两个 option。响应固定为 `{"mode":...,"minimum_expected_margin_bps":...}`。

路由结构增加：

```go
type RouteTarget struct {
	// existing fields
	MinimumExpectedMarginBPS *int `json:"minimum_expected_margin_bps" gorm:"default:null"`
}

type Target struct {
	// existing fields
	MinimumExpectedMarginBPS *int `json:"minimum_expected_margin_bps,omitempty"`
}
```

同步修改 `RouteTargetWriteRequest`、`RouteTargetView`、`SaveRoutingPolicy`、`routingPolicyViewFromRow`、`routingPolicyWriteRequestFromView` 和两个快照构造点。不能添加 GORM boolean default 或数据库专用 DDL。

- [ ] **步骤 5：运行并提交后端配置基础**

执行：`gofmt -w setting/cost_setting dto/cost_accounting.go controller/cost_accounting.go controller/cost_accounting_test.go model/routing_policy.go model/routing_policy_test.go model/routing_policy_cache.go model/routing_policy_cache_test.go pkg/modelrouting service/routing_policy.go service/routing_policy_test.go`

执行：`go test ./setting/cost_setting ./controller ./model ./pkg/modelrouting ./service -run 'MinimumExpectedMargin|CostAccountingSettings|RoutingPolicy|Migration' -count=1`

预期： PASS，NULL 与显式 0 在数据库、服务视图和缓存快照中保持可区分。

```text
git add setting/cost_setting dto/cost_accounting.go controller/cost_accounting.go controller/cost_accounting_test.go model/routing_policy.go model/routing_policy_test.go model/routing_policy_cache.go model/routing_policy_cache_test.go model/cost_accounting_migration_test.go pkg/modelrouting service/routing_policy.go service/routing_policy_test.go
git commit -m "feat: configure minimum expected routing margin"
```

### 任务 2：增加管理后台阈值编辑和 i18n

**文件：**

- 修改： `web/src/features/cost-accounting/types.ts`
- 修改： `web/src/features/cost-accounting/api.ts`
- 修改： `web/src/features/cost-accounting/index.tsx`
- 修改： `web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`
- 修改： `web/src/features/model-routing/types.ts`
- 修改： `web/src/features/model-routing/components/route-target-editor.tsx`
- 修改： `web/src/features/model-routing/components/route-target-editor-client.test.tsx`
- 修改： `web/src/i18n/locales/en.json`
- 修改： `web/src/i18n/locales/zh.json`
- 修改： `web/src/i18n/locales/zh-TW.json`
- 修改： `web/src/i18n/locales/fr.json`
- 修改： `web/src/i18n/locales/ja.json`
- 修改： `web/src/i18n/locales/ru.json`
- 修改： `web/src/i18n/locales/vi.json`

- [ ] **步骤 1：写类型转换与交互失败测试**

```ts
test('routing target margin keeps null inheritance and explicit zero', () => {
  const inherited = createEmptyTarget()
  expect(inherited.minimum_expected_margin_bps).toBeNull()

  const request = toWriteRequest({
    ...createEmptyPolicyForm(),
    targets: [{ ...inherited, channel_id: 1, name: 'a', upstream_model: 'm', minimum_expected_margin_bps: 0 }],
  })
  expect(request.targets[0].minimum_expected_margin_bps).toBe(0)
})
```

组件测试断言全局输入 `10.25` 提交为 1025；目标输入留空提交 `null`；输入 `0` 提交 0；负数、超过 100、超过两位小数显示表单错误且不请求 API。

- [ ] **步骤 2：运行前端测试并确认 RED**

执行：`bun test web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx web/src/features/model-routing/components/route-target-editor-client.test.tsx`

预期： 失败，提示 settings/target schema 没有阈值字段。

- [ ] **步骤 3：实现 BPS 类型、百分比输入和 API 更新**

```ts
export interface CostAccountingSettings {
  mode: CostAccountingMode
  minimum_expected_margin_bps: number
}

export interface UpdateCostAccountingSettingsRequest {
  mode: CostAccountingMode
  minimum_expected_margin_bps: number
}

const marginBPSSchema = z.number().int().min(0).max(10_000)
```

`routeTargetFormSchema`、`routeTargetSchema` 和 write schema 增加 `minimum_expected_margin_bps: marginBPSSchema.nullable()`；`createEmptyTarget()` 默认 null；copy/from/to 函数原样保留 null/0。UI 显示百分比，转换规则固定为 `Math.round(percent * 100)`，输入 `step="0.01" min="0" max="100"`。全局设置 mutation 改为提交完整对象，成功后同时刷新 settings 和 coverage query。

- [ ] **步骤 4：添加七种语言的精确文案**

增加以下英文 key；各 locale 不允许保留英文占位：

| Key | zh | zh-TW | fr | ja | ru | vi |
|---|---|---|---|---|---|---|
| Minimum expected gross margin | 最低预计毛利率 | 最低預計毛利率 | Marge brute prévisionnelle minimale | 最低予想粗利率 | Минимальная ожидаемая валовая маржа | Biên lợi nhuận gộp dự kiến tối thiểu |
| Leave empty to inherit the global setting | 留空则继承全局设置 | 留空則繼承全域設定 | Laisser vide pour hériter du réglage global | 空欄の場合はグローバル設定を継承します | Оставьте пустым для наследования глобальной настройки | Để trống để kế thừa cài đặt toàn cục |
| Effective minimum margin: {{value}}% | 生效最低毛利率：{{value}}% | 生效最低毛利率：{{value}}% | Marge minimale effective : {{value}} % | 有効な最低粗利率：{{value}}% | Действующая минимальная маржа: {{value}}% | Biên lợi nhuận tối thiểu có hiệu lực: {{value}}% |
| Enter a percentage from 0 to 100 with at most two decimals | 请输入 0 到 100 之间且最多两位小数的百分比 | 請輸入 0 到 100 之間且最多兩位小數的百分比 | Saisissez un pourcentage de 0 à 100 avec deux décimales au maximum | 0～100 の範囲で小数点以下2桁まで入力してください | Введите процент от 0 до 100, не более двух знаков после запятой | Nhập tỷ lệ từ 0 đến 100 với tối đa hai chữ số thập phân |

- [ ] **步骤 5：运行并提交前端配置**

执行：`bun run --cwd web i18n:sync`

执行：`bun test web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx web/src/features/model-routing/components/route-target-editor-client.test.tsx`

执行：`bun run --cwd web typecheck`

预期： PASS；i18n sync 不产生未翻译 key 报告，最长文案在目标编辑器窄宽度下换行而不重叠。

```text
git add web/src/features/cost-accounting web/src/features/model-routing web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat: edit expected margin thresholds"
```

### 任务 3：共享 Seedance 定价资料并扩展路由事实

**文件：**

- 新建： `pkg/seedancepricing/profile.go`
- 新建： `pkg/seedancepricing/profile_test.go`
- 修改： `relay/channel/task/doubao/constants.go`
- 修改： `relay/channel/task/doubao/constants_test.go`
- 修改： `relay/channel/task/doubao/billing_acceptance_test.go`
- 修改： `pkg/modelrouting/types.go`
- 修改： `middleware/model_routing.go`
- 修改： `middleware/model_routing_test.go`

- [ ] **步骤 1：写共享价格资料和 URL 隐私失败测试**

测试 `480p/720p/1080p/4k` 的长短边、24 fps、Seedance 2.0/fast/mini 的官方不含视频和含视频倍率；未知模型或分辨率必须返回 `ok=false`。路由解析测试提交一个带签名查询参数的视频 URL，断言 `FactsInput.ReferenceVideoURLs` 保留给内部使用，`FactsInput`/`Facts`/`Audit` 的 `common.Marshal` JSON 都不包含 URL。

```go
func TestFactsDoNotMarshalReferenceVideoURLs(t *testing.T) {
	input := FactsInput{ReferenceVideoURLs: []string{"https://assets.example/a.mp4?sig=secret"}}
	body, err := common.Marshal(input)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "assets.example")
	assert.NotContains(t, string(body), "sig=secret")
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./pkg/seedancepricing ./relay/channel/task/doubao ./middleware ./pkg/modelrouting -run 'Seedance|Routing.*URL|Facts' -count=1`

预期： 编译失败，提示共享定价资料和 `ReferenceVideoURLs` 尚未定义；现有 Doubao 价格测试保持可定位。

- [ ] **步骤 3：实现共享资料和兼容包装**

`pkg/seedancepricing/profile.go` 只保存只读资料和规范化查询：

```go
type ResolutionProfile struct {
	Name         string
	Width        int
	Height       int
	FrameRateNum int64
	FrameRateDen int64
}

func Profile(resolution string) (ResolutionProfile, bool)
func VideoInputRatio(model, resolution string, hasVideo bool) (float64, bool)
```

资料固定为 480p `864x496`、720p `1280x720`、1080p `1920x1080`、4k `3840x2160` 和 24/1 fps。把 Doubao 现有价格表迁移到共享包，`GetVideoInputRatio`/`GetVideoBillingRatio` 保留原签名并委托共享包，避免官方计价与利润预测各自维护一套表。共享函数返回的倍率必须经过 `types.PriceData.AddOtherRatio` 的正数/有限值校验后才进入计费。

- [ ] **步骤 4：解析并验证输入视频 URL，但禁止进入公开事实**

将 `validateRoutingMedia` 改为返回规范化后的 URL；`extractSeedanceContentFacts` 收集最多 3 个 `video_url.url` 到内部切片，拒绝空 URL、非 HTTP(S)、非法主机和超过现有素材数量限制。`parseSeedanceRoutingFields` 写入 `FactsInput.ReferenceVideoURLs`，字段使用 `json:"-"`，`ResolveFacts` 只读取视频数量，不把 URL 复制到 `Facts` 或 `Audit`。测试确认日志、错误消息和审计快照中也没有完整 URL 或查询参数。

- [ ] **步骤 5：格式化、回归并提交共享资料**

执行：`gofmt -w pkg/seedancepricing relay/channel/task/doubao/constants.go middleware/model_routing.go pkg/modelrouting/types.go`

执行：`go test ./pkg/seedancepricing ./relay/channel/task/doubao ./middleware ./pkg/modelrouting -run 'Seedance|Routing.*URL|Facts' -count=1`

预期： PASS；Doubao 适配器与官方价格验收仍使用同一份分辨率/fps/倍率资料，任何 URL 都不会进入可序列化路由事实。

```text
git add pkg/seedancepricing relay/channel/task/doubao/constants.go relay/channel/task/doubao/constants_test.go relay/channel/task/doubao/billing_acceptance_test.go pkg/modelrouting/types.go middleware/model_routing.go middleware/model_routing_test.go
git commit -m "refactor: share seedance pricing profile"
```

### 任务 4：接入独立元数据服务并实现请求级懒加载

**文件：**

- 新建： `service/video_metadata_client.go`
- 新建： `service/video_metadata_client_test.go`
- 修改： `service/channel_select.go`
- 修改： `service/model_routing.go`
- 修改： `main.go`

- [ ] **步骤 1：写客户端错误分类、并发和单次调用失败测试**

使用 `httptest.Server` 覆盖：成功响应、缺失/错误服务令牌、401、5xx、超时、非法 JSON、越界字段、`duration_ms=0`、响应中夹带未知恶意字段。断言客户端二次校验后只返回合法 `videometa.Metadata`；invalid media 与 unavailable 分类不同，错误文本不含 URL、查询参数和令牌。一个请求带 3 个素材时并行上限为 3，同一 `RetryParam` 多次评估只调用客户端一次。

```go
func TestVideoMetadataLoaderUsesSyncOnce(t *testing.T) {
	var calls atomic.Int32
	client := fakeMetadataClient{Calls: &calls}
	state := NewProfitRoutingRequestState(client, []string{"https://assets.example/a.mp4"})
	_, firstErr := state.Metadata(context.Background())
	_, secondErr := state.Metadata(context.Background())
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, int32(1), calls.Load())
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./service -run 'VideoMetadata|MetadataLoader|ProfitRoutingRequestState' -count=1`

预期： 编译失败，提示客户端和请求级状态尚未定义。

- [ ] **步骤 3：实现内部 HTTP JSON 客户端和稳定错误类型**

定义接口，直接复用独立服务包的稳定 DTO：

```go
type VideoMetadataClient interface {
	Metadata(context.Context, string) (videometa.Metadata, error)
}

type VideoMetadataError struct {
	Kind  VideoMetadataErrorKind
	Status int
}
```

从 `VIDEO_METADATA_SERVICE_URL`、`VIDEO_METADATA_SERVICE_TOKEN`、`VIDEO_METADATA_TIMEOUT_SECONDS`、`VIDEO_METADATA_MAX_BYTES` 读取配置；令牌只放在请求头，不写入错误或日志。请求使用 `common.Marshal`，响应使用 `common.DecodeJson`，并再次调用 `Metadata.Validate()` 和 `relaycommon.MaxTaskDurationSeconds` 相关边界校验。401/5xx/网络错误/超时归为 unavailable；服务明确返回素材无效、格式不支持或超限归为 invalid media；响应字段非法归为 internal invalid response，全部失败关闭。

- [ ] **步骤 4：在候选需要时才解析，并在请求内共享结果**

增加 `ProfitRoutingRequestState`，内部持有 URL 切片、`sync.Once`、聚合时长和错误；URL 只存在请求内存，不放入 `Facts`、审计或日志。增加 `RetryParam` 对该状态的引用；存在输入视频，且官方用户售价或任一活动候选成本规则需要 Token 时，`service/channel_select.go` 才调用它。没有输入视频时输入 Token 为 0，不发元数据请求。使用共享 30 秒 context，最多并行处理 3 个 URL；成功后按毫秒 Decimal 求和，任何 0/负数/越界结果都拒绝。元数据服务不可用只排除依赖 Token 的候选；若用户收入本身依赖 Token，则该分组全部候选未知并排除，无其他候选时返回兼容渠道不可用 503。素材无效则在路由前返回 400，禁止降级为 0 秒或 0 Token。`main.go` 完成客户端装配；地址或令牌缺失时注册一个明确返回 unavailable 的客户端并记录不含秘密的启动告警，允许无需视频元数据的按次/时长/免费路径继续工作。

- [ ] **步骤 5：格式化并提交客户端集成**

执行：`gofmt -w service/video_metadata_client.go service/video_metadata_client_test.go service/channel_select.go service/model_routing.go main.go`

执行：`go test ./service -run 'VideoMetadata|MetadataLoader|ProfitRoutingRequestState' -count=1`

预期： PASS；服务不可用不会被误判成免费候选，3 个输入视频共享一次请求级解析结果，普通日志没有 URL/token。

```text
git add service/video_metadata_client.go service/video_metadata_client_test.go service/channel_select.go service/model_routing.go main.go
git commit -m "feat: load video metadata for profit routing"
```

### 任务 5：实现纯利润预测器并复用用户收入和 Token 结算

**文件：**

- 新建： `service/profit_routing.go`
- 新建： `service/profit_routing_test.go`
- 新建： `relay/helper/profit_preview.go`
- 修改： `service/task_billing.go`
- 修改： `service/task_billing_test.go`
- 修改： `relay/helper/cost_preview.go`
- 修改： `main.go`

- [ ] **步骤 1：写四种成本模式、Token 和毛利边界失败测试**

覆盖：免费成本、按次成本、按时长成本、total/completion/input_output 三种 Token 子模式；收入为零、规则缺失、计量缺失、Decimal 溢出、未知模型和未知输出分辨率必须返回不可准入结果。使用业务示例断言官方每秒 0.99 元、分组倍率 0.5、渠道 5 元/次时，0% 阈值下 5 秒排除、11 秒准入，10% 阈值下 11 秒排除、12 秒准入；毛利率等于阈值时准入。

```go
func TestEvaluateProfitEligibilityPerRequestThreshold(t *testing.T) {
	caseInput := ProfitRoutingInput{RevenueNanoUSD: yuan("2.475"), CostNanoUSD: yuan("5"), ThresholdBPS: 0}
	result := EvaluateProfitEligibility(caseInput)
	require.False(t, result.Eligible)
	assert.Equal(t, ProfitReasonMarginBelowThreshold, result.Reason)
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./service -run 'Profit|Margin|TokenEstimate|RecalculateTaskQuota' -count=1`

预期： 编译失败，提示预测器、理由枚举和纯 Token 结算函数尚未定义。

- [ ] **步骤 3：实现有界 Decimal Token 估算和统一金额比较**

定义请求事实和结果：

```go
type ProfitRoutingFacts struct {
	OutputDurationSeconds int
	InputDurationMS       int64
	Width                 int
	Height                int
	FrameRateNum          int64
	FrameRateDen          int64
	InputTokens           int64
	OutputTokens          int64
	TotalTokens           int64
}

type ProfitEligibilityResult struct {
	Eligible       bool
	Reason         ProfitExclusionReason
	RevenueNanoUSD int64
	CostNanoUSD    int64
	ProfitNanoUSD  int64
	MarginPPM      *int64
	ThresholdBPS   int
	RuleID         int64
	RuleVersion    int
}
```

`EstimateSeedanceTokens` 使用输入总时长和请求输出时长分别计算，宽度、高度、帧率均来自 `pkg/seedancepricing`；多段输入已在请求状态聚合。中间使用 `decimal.Decimal`，输入、输出及总 Token 在各自成为成本 meter 时向上取整；`total_tokens` 必须对精确的输入+输出 Decimal 求和后再 ceil，不能把两个已 ceil 的值相加。结果在 `relaycommon.MaxTokensLimit` 前拒绝或返回 `meter_unknown`。不允许裸 `int(float64(...))`；quota 转换只用 `common.QuotaFromDecimalChecked`/`common.QuotaRoundChecked` 并传播 clamp。

候选成本构造 `types.CostMeter`：按次为 1、按时长为已校验请求秒数、按 Token 按规则子模式填入预测 Token；`CalculateAttemptCost` 负责原币快照和 nano-USD 标准化。预计收入由注入的 `relay/helper.PreviewRoutingRevenue` 回调生成，回调只能复用 `ModelPriceHelperPerCall`、共享 Seedance 官方倍率和 `PreviewFinalUserQuota`，不得按渠道映射模型重算售价。`RevenueEquivalentNanoUSD`、`CheckedNanoSubtract`、`GrossMarginPPM` 复用现有实现；BPS 转 PPM 使用精确乘 100，`margin >= threshold` 才准入。

- [ ] **步骤 4：抽取实际 Token 结算的纯函数，避免预计与实际漂移**

从 `RecalculateTaskQuotaByTokens` 抽出可测试函数，例如：

```go
func CalculateTaskTokenQuota(totalTokens int64, modelRatio, groupRatio, otherMultiplier float64) (int64, *common.QuotaClamp, error)
```

函数负责非负/有限值校验、`common.QuotaFromFloatChecked` 饱和转换和错误返回；异步实际结算与利润预估共用该函数。`relay/helper/profit_preview.go` 仅负责组装 `RelayInfo`、使用当前用户分组和官方模型价格，不把供应商成本或 URL 放入收入结果。`main.go` 在启动时注入回调，禁止 `service` 反向导入 `relay/helper`。

- [ ] **步骤 5：运行计算回归并提交预测器**

执行：`gofmt -w service/profit_routing.go service/profit_routing_test.go service/task_billing.go service/task_billing_test.go relay/helper/profit_preview.go relay/helper/cost_preview.go main.go`

执行：`go test ./service ./relay/helper -run 'Profit|Margin|TokenEstimate|RecalculateTaskQuota' -count=1`

预期： PASS；四种成本模式、输入/输出 Token、0/10% 阈值边界和实际 Token 重算共享同一安全计算链路。

```text
git add service/profit_routing.go service/profit_routing_test.go service/task_billing.go service/task_billing_test.go relay/helper/profit_preview.go relay/helper/cost_preview.go main.go
git commit -m "feat: calculate profit-aware channel eligibility"
```

### 任务 6：批量加载活动规则并在候选阶段过滤

**文件：**

- 修改： `service/cost_rule.go`
- 修改： `service/cost_rule_test.go`
- 修改： `model/ability.go`
- 修改： `model/channel_cache.go`
- 修改： `model/channel_satisfy.go`
- 修改： `model/channel_routing_filter_test.go`
- 修改： `service/model_routing.go`
- 修改： `service/channel_select.go`
- 修改： `service/model_routing_test.go`

- [ ] **步骤 1：写批量查询和统一入口失败测试**

构造 3 个候选渠道，验证活动规则按 `channel_id + billable_upstream_model` 一次读取且无 N+1；未激活、过期、重复版本和缺少模型映射的规则均不可准入。测试普通随机、`auto` 分组、亲和锁定、指定渠道和重试入口都得到同一 `AllowedChannelIDs` 过滤结果；过滤后选择仍按优先级、权重和随机语义执行。

```go
func TestActiveCostRulesBatchDoesNotQueryPerCandidate(t *testing.T) {
	queryCounter := newQueryCounter()
	rules, err := ActiveCostRules([]CostRuleCandidate{{ChannelID: 1, BillableUpstreamModel: "a"}, {ChannelID: 2, BillableUpstreamModel: "b"}}, true)
	require.NoError(t, err)
	assert.Len(t, rules, 2)
	assert.Equal(t, 1, queryCounter.Count())
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./service ./model -run 'ActiveCostRules|Profit.*Route|AllowedChannel|Candidate' -count=1`

预期： 编译失败，提示批量规则 API 和筛选前候选列表函数尚未定义。

- [ ] **步骤 3：实现批量活动规则读取和候选 ID 列举**

新增 `ActiveCostRules(candidates, authoritative)`：使用一次 GORM 查询按候选键取得活动规则，内存缓存只作加速，权威发送前复核仍走数据库语义；规则激活、停用和版本变更继续调用现有覆盖缓存失效函数。跨 SQLite/MySQL/PostgreSQL 不使用方言 SQL，必要时在内存按复合键去重。

在 `model/ability.go`/`channel_cache.go` 增加列出能力、启用状态、请求路径和 `ChannelSelectFilter` 后的所有候选 ID 的函数，不在此处随机选择。能力路由目标使用 `Target.UpstreamModel`，legacy 渠道沿用 `ResolveMappedModel`，所有预测模型为空、映射循环或活动规则缺失都记录排除理由。

- [ ] **步骤 4：将利润过滤接入全部选路路径且不改变选择排序**

在 `selectChannelForGroup` 前先取得筛选前候选，按当前分组倍率和 `Target.MinimumExpectedMarginBPS`/全局阈值调用同一个 `FilterProfitEligibleChannels`，将通过的 ID 写入 `filter.AllowedChannelIDs`，再调用现有 `GetRandomSatisfiedChannel`。`strict` 关闭时保持原逻辑；严格模式下收入为 0、成本/计量未知、规则缺失、元数据不可用和计算溢出均排除。候选过滤不能按利润排序或改变 priority/weight。

`ValidateKnownChannelForRouting`、渠道亲和、指定渠道、`auto` 跨组和重试必须复用相同过滤函数；锁定渠道若不达标不得静默切换，交由控制器返回通用 503。`costCoverageMisses` 与利润排除诊断分开维护，不能把未知成本当作覆盖成功。

- [ ] **步骤 5：运行路由回归并提交候选过滤**

执行：`gofmt -w service/cost_rule.go model/ability.go model/channel_cache.go model/channel_satisfy.go service/model_routing.go service/channel_select.go`

执行：`go test ./service ./model -run 'ActiveCostRules|Profit.*Route|AllowedChannel|Candidate|Affinity|Auto' -count=1`

预期： PASS；普通、auto、亲和、指定和重试都无法绕过利润准入，未被过滤的候选仍保持原选择顺序语义。

```text
git add service/cost_rule.go service/cost_rule_test.go model/ability.go model/channel_cache.go model/channel_satisfy.go model/channel_routing_filter_test.go service/model_routing.go service/channel_select.go service/model_routing_test.go
git commit -m "feat: filter channels by expected margin"
```

### 任务 7：最终模型确认后执行发送前权威复核

**文件：**

- 修改： `relay/relay_task.go`
- 修改： `relay/cost_accounting_adaptor.go`
- 修改： `relay/relay_task_billing_test.go`
- 修改： `controller/relay.go`
- 修改： `controller/cost_task_relay_test.go`
- 修改： `service/profit_routing.go`

- [ ] **步骤 1：写配置变化和重试失败测试**

先让候选阶段通过，再在模型映射确认后替换活动成本规则或提高全局/目标阈值；断言 `PrepareCostAttempt`、`AuthorizeCostDispatch` 和适配器 `DoRequest` 均未调用，渠道被加入排除集合后继续尝试下一个候选。测试已预扣额度在最终失败时走现有退款路径；锁定亲和渠道无法切换时返回统一 503。

```go
func TestProfitRecheckBlocksDispatchAfterRuleChange(t *testing.T) {
	// candidate stage passes; authoritative rule is changed before dispatch.
	result := RecheckSelectedChannelProfit(ctx, info)
	require.Error(t, result)
	assert.ErrorIs(t, result, ErrProfitEligibility)
	assert.Zero(t, fakeUpstream.Calls)
}
```

- [ ] **步骤 2：运行测试并确认 RED**

执行：`go test ./relay ./controller ./service -run 'ProfitRecheck|Dispatch|CostTaskRelay|Retry' -count=1`

预期： 编译失败，提示发送前权威复核和可重试错误尚未定义。

- [ ] **步骤 3：在成本 attempt 前复核最终 billable 模型**

在 `relay/relay_task.go` 中保留现有顺序：适配器完成 `ConfirmTaskCostIdentity` 后取得最终 `BillableUpstreamModel`，立即调用 `service.RecheckSelectedChannelProfit`，然后才允许 `PrepareCostAttempt` 和 `AuthorizeCostDispatch`，最后才调用 `DoRequest`。复核必须读取当前活动规则和最新全局/目标阈值，不复用候选阶段的规则缓存；按最终模型重新计算同一请求事实、官方用户收入、渠道成本和毛利率。

新增 `ProfitEligibilityError{ChannelID, Reason}`，金额、阈值、规则 ID/版本只写请求级管理员诊断。复核失败不得创建 attempt、不得标记 dispatching、不得调用上游；候选失败作为本地可重试排除，沿用 `CostCoverageError` 的控制器处理模式。普通错误只返回通用 `compatible channel is unavailable`/`available channel is unavailable`；锁定渠道或指定渠道无法切换时直接 503。已有预扣通过 `RelayTask` 的 defer/refund 路径回收。

- [ ] **步骤 4：覆盖同步任务、异步任务和所有重试入口**

同步响应、视频任务提交、auto 跨组、渠道亲和、指定渠道和上游失败重试都必须在最终模型确认后经过同一复核函数。确认 `BillableUpstreamModel` 为空、模型映射发生变化、规则版本变化或请求事实缺失时均失败关闭；不得因为 `specific_channel_id`、`LockedChannel` 或 retry 次数绕过阈值。

- [ ] **步骤 5：运行发送前回归并提交复核逻辑**

执行：`gofmt -w relay/relay_task.go relay/cost_accounting_adaptor.go controller/relay.go service/profit_routing.go`

执行：`go test ./relay ./controller ./service -run 'ProfitRecheck|Dispatch|CostTaskRelay|Retry' -count=1`

预期： PASS；配置变化不会产生上游副作用，失败候选可重试，普通用户不会获得金额或内部原因。

```text
git add relay/relay_task.go relay/cost_accounting_adaptor.go relay/relay_task_billing_test.go controller/relay.go controller/cost_task_relay_test.go service/profit_routing.go
git commit -m "feat: recheck profit before upstream dispatch"
```

### 任务 8：管理员诊断、隐私、端到端和跨数据库验收

**文件：**

- 修改： `service/model_routing.go`
- 修改： `service/log_info_generate.go`
- 修改： `service/channel_select.go`
- 修改： `controller/relay.go`
- 修改： `service/profit_routing_test.go`
- 修改： `service/model_routing_test.go`
- 修改： `controller/cost_task_relay_test.go`
- 修改： `model/cost_accounting_migration_test.go`
- 修改： `web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`

- [ ] **步骤 1：固化原因枚举和管理员专用诊断**

原因只允许以下稳定值：`revenue_unknown`、`cost_rule_missing`、`meter_unknown`、`margin_below_threshold`、`calculation_error`、`metadata_unavailable`。扩展请求级诊断存储，包含 channel ID、billable model、预计收入/成本/利润、毛利率、阈值、规则 ID/版本和原因；调用 `AppendRoutingAdminInfoFromContext` 时放入 `other.admin_info.routing_diagnostics`，普通日志视图继续整体剥离 `admin_info`。URL、令牌、完整素材内容和查询参数一律禁止写入诊断、Facts、Audit、API 错误或日志。

- [ ] **步骤 2：增加端到端场景和隐私回归**

用 fake metadata client、fake upstream 和可切换活动规则覆盖：0% 阈值下 11 秒按次渠道准入；10% 阈值下 11 秒拒绝、12 秒准入；按时长、免费和 Token 成本；输入视频多段求和；普通、auto、亲和、指定渠道和重试。断言所有候选排除时是 503，素材无效是 400，服务不可用只排除依赖 Token 的候选。捕获普通响应、管理员错误日志和任务消费日志，断言普通用户看不到渠道 ID、金额、阈值、规则版本和排除原因，管理员可看到结构化诊断但看不到 URL/token。

- [ ] **步骤 3：验证数据库、缓存和表达式计费兼容性**

在 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ migration harness 中验证 `route_targets.minimum_expected_margin_bps` 可空列、显式 0、策略缓存刷新和批量规则查询；激活/停用/版本变更立即失效相关缓存。复核 `pkg/billingexpr/expr.md` 的 token 规范化、版本和配额转换契约，确保预测路径不复制或改变表达式语义。

- [ ] **步骤 4：扫描危险数据流并运行全量验证**

执行：`rg -n 'ReferenceVideoURLs|RawQuery|RequestURI|Authorization|URL\.String\(\)' service middleware pkg/modelrouting relay controller`

预期： URL 只出现在解析、内部请求和测试构造处，不出现在日志拼接、Facts/Audit JSON 或普通响应。

执行：`go test ./... -count=1`

执行：`bun test --cwd web web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`

执行：`bun run --cwd web typecheck`

执行：`bun run --cwd web lint`

执行：`bun run --cwd web build`

预期： 后端、前端测试和构建通过；已有无关失败需记录包名和测试名，不修改无关工作树文件。

- [ ] **步骤 5：提交最终验收补充**

```text
git add service/model_routing.go service/log_info_generate.go service/channel_select.go controller/relay.go service/profit_routing_test.go service/model_routing_test.go controller/cost_task_relay_test.go model/cost_accounting_migration_test.go web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx
git commit -m "test: verify profit-aware routing privacy and compatibility"
```

## 实施顺序与完成定义

1. 先完成任务 1 的数据库/运行时设置，再完成任务 2 的管理后台阈值编辑。
2. 部署并健康检查独立元数据服务后执行任务 3-4；严格成本模式默认阈值为 0，缺失元数据仍不得按零成本降级。
3. 完成任务 5 的纯预测器和现有结算复用，再执行任务 6 候选过滤与任务 7 发送前复核。
4. 任务 8 的全量测试、隐私扫描和三数据库验证通过后，才允许把非零阈值推广到生产路由目标。

完成定义：按官方售价和用户分组倍率计算用户收入，按次/时长/Token/免费四种渠道成本实时预测并按最低预计毛利率准入；候选过滤保持既有优先级、权重和随机语义；输入视频由独立服务解析；最终发送前再次权威复核；普通用户不见成本利润内部信息；管理员可通过诊断和既有成本报告查看预计/实际金额；SQLite、MySQL、PostgreSQL 和前端检查均通过。

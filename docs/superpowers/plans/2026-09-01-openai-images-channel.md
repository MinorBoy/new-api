# OpenAI Images Unified Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让真实兼容 OpenAI Images 的 `gpt-image`、`gemini-*-image` 上游可以只通过全局图像目录和渠道页完成接入、定价、成本配置、兼容性验证与最低成本路由，停止依赖 `sd收录` 重复维护图像协议能力。

**Architecture:** 新增纯 Go `pkg/imageprofile` 协议合同和 `setting/image_setting` 版本化全局配置；请求进入现有 OpenAI Images relay 后先解析公共 SKU，再以渠道 `image_profile`、现有 `Models`/`ModelMapping` 和 active 成本规则建立候选。`manual` 保持现有 Priority/Weight，`lowest_cost` 以本次预计供应商成本为第一排序键，并把售价、成本规则、兼容性合同和实际出图数量冻结到请求审计快照。

**Tech Stack:** Go 1.22、Gin、GORM v2、Testify、shopspring/decimal、React 19、TypeScript、Zod、TanStack Query、Base UI、Tailwind CSS、Bun、httptest；SQLite/MySQL/PostgreSQL 均须兼容。

---

## 实施约束

- 首期只接入 `/v1/images/generations` 和 `/v1/images/edits`；不把原生 Gemini `generateContent` 生图转换为 OpenAI Images。
- 绑定 `openai_images` 的渠道必须实际使用 OpenAI-compatible adaptor；`gemini-*-image` 只是公共或上游模型名，不改变协议。
- `refreshing-sd-channel-config` 和现有 Seedance 导入/发布流程不做任何修改。
- 计费代码开始前完整阅读 `pkg/billingexpr/expr.md`；图像数量复用 `dto.MaxImageN`，金额转 quota 使用 `common.QuotaFromDecimalChecked`，倍率只经 `PriceData.AddOtherRatio`。
- 所有后端 JSON 编解码使用 `common.Marshal`、`common.Unmarshal` 或 `common.UnmarshalJsonStr`；不得新增直接调用 `encoding/json` 的业务代码。
- 每个任务先提交失败测试，再做最小实现；新 Go 测试使用 `require`/`assert`，前端使用确定性 Bun 测试。

## Task 1：内置 `openai_images` 档案与渠道绑定合同

**Files:**

- Create: `pkg/imageprofile/profile.go`
- Create: `pkg/imageprofile/profile_test.go`
- Modify: `relaykit/dto/channel_settings.go`
- Modify: `relaykit/dto/channel_settings_test.go`
- Modify: `model/channel.go`
- Modify: `model/channel_settings_test.go`

- [ ] 在 `pkg/imageprofile/profile_test.go` 写表驱动测试，覆盖：档案名/版本、默认 generations/edits 路径、相对路径与完整 HTTPS URL、拒绝 query/userinfo/fragment、重复能力值、非法 `max_n`、兼容性状态枚举。先运行：

  ```powershell
  go test ./pkg/imageprofile -run 'Test(OpenAIImagesProfile|BindingValidate)' -count=1
  ```

  预期：因 `Binding`、`Lookup` 尚未定义而编译失败。

- [ ] 在 `pkg/imageprofile/profile.go` 实现稳定合同，结构和入口固定为：

  ```go
  type Endpoint string

  const (
      EndpointGenerations Endpoint = "generations"
      EndpointEdits       Endpoint = "edits"
      OpenAIImagesProfile          = "openai_images"
      OpenAIImagesVersion          = 1
  )

  type Capability struct {
      Enabled         bool     `json:"enabled"`
      Sizes           []string `json:"sizes,omitempty"`
      Qualities       []string `json:"qualities,omitempty"`
      ResponseFormats []string `json:"response_formats,omitempty"`
      MaxN            uint     `json:"max_n"`
      MaxInputImages  uint     `json:"max_input_images"`
      SupportsMask    bool     `json:"supports_mask"`
  }

  type Binding struct {
      Profile             string                       `json:"profile"`
      ProfileVersion      int                          `json:"profile_version"`
      Paths               map[Endpoint]string          `json:"paths,omitempty"`
      CapabilityOverrides map[string]ModelCapabilities `json:"capability_overrides,omitempty"`
      Compatibility       map[string]Compatibility     `json:"compatibility,omitempty"`
  }

  func Lookup(name string, version int) (Profile, bool)
  func (b Binding) Validate() error
  func (b Binding) Path(endpoint Endpoint) string
  ```

  `Lookup` 只注册版本 1；默认路径分别是 `/v1/images/generations`、`/v1/images/edits`。完整 URL 只允许 `http/https` 且不得含 userinfo、query、fragment；相对路径必须以单个 `/` 开始。

- [ ] 给 `relaykit/dto.ChannelOtherSettings` 增加 `ImageProfile *imageprofile.Binding \`json:"image_profile,omitempty"\``，并让 `model.Channel.ValidateSettings()` 调用 `Binding.Validate()`。绑定存在时只允许能解析为 OpenAI adaptor 的渠道类型；原生 Gemini/Vertex 渠道返回明确配置错误，管理员需新建 OpenAI-compatible 渠道。

- [ ] 增加模型校验测试：旧 `settings` 无此字段时行为不变；OpenAI 类型通过；Gemini 类型拒绝；错误档案/路径拒绝；JSON round-trip 保留完整 `image_profile`。

- [ ] 运行并确认通过：

  ```powershell
  gofmt -w pkg/imageprofile/profile.go pkg/imageprofile/profile_test.go relaykit/dto/channel_settings.go relaykit/dto/channel_settings_test.go model/channel.go model/channel_settings_test.go
  go test ./pkg/imageprofile ./relaykit/dto ./model -run 'Test.*(ImageProfile|OpenAIImages|ChannelSettings)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add pkg/imageprofile relaykit/dto/channel_settings.go relaykit/dto/channel_settings_test.go model/channel.go model/channel_settings_test.go
  git commit -m "feat: add OpenAI Images channel profile"
  ```

## Task 2：全局图像模型目录和路由策略系统选项

**Files:**

- Create: `setting/image_setting/catalog.go`
- Create: `setting/image_setting/catalog_test.go`
- Create: `setting/image_setting/routing.go`
- Create: `setting/image_setting/routing_test.go`
- Modify: `model/option.go`
- Modify: `model/option_test.go`

- [ ] 在 `setting/image_setting` 先写失败测试，覆盖版本、模型/端点/SKU 唯一性、默认 size/quality/response format、`max_n <= dto.MaxImageN`、输入图/mask 一致性、十进制定点售价、路由策略和毛利范围。运行：

  ```powershell
  go test ./setting/image_setting -count=1
  ```

  预期：新包和导出类型尚不存在而失败。

- [ ] 实现版本化目录，不接受浮点价格：

  ```go
  const (
      CatalogOptionKey = "ImageModelCatalog"
      CatalogVersion   = 1
  )

  type Catalog struct {
      Version int                   `json:"version"`
      Models  map[string]ModelEntry `json:"models"`
  }

  type EndpointCatalog struct {
      Capability            imageprofile.Capability `json:"capability"`
      DefaultSize           string                  `json:"default_size"`
      DefaultQuality        string                  `json:"default_quality"`
      DefaultResponseFormat string                  `json:"default_response_format"`
  }

  type SKU struct {
      Endpoint     imageprofile.Endpoint `json:"endpoint"`
      Size         string                `json:"size"`
      Quality      string                `json:"quality"`
      Unit         string                `json:"unit"`
      SalePriceUSD string                `json:"sale_price_usd"`
  }

  type Selection struct {
      Model, Size, Quality, ResponseFormat string
      Endpoint                            imageprofile.Endpoint
      N, InputImages                      uint
      HasMask                             bool
  }

  type ResolvedSKU struct {
      CatalogVersion int
      Model, SKUKey, Size, Quality, ResponseFormat, SalePriceUSD string
      Endpoint imageprofile.Endpoint
      N, InputImages uint
      HasMask bool
  }

  func UpdateCatalogByJSONString(raw string) error
  func Catalog2JSONString() string
  func Snapshot() Catalog
  func Resolve(selection Selection) (ResolvedSKU, error)
  ```

  `Resolve` 缺模型、端点或精确 SKU 时返回可映射为 400 的类型化错误；空 size/quality/response format 使用端点默认值，并生成 `gen-{size}-{quality}` 或 `edit-{size}-{quality}` 的稳定 SKU key。

- [ ] 实现路由设置：

  ```go
  const RoutingOptionKey = "ImageRoutingPolicy"

  type Strategy string
  const (
      StrategyManual     Strategy = "manual"
      StrategyLowestCost Strategy = "lowest_cost"
  )

  type Policy struct {
      Strategy                 Strategy `json:"strategy"`
      MinimumExpectedMarginBPS *int     `json:"minimum_expected_margin_bps,omitempty"`
      RequireCompatibilityTest bool     `json:"require_compatibility_test"`
  }

  type RoutingConfig struct {
      Version int                          `json:"version"`
      Default Policy                       `json:"default"`
      Groups  map[string]map[string]Policy `json:"groups,omitempty"`
  }

  func UpdateRoutingByJSONString(raw string) error
  func Routing2JSONString() string
  func PolicyFor(group, model string) Policy
  ```

  默认策略固定为 `manual`；毛利基点限定在 `0..10000`。

- [ ] 在 `model.InitOptionMap()` 注册两个默认 JSON，在 `updateOptionMap` 的明确分支调用对应更新函数；失败更新不得替换内存快照。使用现有 Option/GORM 路径，不新增数据库表或方言 SQL。

- [ ] 运行：

  ```powershell
  gofmt -w setting/image_setting model/option.go model/option_test.go
  go test ./setting/image_setting ./model -run 'Test.*(ImageCatalog|ImageRouting|Option)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add setting/image_setting model/option.go model/option_test.go
  git commit -m "feat: add global image model catalog"
  ```

## Task 3：请求归一化、SKU 解析和渠道能力过滤

**Files:**

- Modify: `relaykit/dto/openai_image.go`
- Modify: `relay/helper/valid_request.go`
- Modify: `relay/helper/openai_image_request_test.go`
- Create: `service/image_profile.go`
- Create: `service/image_profile_test.go`
- Modify: `service/channel_select.go`
- Modify: `service/model_routing.go`
- Modify: `relay/channel/openai/adaptor.go`
- Create: `relay/channel/openai/image_profile_test.go`
- Modify: `controller/relay.go`

- [ ] 为 JSON generations、JSON edits、multipart edits 写失败测试：显式 `n=0` 拒绝，`n > MaxImageN` 拒绝，多图数量和 mask 被记录，空 size/quality 使用目录默认值，不支持组合返回 400。现有显式零值和超大无符号数回归必须保留。

- [ ] 给 `dto.ImageRequest` 添加仅供内部校验的 `InputImageCount uint \`json:"-"\`` 和 `HasMask bool \`json:"-"\``。在 `GetAndValidOpenAIImageRequest` 统一要求 `n` 为 `1..dto.MaxImageN`；multipart 从 `image`、`image[]`、`image[index]` 文件项计数，并在进入计费前执行同一上限。

- [ ] 在 `service/image_profile.go` 实现以下入口：

  ```go
  type ImageRequestContext struct {
      Resolved image_setting.ResolvedSKU
  }

  type ImageChannelEligibility struct {
      ChannelID, Priority, Weight int
      UpstreamModel, CostVariantKey, ContractHash string
  }

  func ResolveImageRequest(request *dto.ImageRequest, relayMode int) (ImageRequestContext, error)
  func EvaluateImageChannel(channel *model.Channel, publicModel string, request ImageRequestContext) (ImageChannelEligibility, error)
  func ResolveImageContractHash(channel *model.Channel, publicModel string, request ImageRequestContext) (string, error)
  ```

  `EvaluateImageChannel` 必须依次检查：OpenAI adaptor、`image_profile`、`Models`、`ModelMapping`、端点启用、渠道 override 是全局能力子集、当前请求 size/quality/response format/n/输入图/mask。没有 `image_profile` 的旧渠道继续走旧图像路径，但不得进入新的 `lowest_cost` 池。

- [ ] 给 `RetryParam` 增加 `ImageRequest *ImageRequestContext`。`controller.Relay` 在用户预扣之前调用 `ResolveImageRequest` 并写入该字段；只有目录中启用 `openai_images` 的公共模型走新逻辑，其他图像模型保持现状。

- [ ] 在 `selectChannelForGroup` 的现有 filter 上相交图像合格渠道 ID；拒绝原因只写管理员诊断。原生 Gemini relay 请求没有 `ImageRequestContext`，因此不能进入该候选池。

- [ ] 在 OpenAI adaptor 的 `GetRequestURL` 中，当 relay mode 是 generations/edits 且存在 `image_profile` 时，使用 `Binding.Path(endpoint)` 与现有 `relaycommon.GetFullRequestURL` 拼接；没有绑定时保持 `info.RequestURLPath` 行为。补充相对路径和完整 URL 测试。

- [ ] 运行：

  ```powershell
  gofmt -w relaykit/dto/openai_image.go relay/helper/valid_request.go relay/helper/openai_image_request_test.go service/image_profile.go service/image_profile_test.go service/channel_select.go service/model_routing.go relay/channel/openai/adaptor.go relay/channel/openai/image_profile_test.go controller/relay.go
  go test ./relay/helper ./service ./relay/channel/openai ./controller -run 'Test.*(OpenAIImage|ImageProfile|ImageRequest|ImageChannel)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add relaykit/dto/openai_image.go relay/helper/valid_request.go relay/helper/openai_image_request_test.go service/image_profile.go service/image_profile_test.go service/channel_select.go service/model_routing.go relay/channel/openai/adaptor.go relay/channel/openai/image_profile_test.go controller/relay.go
  git commit -m "feat: resolve image SKU capabilities"
  ```

## Task 4：`per_image` 供应商成本模式和实际出图计量

**Files:**

- Modify: `types/cost_accounting.go`
- Modify: `types/cost_accounting_test.go`
- Modify: `service/cost_amount.go`
- Modify: `service/cost_amount_test.go`
- Modify: `service/cost_rule.go`
- Modify: `service/cost_rule_test.go`
- Modify: `service/cost_accounting.go`
- Modify: `service/cost_accounting_test.go`
- Modify: `relay/cost_accounting_adaptor.go`
- Modify: `relay/cost_accounting_adaptor_test.go`

- [ ] 先添加失败测试，断言 `CostModePerImage == "per_image"`、缺失 `ImageCount` 拒绝、0/负数/超过 `dto.MaxImageN` 拒绝、`UnitPrice * ImageCount` 精确计算、per-request 不乘数量、响应实际数量优先于请求数量。

- [ ] 按以下类型扩展成本合同：

  ```go
  const CostModePerImage CostMode = "per_image"

  type CostMeter struct {
      Source           CostMeterSource `json:"source"`
      ImageCount       *int64          `json:"image_count,omitempty"`
      DurationSeconds  *string         `json:"duration_seconds,omitempty"`
      InputTokens      *int64          `json:"input_tokens,omitempty"`
      OutputTokens     *int64          `json:"output_tokens,omitempty"`
      CompletionTokens *int64          `json:"completion_tokens,omitempty"`
      TotalTokens      *int64          `json:"total_tokens,omitempty"`
  }
  ```

  `per_image` 使用 `CostRuleConfigV1.UnitPrice`，允许 `validated_request` 和 `upstream_actual` meter；charge event 只允许 `response_succeeded`。`validateCostMeterBounds` 对图像数量应用 `1..dto.MaxImageN`。

- [ ] 在 `NormalizeCostRuleConfig`、`ValidateCostRuleDraft`、`CalculateAttemptCost`、成本目录/导入允许模式中加入 `per_image`。使用 decimal 乘法，最终 nano-USD 仍走现有 checked/saturation 路径。

- [ ] 为 OpenAI Images 增加专用成本合同，保持聊天 token 合同不变：

  ```go
  func openAIImagesCostContract() channel.CostAccountingAdaptor
  func (c *openAIImagesCostContract) NormalizeCostMeter(info *relaycommon.RelayInfo, usage any) (types.CostMeter, error)
  ```

  `CostCapabilitiesForRoute` 仅对 `/v1/images/generations` 和 `/v1/images/edits` 暴露 `per_image` 所需 meter；响应 data 数量可靠时标为 `upstream_actual`，否则由结算回退到已验证请求数量并写 meter source。

- [ ] 让 `costAccountingAdaptor.DoResponse` 将 `per_image` 纳入结算分支；非法或缺失 meter 进入现有 `settlement_failed`，不得用 0 成本完成。

- [ ] 运行：

  ```powershell
  gofmt -w types/cost_accounting.go types/cost_accounting_test.go service/cost_amount.go service/cost_amount_test.go service/cost_rule.go service/cost_rule_test.go service/cost_accounting.go service/cost_accounting_test.go relay/cost_accounting_adaptor.go relay/cost_accounting_adaptor_test.go
  go test ./types ./service ./relay -run 'Test.*(PerImage|CostMeter|AttemptCost|CostRule)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add types/cost_accounting.go types/cost_accounting_test.go service/cost_amount.go service/cost_amount_test.go service/cost_rule.go service/cost_rule_test.go service/cost_accounting.go service/cost_accounting_test.go relay/cost_accounting_adaptor.go relay/cost_accounting_adaptor_test.go
  git commit -m "feat: account supplier cost per image"
  ```

## Task 5：图像用户售价快照、预扣和结算安全

**Files:**

- Create: `types/image_billing.go`
- Create: `types/image_billing_test.go`
- Modify: `relay/common/relay_info.go`
- Modify: `relay/helper/price.go`
- Modify: `relay/helper/price_test.go`
- Modify: `relay/channel/openai/relay_image.go`
- Modify: `relay/channel/openai/image_stream_test.go`
- Modify: `service/quota.go`
- Modify: `service/quota_test.go`
- Modify: `service/log_info_generate.go`

- [ ] 在动计费代码前阅读 `pkg/billingexpr/expr.md`，并在实现记录中写明本任务使用的 quota 转换和饱和审计入口。

- [ ] 先写失败测试：目录单价 × 请求 `n` × 分组倍率；管理员中途改目录不改变结算；响应实际图数改变最终扣费；缺失实际数量回退请求数；0/超大/NaN/Infinity 不能形成负 quota；clamp 写入 `admin_info.quota_saturation`。

- [ ] 新增不可变快照：

  ```go
  type ImageBillingSnapshot struct {
      CatalogVersion   int    `json:"catalog_version"`
      Model            string `json:"model"`
      Endpoint         string `json:"endpoint"`
      SKUKey           string `json:"sku"`
      UnitSalePriceUSD string `json:"unit_sale_price_usd"`
      RequestedImages  int64  `json:"requested_images"`
      SettledImages    *int64 `json:"settled_images,omitempty"`
      MeterSource      string `json:"meter_source,omitempty"`
      GroupRatio       string `json:"group_ratio"`
      QuotaPerUnit     string `json:"quota_per_unit"`
  }

  func (s ImageBillingSnapshot) Quota(imageCount int64) (int, *common.QuotaClamp, error)
  ```

  `Quota` 用 `decimal.NewFromString` 和 `common.QuotaFromDecimalChecked`；数量必须为 `1..dto.MaxImageN`。

- [ ] 给 `RelayInfo` 增加 `ImageBillingSnapshot *types.ImageBillingSnapshot`。实现：

  ```go
  func ImageSKUPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, sku image_setting.ResolvedSKU) (types.PriceData, error)
  func RecordImageSettlementCount(info *relaycommon.RelayInfo, count int64, source types.CostMeterSource) error
  ```

  预扣只读目录一次并保存版本、单价、n、分组倍率和 `QuotaPerUnit`；同时用 `PriceData.AddOtherRatio("n", ...)` 保持现有日志/价格行为，但最终图像 quota 以快照为准。

- [ ] 让 OpenAI 非流式、SSE 和 JSON-as-stream 三条响应路径统一调用 `RecordImageSettlementCount`。只接受 `1..MaxImageN` 的权威数量；客户端断开或响应无可靠数量时保留 requested count，并标记 `validated_request`。

- [ ] 在最终用户 quota 计算中优先使用 `ImageBillingSnapshot.Quota`，将返回的 clamp 合并到 `relayInfo.QuotaClamp`；`RecognizeBilledRevenue` 继续读取最终 quota，不读取实时目录。

- [ ] 运行：

  ```powershell
  gofmt -w types/image_billing.go types/image_billing_test.go relay/common/relay_info.go relay/helper/price.go relay/helper/price_test.go relay/channel/openai/relay_image.go relay/channel/openai/image_stream_test.go service/quota.go service/quota_test.go service/log_info_generate.go
  go test ./types ./relay/helper ./relay/channel/openai ./service -run 'Test.*(ImageBilling|ImageSKU|ImageSettlement|QuotaSaturation)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add types/image_billing.go types/image_billing_test.go relay/common/relay_info.go relay/helper/price.go relay/helper/price_test.go relay/channel/openai/relay_image.go relay/channel/openai/image_stream_test.go service/quota.go service/quota_test.go service/log_info_generate.go
  git commit -m "feat: bill image requests from SKU snapshots"
  ```

## Task 6：`manual`/`lowest_cost` 选择、毛利过滤和安全重试

**Files:**

- Modify: `model/ability.go`
- Modify: `model/channel_cache.go`
- Create: `model/channel_selection_test.go`
- Create: `service/image_routing.go`
- Create: `service/image_routing_test.go`
- Modify: `service/model_routing.go`
- Modify: `service/model_routing_test.go`
- Modify: `relay/common/relay_info.go`
- Modify: `relay/cost_accounting_adaptor.go`
- Modify: `controller/relay.go`
- Modify: `controller/relay_routing_test.go`

- [ ] 先写模型层回归测试，在 memory cache 开/关两种路径断言候选清单相同，现有 manual 仍按 Priority 降序分层、同层 Weight 加权、`ExcludedChannelIDs` 生效。

- [ ] 抽出不做随机选择的候选 API，并让现有 `GetRandomSatisfiedChannel` 复用它：

  ```go
  type SatisfiedChannel struct {
      Channel  *Channel
      Priority int64
      Weight   int
  }

  func ListSatisfiedChannels(group, modelName, requestPath string, filter ChannelSelectFilter) ([]SatisfiedChannel, error)
  func SelectManualChannel(candidates []SatisfiedChannel, priorityRetry int) *Channel
  ```

  该重构必须保持模型名 fallback、Advanced Custom path filter 和两种缓存模式一致。

- [ ] 在 `service/image_routing_test.go` 写确定性失败测试：成本低者优先；成本完全相同后 Priority 高者优先；成本和 Priority 相同才调用可注入的 Weight 选择器；严格模式缺成本排除；tracking/disabled 未知成本最后；最低毛利排除；失败渠道在下一次完整重算中不再出现。

- [ ] 实现图像选择数据结构和入口：

  ```go
  type ImageRouteCandidate struct {
      ChannelID, Priority, Weight int
      UpstreamModel, SKUKey string
      CostKnown bool
      EstimatedCostNanoUSD int64
      EstimatedRevenueNanoUSD int64
      RuleID int64
      RuleVersion int
      ExclusionReason string
  }

  type ImageRouteDecision struct {
      Strategy image_setting.Strategy
      Selected *ImageRouteCandidate
      Candidates []ImageRouteCandidate
  }

  func BuildImageRouteDecision(param *RetryParam, group string, candidates []model.SatisfiedChannel) (ImageRouteDecision, error)
  ```

  `per_image` 成本 = 单价 × n，`per_request` 保持完整按次，`free` 明确为 0；token/duration 在首期标记未知。排序必须是已知成本、预计总成本升序、Priority 降序、Weight；不得先截断最高 Priority 层。

- [ ] 复用 active 成本规则批量查询和现有最低毛利公式。`manual` 继续走旧选择器；`lowest_cost` 才使用新 decision。严格模式所有候选缺成本或低于毛利时返回 503；普通用户错误不暴露价格，完整 decision 写 `admin_info.image_routing`。

- [ ] 给 `RelayInfo` 增加最近一次 `CostOutcome *types.CostOutcome`，由 `costAccountingAdaptor.DoRequest/DoResponse` 写入。同步图像请求只在 outcome 明确为 `not_dispatched` 时允许换渠道；`cost_unknown` 或 `UpstreamAccepted=true` 立即停止重试，防止同一生图请求重复扣供应商费用。明确的、确认未接受请求的可重试 HTTP 错误继续排除当前渠道后重算。

- [ ] 运行：

  ```powershell
  gofmt -w model/ability.go model/channel_cache.go model/channel_selection_test.go service/image_routing.go service/image_routing_test.go service/model_routing.go service/model_routing_test.go relay/common/relay_info.go relay/cost_accounting_adaptor.go controller/relay.go controller/relay_routing_test.go
  go test ./model ./service ./relay ./controller -run 'Test.*(SatisfiedChannel|ImageRoute|LowestCost|ImageRetry|Profit)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add model/ability.go model/channel_cache.go model/channel_selection_test.go service/image_routing.go service/image_routing_test.go service/model_routing.go service/model_routing_test.go relay/common/relay_info.go relay/cost_accounting_adaptor.go controller/relay.go controller/relay_routing_test.go
  git commit -m "feat: route image requests by supplier cost"
  ```

## Task 7：兼容性测试 API、合同哈希和状态防伪

**Files:**

- Create: `service/image_compatibility.go`
- Create: `service/image_compatibility_test.go`
- Create: `controller/channel_image_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel_authz_test.go`
- Modify: `router/channel-router.go`
- Modify: `router/channel_router_test.go`

- [ ] 先写失败测试：普通 Add/Update 不能注入 `passed`；未改合同的普通更新保留数据库已有结果；路径、映射、能力、目录版本变化使旧 hash 失效；测试 API 成功/失败状态；响应和日志不含 Key、Authorization、完整上游 body 或签名 URL。

- [ ] 实现合同哈希与合并规则：

  ```go
  type ImageCompatibilityTestRequest struct {
      PublicModel string                `json:"model" binding:"required"`
      Endpoint    imageprofile.Endpoint `json:"endpoint" binding:"required"`
  }

  type ImageCompatibilityTestResult struct {
      Status         string `json:"status"`
      ProfileVersion int    `json:"profile_version"`
      ContractHash   string `json:"contract_hash"`
      TestedAt       int64  `json:"tested_at"`
      ErrorSummary   string `json:"error_summary,omitempty"`
  }

  func ImageContractHash(channel *model.Channel, publicModel string, endpoint imageprofile.Endpoint, catalog image_setting.Catalog) (string, error)
  func MergeStoredImageCompatibility(stored, submitted *model.Channel) error
  func RunImageCompatibilityTest(ctx context.Context, channel *model.Channel, request ImageCompatibilityTestRequest) (ImageCompatibilityTestResult, error)
  ```

  hash 输入为结构化 canonical JSON：档案名/版本、目录版本、公共模型、映射后模型、端点路径、有效能力；使用 SHA-256。普通 CRUD 忽略 submitted compatibility，只保留 stored 中 hash 仍匹配的条目；新增渠道全部清空。

- [ ] `RunImageCompatibilityTest` 复用 OpenAI image adaptor 和现有鉴权/override/代理：generations 发小尺寸、`n=1`、固定 prompt；edits 使用内置最小 PNG，支持 mask 时额外带 mask。只接受合法 OpenAI Images 响应且至少一张图，验证模型映射后的上游请求。

- [ ] 新增 `POST /api/channel/:id/image-profile/test`，权限为 `authz.ChannelOperate`。只有该 handler 可写 compatibility 结果；持久化时只更新 `settings` JSON 并刷新 channel cache。失败摘要截断并脱敏。

- [ ] 在 `controller.UpdateChannel` 调用 `MergeStoredImageCompatibility`；在敏感字段分类测试中明确新增 request 字段属于非密钥配置，测试端点仍受管理员权限保护。

- [ ] 运行：

  ```powershell
  gofmt -w service/image_compatibility.go service/image_compatibility_test.go controller/channel_image_test.go controller/channel.go controller/channel_authz_test.go router/channel-router.go router/channel_router_test.go
  go test ./service ./controller ./router -run 'Test.*(ImageCompatibility|ImageProfileTest|ChannelFields)' -count=1 -p=1
  ```

- [ ] 提交：

  ```powershell
  git add service/image_compatibility.go service/image_compatibility_test.go controller/channel_image_test.go controller/channel.go controller/channel_authz_test.go router/channel-router.go router/channel_router_test.go
  git commit -m "feat: test image channel compatibility"
  ```

## Task 8：渠道页图像协议、测试状态和供应商成本入口

**Files:**

- Create: `web/src/features/channels/lib/image-profile.ts`
- Create: `web/src/features/channels/lib/image-profile.test.ts`
- Modify: `web/src/features/channels/types.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Create: `web/src/features/channels/lib/channel-form.test.ts`
- Create: `web/src/features/channels/components/drawers/sections/channel-image-profile-section.tsx`
- Modify: `web/src/features/channels/components/drawers/sections/index.ts`
- Modify: `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- Modify: `web/src/features/channels/lib/channel-actions.ts`
- Modify: `web/src/features/cost-accounting/types.ts`
- Modify: `web/src/features/cost-accounting/lib/cost-rule.ts`
- Modify: `web/src/features/cost-accounting/lib/cost-rule.test.ts`
- Modify: `web/src/features/cost-accounting/components/cost-rule-drawer.tsx`

- [ ] 先写 Bun 失败测试：解析/序列化档案、只收窄能力、保留 `settings` 未知字段、compatibility 只读、路径校验、per-image 成本表单序列化。

- [ ] 在 `image-profile.ts` 使用严格 Zod schema 导出：

  ```ts
  export type ImageEndpoint = 'generations' | 'edits'
  export type ImageCompatibilityStatus = 'passed' | 'failed' | 'untested'

  export function parseImageProfile(settings: string): ImageProfileBinding | null
  export function mergeImageProfile(
    settings: string,
    binding: EditableImageProfileBinding | null
  ): string
  export function validateCapabilityNarrowing(
    catalog: ImageModelCatalog,
    binding: EditableImageProfileBinding
  ): Record<string, string>
  ```

  `mergeImageProfile` 从原 JSON 开始，仅替换 `image_profile` 的可编辑字段，原样保留未知顶层键和服务端 compatibility。

- [ ] 在渠道抽屉新增紧凑的“图像协议” section：档案选择固定为 `OpenAI Images Compatible`、版本 1、generations/edits 开关、路径覆盖、按模型能力收窄、生成/编辑测试按钮和状态。绑定时提示渠道应选择 OpenAI-compatible 类型，不提供原生 Gemini 转换选项。

- [ ] `channel-actions.ts` 增加：

  ```ts
  export async function testImageProfile(
    channelId: number,
    request: { model: string; endpoint: ImageEndpoint }
  ): Promise<ImageCompatibilityTestResult>
  ```

  测试按钮需要 loading/disabled/error 状态，成功后刷新渠道 query；错误只显示服务端脱敏摘要。

- [ ] 在供应商成本类型和表单中加入 `per_image`，字段使用“每张图成本”和 SKU variant；渠道 section 的成本入口复用现有 `ChannelCostDrawer`，以 channel id、映射后模型和 SKU key 作为 seed，不复制成本 CRUD。

- [ ] 运行：

  ```powershell
  Set-Location web
  bun test src/features/channels/lib/image-profile.test.ts src/features/channels/lib/channel-form.test.ts src/features/cost-accounting/lib/cost-rule.test.ts
  bun run typecheck
  Set-Location ..
  ```

- [ ] 提交：

  ```powershell
  git add web/src/features/channels web/src/features/cost-accounting
  git commit -m "feat(web): configure OpenAI Images channels"
  ```

## Task 9：全局图像目录与用户售价 UI

**Files:**

- Create: `web/src/features/system-settings/models/image-catalog-schema.ts`
- Create: `web/src/features/system-settings/models/image-catalog-schema.test.ts`
- Create: `web/src/features/system-settings/models/image-catalog-editor.tsx`
- Create: `web/src/features/system-settings/models/image-catalog-section.tsx`
- Modify: `web/src/features/system-settings/models/section-registry.tsx`
- Modify: `web/src/features/system-settings/models/index.tsx`
- Modify: `web/src/features/system-settings/api.ts`

- [ ] 先写 schema 失败测试：版本、端点默认值、能力集合、唯一 SKU、SKU 与端点组合一致、非负有限十进制字符串、`max_n` 上限、删除默认 SKU 拒绝。

- [ ] 导出前端合同：

  ```ts
  export const imageModelCatalogSchema: z.ZodType<ImageModelCatalog>
  export function parseImageModelCatalog(raw: string): ImageModelCatalog
  export function serializeImageModelCatalog(value: ImageModelCatalog): string
  export function buildImageSKUKey(
    endpoint: ImageEndpoint,
    size: string,
    quality: string
  ): string
  ```

- [ ] 在模型设置新增无嵌套卡片的目录 section：模型列表、generations/edits tabs、能力表单、默认值、SKU 表格和每张图 USD 售价。尺寸/质量使用可编辑 option set，端点用 tabs，布尔能力用 checkbox/switch，命令按钮使用现有图标库并带 tooltip。

- [ ] 保存复用系统设置批量更新 API，只提交 `ImageModelCatalog`；后端校验错误定位到模型/端点/SKU。目录版本必须显式递增后才能保存合同变更，并在 UI 说明相关兼容性测试会失效。

- [ ] 运行：

  ```powershell
  Set-Location web
  bun test src/features/system-settings/models/image-catalog-schema.test.ts
  bun run typecheck
  Set-Location ..
  ```

- [ ] 提交：

  ```powershell
  git add web/src/features/system-settings/models web/src/features/system-settings/api.ts
  git commit -m "feat(web): manage image model catalog"
  ```

## Task 10：图像路由策略和候选预览 UI

**Files:**

- Create: `controller/image_routing_preview.go`
- Create: `controller/image_routing_preview_test.go`
- Modify: `router/routing-policy-router.go`
- Modify: `router/routing_policy_router_test.go`
- Create: `web/src/features/system-settings/models/image-routing-schema.ts`
- Create: `web/src/features/system-settings/models/image-routing-schema.test.ts`
- Create: `web/src/features/system-settings/models/image-routing-section.tsx`
- Create: `web/src/features/system-settings/models/image-routing-preview-dialog.tsx`
- Modify: `web/src/features/system-settings/models/section-registry.tsx`
- Modify: `web/src/features/system-settings/models/index.tsx`
- Modify: `web/src/features/model-routing/api.ts`
- Modify: `web/src/features/model-routing/types.ts`

- [ ] 先写 controller 失败测试：仅管理员可预览；request 校验和实际路由一致；响应包含渠道、成本已知状态、规则版本、预计成本、毛利和排除原因；不含 Key、Cookie、header override、签名 URL 或上游正文。

- [ ] 新增 `POST /api/routing-policies/image/preview`，权限 `authz.ChannelRead`，请求/响应固定为：

  ```go
  type ImageRoutingPreviewRequest struct {
      Group, Model, Endpoint, Size, Quality, ResponseFormat string
      N uint `json:"n"`
      InputImages uint `json:"input_images"`
      HasMask bool `json:"has_mask"`
  }

  type ImageRoutingPreviewResponse struct {
      Strategy string `json:"strategy"`
      SKU string `json:"sku"`
      SelectedChannelID *int `json:"selected_channel_id,omitempty"`
      Candidates []ImageRouteCandidate `json:"candidates"`
  }
  ```

  controller 调用 Task 6 的同一 `BuildImageRouteDecision`，不得另写排序公式。

- [ ] 前端路由 schema 支持 default 及 group/model override，策略仅 `manual | lowest_cost`，毛利 `0..10000`，兼容性测试开关为 boolean。保存只提交 `ImageRoutingPolicy`。

- [ ] 新增图像路由 section：按分组/公共模型编辑策略、最低毛利和测试要求；候选预览 dialog 展示预计成本、规则版本、Priority、Weight、兼容性状态和排除原因。供应商金额仅管理员可见。

- [ ] 运行：

  ```powershell
  gofmt -w controller/image_routing_preview.go controller/image_routing_preview_test.go router/routing-policy-router.go router/routing_policy_router_test.go
  go test ./controller ./router -run 'Test.*ImageRoutingPreview' -count=1 -p=1
  Set-Location web
  bun test src/features/system-settings/models/image-routing-schema.test.ts
  bun run typecheck
  Set-Location ..
  ```

- [ ] 提交：

  ```powershell
  git add controller/image_routing_preview.go controller/image_routing_preview_test.go router/routing-policy-router.go router/routing_policy_router_test.go web/src/features/system-settings/models web/src/features/model-routing
  git commit -m "feat: preview image routing decisions"
  ```

## Task 11：全部前端文案国际化与可访问性回归

**Files:**

- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Create: `web/src/features/channels/components/drawers/__tests__/channel-image-profile-accessibility.test.tsx`
- Create: `web/src/features/system-settings/models/image-settings-accessibility.test.tsx`

- [ ] 使用项目 `i18n-translate` skill 扫描本计划新增 UI 的所有 `t('English key')`；先运行 `bun run i18n:sync`，确认新增 key 出现在七个 locale 的差异中。

- [ ] 为 en、zh、zh-TW、fr、ja、ru、vi 填写真实翻译；协议名、模型名、SKU、API path 保持原文。不得用英文值占位非英文 locale。

- [ ] 添加可访问性组件测试：section 标题关联、输入 label、端点 tabs 键盘切换、测试按钮 loading name、错误摘要 live region、成本入口和预览 dialog 的焦点返回。

- [ ] 运行：

  ```powershell
  Set-Location web
  bun run i18n:sync
  bun test src/features/channels/components/drawers/__tests__/channel-image-profile-accessibility.test.tsx src/features/system-settings/models/image-settings-accessibility.test.tsx
  bun run lint
  bun run typecheck
  Set-Location ..
  ```

  预期：同步命令不再产生 locale 差异，测试/lint/typecheck 全部通过。

- [ ] 提交：

  ```powershell
  git add web/src/i18n/locales web/src/features/channels/components/drawers/__tests__ web/src/features/system-settings/models/image-settings-accessibility.test.tsx
  git commit -m "feat(i18n): translate image channel settings"
  ```

## Task 12：Mock E2E、三数据库验证、文档和灰度验收

**Files:**

- Create: `e2e/openai_images_channel_e2e_test.go`
- Create: `model/image_configuration_database_test.go`
- Modify: `docs/api/image-generation.md`
- Create: `docs/openai-images-channel.md`

- [ ] 先写 `httptest` Mock E2E，建立两个支持同一公共模型/SKU 的 OpenAI-compatible 渠道：A 成本低，B 成本高。测试 generations、multipart edits、A 明确未接受后重试 B、A 状态不确定时不发 B、strict 缺成本 503、最低毛利 503、manual 回归。

- [ ] E2E 同时断言：公共模型映射到各自上游模型；路径 override 生效；响应实际图数结算；普通日志隐藏供应商成本；管理员日志包含档案/合同 hash/SKU/策略/候选/规则版本/meter source；没有真实供应商网络请求。

- [ ] 在 `model/image_configuration_database_test.go` 复用项目 DSN 环境变量，对 SQLite 默认运行，对 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 条件运行同一 fixture：Option JSON 保存/读取、渠道 settings round-trip、`per_image` 成本规则 draft/activate/lookup。测试不得对外部已有表执行破坏性迁移。

- [ ] 更新 `docs/api/image-generation.md`，明确下游支持 generations/edits、首期不支持 variations、原生 Gemini 不走此档案。新增简体中文 `docs/openai-images-channel.md`，写清管理员配置顺序：全局目录 → OpenAI-compatible 渠道/映射 → compatibility test → per-image 成本草稿/激活 → image 分组策略 → 预览 → 灰度。

- [ ] 运行完整后端验证：

  ```powershell
  go test ./pkg/imageprofile ./setting/image_setting ./relaykit/dto ./types ./model ./service ./relay/... ./controller ./router ./e2e -count=1 -p=1
  ```

  若提供数据库 DSN，再运行：

  ```powershell
  go test ./model -run 'TestImageConfigurationDatabaseCompatibility' -count=1 -p=1
  ```

- [ ] 运行完整前端验证：

  ```powershell
  Set-Location web
  bun test src/features/channels src/features/cost-accounting src/features/model-routing src/features/system-settings/models
  bun run lint
  bun run typecheck
  bun run build
  Set-Location ..
  ```

- [ ] 执行静态检查并核对只含预期文件：

  ```powershell
  git diff --check
  git status --short
  ```

- [ ] 灰度时保持全局 `manual`，只对测试 `image` 分组的一个公共模型启用 `lowest_cost`。通过候选预览和管理员日志确认成本/规则/hash/实际图数一致后，再逐模型扩大；回滚只需把策略切回 `manual`，无需修改 `sd收录`。

- [ ] 提交：

  ```powershell
  git add e2e/openai_images_channel_e2e_test.go model/image_configuration_database_test.go docs/api/image-generation.md docs/openai-images-channel.md
  git commit -m "test: verify unified OpenAI Images channels"
  ```

## 最终验收清单

- [ ] 下游 `gpt-image` 和上游提供 OpenAI-compatible endpoint 的 `gemini-*-image` 均通过现有 OpenAI Images generations/edits 接入；原生 Gemini `generateContent` 不被误接。
- [ ] 公共能力和用户 SKU 售价只维护在 `ImageModelCatalog`；渠道页只维护映射、收窄能力、路径、测试状态和供应商成本入口。
- [ ] `manual` 与现有 Priority/Weight 行为一致；`lowest_cost` 严格按成本 → Priority → Weight，失败排除后完整重算。
- [ ] strict 缺成本 fail closed；tracking/disabled 未知成本最后回退并写管理员审计。
- [ ] 已接受或状态不确定的图片请求不自动换供应商；明确未 dispatch 的失败才允许重试。
- [ ] 售价和成本分别使用请求快照；`n`、实际图数、decimal 乘积和 quota saturation 全程有界且不会产生负扣费。
- [ ] compatibility `passed` 只能由测试 API 写入，任何合同变化使旧 hash 失效。
- [ ] SQLite、MySQL、PostgreSQL 行为一致；Mock E2E 不产生真实供应商费用。
- [ ] `refreshing-sd-channel-config`、Seedance 模板、导入发布和素材矩阵不受影响。

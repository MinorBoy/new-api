# OpenAI Images 统一渠道接入设计

**日期：** 2026-09-01
**状态：** 设计已获确认，等待 spec 审阅
**范围：** OpenAI Images-compatible 图像渠道、全局图像模型目录、供应商成本和成本感知路由

## 1. 背景与目标

当前 new-api 已有 `/v1/images/generations` 和 `/v1/images/edits` 路由，也已有 OpenAI、Gemini 及多个图像渠道适配器。问题在于图像模型的协议能力、用户售价和各上游成本分散在渠道配置、模型价格设置和 `sd收录` 表中，导致同一模型需要重复维护，且人工录入容易出现映射、价格单位和能力错误。

本设计的目标是：

1. 以一个内置 `openai_images` 协议档案统一真实兼容 OpenAI Images 的上游。
2. 在全局位置维护公共图像模型能力和下游用户售价。
3. 在渠道页完成上游模型映射、能力收窄、路径覆盖、兼容性测试和供应商成本配置。
4. 允许多个渠道加入同一个用户分组，并按最低供应商成本优先路由，同时保留 Priority/Weight 的可控行为。
5. 复用现有成本规则、利润审计、配置发布和日志体系，保证预扣、结算和重试不会产生负计费或重复扣费。
6. 让 `refreshing-sd-channel-config` 继续只服务 Seedance，不再把图像协议能力作为其日常事实来源。

## 2. 首期边界

### 2.1 纳入范围

- 上游必须真实支持 OpenAI Images-compatible 的 `/v1/images/generations` 和/或 `/v1/images/edits`。
- 下游统一使用现有 OpenAI Images 请求格式。
- 公共模型名可以是 `gpt-image-1`、`gpt-image-2`、`gemini-*-image` 等，但模型名本身不代表协议能力；每个渠道必须明确绑定和映射。
- 支持 JSON 生成请求、multipart 编辑请求、尺寸、质量、`n`、响应格式、输入图和 mask 等档案能力。
- 原生 Gemini `generateContent` 生图不纳入 `openai_images`；继续使用现有 Gemini 协议和 `imagen`/Gemini 图像适配路径。上游若额外提供 OpenAI-compatible endpoint，可以为同一供应商建立单独的 `openai_images` 渠道绑定。

### 2.2 不纳入范围

- 不把原生 Gemini `generateContent` 强行转换成 OpenAI Images。
- 不根据模型名称自动创建渠道、自动猜测上游 endpoint 或自动推断编辑能力。
- 首期不替换现有所有非 OpenAI Images 图像渠道适配器。
- 首期不新建多张图像业务主表；渠道绑定和全局目录使用现有 JSON 配置存储，后续在查询和审计压力明确后再规范化迁移。

## 3. 现有代码基线

- OpenAI Images 路由和主处理链位于 `router/relay-router.go`、`relay/image_handler.go`。
- OpenAI 图像适配器位于 `relay/channel/openai/adaptor.go` 和 `relay/channel/openai/relay_image.go`。
- Gemini 图像转换目前只接受 `imagen` 前缀，位于 `relay/channel/gemini/adaptor.go`。
- 渠道连接、模型列表、模型映射、参数覆盖和其他设置位于 `model/channel.go`、`relaykit/dto/channel_settings.go` 及 `web/src/features/channels`。
- 现有渠道选择按分组和下游模型筛选，Priority 分层后使用 Weight 加权，位于 `model/channel_cache.go`、`model/ability.go`。
- 现有能力路由和最低毛利过滤主要服务 Seedance，位于 `model/routing_policy.go`、`service/model_routing.go`。
- 供应商成本由版本化 `model.ChannelModelCostRule` 管理，已有成本草稿、校验、激活、结算、目录和审计流程。
- 全局用户价格目前位于 `setting/ratio_setting` 及模型定价界面；图像请求已有 `MaxImageN`、尺寸/质量倍率和 `BillingRatios["n"]`。

## 4. 总体架构

### 4.1 四层事实来源

1. **协议档案注册表（内置代码）**

   `openai_images` 只描述协议格式、默认路径、请求/响应转换和安全上限。档案版本是稳定的运行时契约，不保存供应商价格和凭据。

2. **全局图像模型目录（系统配置）**

   按公共模型维护端点、能力和下游售价。它决定用户看到的模型是否可用，以及同一请求的用户收入如何计算。

3. **渠道绑定（`ChannelOtherSettings` JSON）**

   渠道页选择档案版本，复用现有 `Models` 和 `ModelMapping`，并保存路径覆盖、能力收窄和兼容性测试状态。该层不保存 API Key，不复制全局售价。

4. **供应商成本规则（`ChannelModelCostRule`）**

   按渠道、上游模型和图像 SKU 变体保存版本化实际成本。成本规则继续遵循草稿、审阅、激活和退休状态机。

### 4.2 请求数据流

```text
下游 OpenAI Images 请求
  -> 读取公共模型和端点
  -> 全局目录校验尺寸/质量/响应格式/n/输入图
  -> 解析图像 SKU
  -> 按分组、档案、模型映射和能力覆盖筛选渠道
  -> 查找渠道 SKU 成本规则
  -> 最低毛利过滤
  -> manual 或 lowest_cost 选择器
  -> OpenAI Images 适配器发往上游
  -> 解析图像数量和响应
  -> 用户收入、供应商成本、利润和路由审计
```

## 5. 配置契约

### 5.1 渠道图像档案绑定

`relaykit/dto.ChannelOtherSettings` 增加可选的 `image_profile`，旧渠道没有该字段时行为不变。概念结构如下：

```json
{
  "image_profile": {
    "profile": "openai_images",
    "profile_version": 1,
    "paths": {
      "generations": "/v1/images/generations",
      "edits": "/v1/images/edits"
    },
    "capability_overrides": {
      "gpt-image-1": {
        "generations": true,
        "edits": false,
        "sizes": ["1024x1024"],
        "qualities": ["standard"],
        "response_formats": ["b64_json"],
        "max_n": 1,
        "max_input_images": 0,
        "supports_mask": false
      }
    },
    "compatibility": {
      "gpt-image-1:generations": {
        "status": "passed",
        "profile_version": 1,
        "contract_hash": "...",
        "tested_at": 0
      }
    }
  }
}
```

约束：

- `profile` 只能引用已注册档案，`profile_version` 必须是支持的版本。
- `paths` 只能是相对 HTTP 路径或经过现有渠道 URL 校验的完整 URL，不允许查询字符串中写入凭据。
- `capability_overrides` 只能收窄全局目录和协议档案能力，不能通过渠道配置无条件放宽 `max_n`、输入图数量或响应格式。
- 公共模型到上游模型继续使用 `Models` 和 `ModelMapping`；同一渠道不能存在含义冲突的两套映射。
- `compatibility` 由测试 API 写入，普通 JSON 编辑器不能伪造已通过状态。档案、路径、映射或能力变化后，旧测试状态按 `contract_hash` 失效。
- 渠道认证继续使用现有 `Key`、`HeaderOverride`、`ParamOverride` 和代理配置；`image_profile` 不保存密钥。

### 5.2 全局图像模型目录

全局目录使用现有系统选项存储，值为版本化 JSON，价格使用十进制定点字符串：

```json
{
  "version": 1,
  "models": {
    "gpt-image-1": {
      "profile": "openai_images",
      "profile_version": 1,
      "endpoints": {
        "generations": {
          "sizes": ["1024x1024", "1536x1024", "1024x1536"],
          "qualities": ["low", "medium", "high"],
          "response_formats": ["url", "b64_json"],
          "max_n": 10,
          "max_input_images": 0,
          "supports_mask": false
        },
        "edits": {
          "sizes": ["1024x1024"],
          "qualities": ["medium", "high"],
          "response_formats": ["url", "b64_json"],
          "max_n": 1,
          "max_input_images": 16,
          "supports_mask": true
        }
      },
      "skus": {
        "gen-1024x1024-medium": {
          "endpoint": "generations",
          "size": "1024x1024",
          "quality": "medium",
          "unit": "image",
          "sale_price_usd": "0.040000"
        }
      }
    }
  }
}
```

目录规则：

- 缺少模型、端点或精确 SKU 时，返回明确的配置错误，不把未知价格默认为零。
- 未传尺寸或质量时使用目录声明的默认值，并把归一化后的值写入路由和计费快照。
- `sale_price_usd` 是单张图价格；`n` 是经过 `dto.MaxImageN` 限制的整数倍数。
- 现有普通模型价格表继续兼容；对于启用 `openai_images` 的公共模型，图像 SKU 价格优先于单一模型价格。

### 5.3 供应商成本契约

扩展现有成本类型：

- 新增 `CostModePerImage = "per_image"`。
- `CostRuleConfigV1.UnitPrice` 在该模式下表示供应商每张图的实际成本，沿用现有货币、汇率、手续费和折扣归一化流程。
- `CostMeter` 增加有界的 `ImageCount` 字段。
- `CostVariantKey` 由稳定 SKU 解析器生成，例如 `gen-1024x1024-medium` 或 `edit-1024x1024-high`；不把 `n` 放进 variant，避免为每个数量创建规则。
- 供应商固定按请求收费继续使用 `per_request`，其成本不因 `n` 自动摊薄；配置必须明确选择计费模式。

每个渠道、上游模型和 SKU 只能有一个 active 成本规则。成本规则缺失时：

- 严格成本模式：候选渠道排除，并记录 `cost_rule_missing`。
- tracking/disabled 模式：已知成本优先，未知成本只能作为最后回退，并记录管理员审计信息。

### 5.4 图像路由策略

新增全局图像路由策略配置，按用户分组和公共模型生效，默认 `manual` 以保持兼容：

```json
{
  "default": {
    "strategy": "manual"
  },
  "groups": {
    "image": {
      "gpt-image-1": {
        "strategy": "lowest_cost",
        "minimum_expected_margin_bps": 100,
        "require_compatibility_test": false
      }
    }
  }
}
```

策略含义：

- `manual`：复用现有 Priority 分层和 Weight 加权。
- `lowest_cost`：成本是第一排序键，Priority 是成本相同的第二排序键，Weight 只在同成本同 Priority 候选中加权。
- `minimum_expected_margin_bps` 缺省时使用全局最低毛利率；指定值只能在合法范围内覆盖全局值。
- `require_compatibility_test` 为真时，未通过当前档案版本测试的渠道不参与路由；为假时，`failed` 仍排除，`untested` 允许但记录告警。

## 6. 路由选择算法

### 6.1 候选建立

对每个图像请求按以下顺序处理：

1. 解析公共模型、请求端点和标准化后的 `size`、`quality`、`response_format`、`n`。
2. 从全局目录获取端点能力和精确 SKU；未知字段、超限 `n`、不支持的端点或组合直接返回 400。
3. 按用户分组和公共模型获取渠道，并要求渠道明确绑定 `openai_images` 或通过已有适配器声明支持该图像端点。
4. 应用 `ModelMapping`，得到每个渠道的上游模型；应用渠道能力收窄、路径和兼容性状态。
5. 以渠道、上游模型和 SKU 查找 active 成本规则，得到成本模式、规则 ID、版本和预计成本。
6. 根据用户 SKU 单价、`n` 和分组倍率计算收入，执行最低毛利过滤。

### 6.2 `lowest_cost` 排序

候选成本统一换算为本次请求的预估供应商成本：

- `per_image`：单张成本乘请求图像数量。
- `per_request`：完整按次成本，不除以数量或假设固定生成时长。
- `free`：成本为零，但仍须存在明确的免费理由。
- `per_duration`、`per_token`：首期图像策略不自动猜测所需计量；若没有图像专用计量契约，候选按成本未知处理。

排序规则：

1. 已知成本优先于未知成本。
2. 已知成本按预计总成本升序。
3. 成本完全相同时按 `Priority` 降序。
4. 成本和 Priority 都相同时按 `Weight` 加权随机。
5. 重试排除已失败渠道后重新运行完整排序，而不是只递增旧的 Priority 层。

这样可以让请求默认更多地进入价格最低的渠道，同时仍能通过 Priority 和 Weight 处理同价渠道的可靠性和流量分配。首期不引入近似成本桶或复杂概率混合；需要在低价和高可用之间混合时，后续可增加显式成本容差配置。

### 6.3 失败和重试

- 请求校验、SKU 不存在、能力不支持和映射错误属于客户端或配置错误，不重试。
- 上游明确未接受请求的可重试 HTTP 错误，遵循现有全局重试状态码和 `RetryTimes`，重新选择候选。
- 上游已接受但响应丢失、超时或状态不确定时，不自动把同一图像请求发送到下一渠道，避免供应商重复扣费；成本尝试标记为 awaiting/unknown，进入现有异常审计流程。
- 图像请求的默认重试次数保持当前系统设置，不因选择 `lowest_cost` 隐式增加供应商请求次数。

## 7. 计费、结算和安全不变量

### 7.1 用户收入

用户收入由公共模型 SKU 单价、请求 `n` 和分组倍率决定，不读取渠道成本，也不因路由到不同供应商而改变。预扣时保存模型目录版本、SKU、单价、`n` 和分组倍率快照；结算使用同一快照，防止管理员中途改价造成前后不一致。

### 7.2 供应商成本

供应商成本由选中渠道的 active 成本规则版本决定。`per_image` 在预扣和结算都使用有界图像数量；优先使用响应中明确解析出的实际图像数，缺少可靠数量时按该规则声明的回退策略处理并记录 meter source。

### 7.3 安全约束

- 请求 `n` 必须复用 `dto.MaxImageN`，覆盖 JSON、multipart 和 passthrough 路径。
- 所有图像数量、成本乘数和价格乘积都必须通过 `common.QuotaFromFloatChecked`、`common.QuotaRoundChecked` 或 `common.QuotaFromDecimalChecked` 转换，禁止裸 `int` 转换。
- `PriceData.AddOtherRatio` 继续作为倍率入口，禁止直接写入 `OtherRatios`。
- NaN、Infinity、负价格、负数量和超过 int32 饱和值的计算必须失败闭环或安全饱和，并把 `QuotaClamp` 追加到管理员日志。
- 预扣不足必须拒绝请求，不能因成本溢出变成负扣费；结算失败必须保留成本尝试和异常状态。

## 8. 管理界面和接口

### 8.1 渠道页

在现有渠道抽屉中增加“图像协议”区域：

- 选择 `OpenAI Images Compatible` 和档案版本。
- 选择 generations/edits 端点并编辑可选路径覆盖。
- 继续使用现有模型列表和模型映射编辑器。
- 对每个公共模型编辑只能收窄的能力例外。
- 提供“生成测试”和“编辑测试”按钮，显示测试状态、档案版本和最近错误摘要。
- 提供供应商成本入口，直接创建/编辑成本规则草稿并跳转现有审阅、校验和激活流程。
- 未知设置字段必须在保存时保留；所有新增文本走前端 i18n。

### 8.2 全局模型和售价页

在模型定价设置中增加图像模型目录编辑器：

- 模型端点和能力为全局公共事实。
- SKU 价格以每张图展示，支持生成/编辑、尺寸和质量维度。
- 保存前校验 SKU 唯一性、价格非负有限、默认 SKU 存在和 `max_n` 上限。
- 修改目录版本会使相关渠道的旧兼容性测试状态失效。

### 8.3 路由设置页

在模型路由设置中增加图像分组/模型策略：

- `manual` 或 `lowest_cost`。
- 最低毛利率覆盖。
- 是否要求兼容性测试。
- 展示当前候选渠道、成本规则版本、预计成本和排除原因。

### 8.4 后端接口

复用现有渠道 CRUD、模型定价、成本规则和渠道测试接口，新增字段和 endpoint 类型即可；不创建第二套认证或密钥接口。接口响应中的管理员诊断应包含档案、端点、SKU、成本规则版本和选择策略，但不得包含 Key、Cookie、签名 URL 或完整上游响应。

## 9. 兼容性测试

### 9.1 测试合同

每个公共模型和端点至少验证：

- 请求路径和鉴权可用。
- 生成请求能返回合法 OpenAI Images 响应且包含至少一张图。
- 编辑请求能接收最小 PNG 输入；声明支持 mask 时额外验证 mask。
- 返回的 URL 或 `b64_json` 字段符合档案响应契约。
- 上游模型映射确实生效，未把客户端模型名错误透传。

测试请求使用小尺寸、`n=1` 和固定最小输入；真实供应商测试必须由管理员显式触发。默认自动化测试使用 `httptest` Mock，不产生供应商费用。

### 9.2 状态处理

- `passed`：当前档案版本和 contract hash 下可正常路由。
- `failed`：对应模型/端点排除，失败摘要进入管理员审计。
- `untested`：按路由策略决定是否允许；允许时记录告警。
- 配置、档案版本、映射或能力改变后，旧状态不可继续代表新配置。

## 10. 数据迁移和兼容性

- 首期不新增图像绑定表；`image_profile` 放入已有 `OtherSettings` JSON。
- 全局图像目录放入已有系统选项，不新增必需数据库表。
- `CostModePerImage` 和 `CostMeter.ImageCount` 是向后兼容的 JSON/枚举扩展，旧成本规则继续按原模式运行。
- SQLite、MySQL 和 PostgreSQL 不需要新增方言 SQL；若后续规范化为表，必须按项目数据库兼容规则使用 GORM 和跨数据库迁移。
- 没有 `image_profile` 的旧渠道保持现有路由和适配器行为；不会自动把所有 OpenAI 类型渠道标记为兼容。
- 配置导入/导出应携带图像档案和能力配置，但排除 Key、Token、Cookie 和测试响应正文。
- `sd收录` 中已有图像行不自动删除；新的图像运行时事实以全局目录、渠道绑定和成本规则为准。`refreshing-sd-channel-config` 继续只处理 Seedance。

## 11. 错误、日志和审计

客户端可见错误：

- 400：模型、端点、尺寸、质量、响应格式、`n` 或输入图不符合全局目录。
- 503：没有可用的兼容渠道、严格成本模式下没有成本覆盖、或所有候选均低于最低毛利。
- 502/上游映射错误：由现有 relay 错误转换和状态码映射处理。

管理员诊断至少包含：

- 公共模型、上游模型、端点和 SKU。
- 档案名称/版本和 contract hash。
- 路由策略、候选渠道、预计成本、最低毛利和排除原因。
- 选中渠道、成本规则 ID/版本、兼容性测试状态。
- 请求图像数量、实际结算数量、meter source 和配额饱和标记。

普通用户日志继续隐藏供应商价格、路由候选和 `admin_info`。

## 12. 测试计划

### 12.1 后端单元和集成测试

- `ChannelOtherSettings` 图像档案解析、未知字段保留、路径和能力收窄校验。
- 全局目录的模型、端点、SKU、默认值、价格和 `max_n` 校验。
- generations/edits OpenAI 请求转换、模型映射和响应解析。
- 原生 Gemini 请求不会进入 `openai_images` 候选池。
- `per_image` 成本规则的模式、价格、变体、图像数量和计量源校验。
- 最低成本选择的成本排序、Priority/Weight 同价行为、未知成本和最低毛利过滤。
- 重试排除失败渠道；上游 accepted/unknown 状态不重复发送。
- `n` 的最大值、零值、负数、超大无符号数和饱和配额回归。
- 成本目录、路由审计和管理员字段不泄露凭据。

### 12.2 前端测试

- 渠道表单 schema、档案版本、能力覆盖和 JSON 保留。
- 图像生成/编辑测试端点选择和状态展示。
- 全局图像 SKU 售价编辑与校验。
- 路由策略编辑、最低毛利和候选预览。
- 新增文案覆盖 `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi`。

### 12.3 Mock E2E

使用两个 OpenAI-compatible Mock 渠道：

1. 渠道 A：`per_image` 成本较低。
2. 渠道 B：成本较高但同一公共模型和 SKU 能力完整。

验证生成和编辑请求均选择 A；排除 A 后选择 B；同价渠道按 Priority/Weight 工作；成本缺失和最低毛利失败时返回预期状态；日志包含档案、SKU、成本规则和结算数量；全流程不写入真实供应商费用。

### 12.4 数据库验证

首期新增配置不需要数据库迁移；仍需运行现有 SQLite 测试，并对成本规则新增字段/枚举执行 MySQL、PostgreSQL 兼容性测试。

## 13. 实施顺序

1. 添加协议档案、全局目录结构和后端校验，先完成失败测试。
2. 添加渠道设置 JSON、模型映射/端点过滤和 OpenAI Images adapter 绑定。
3. 添加 `per_image` 成本模式、SKU 解析、预扣/结算和成本覆盖检查。
4. 添加图像 `lowest_cost` 选择器、重试重排和管理员诊断。
5. 添加渠道页、全局售价页、路由设置页和全部 i18n。
6. 完成 Mock E2E、数据库兼容性检查、文档更新和灰度开关。
7. 灰度期间默认 `manual`；确认成本和兼容性审计稳定后，再按分组启用 `lowest_cost`。

## 14. 回滚和运维

- 将图像策略切回 `manual` 可立即停止成本感知选择。
- 禁用某渠道的 `image_profile` 或将其测试状态置为 failed，可停止该渠道图像流量而不影响聊天接口。
- 退休 active 成本规则并恢复上一版本，遵循现有成本规则状态机。
- 全局目录错误时不自动猜价格；先禁用受影响公共模型或恢复上一目录版本。
- 任何真实供应商测试和灰度都必须限制模型、端点和预算，并在报告中区分 Mock 与真实请求。

## 15. 验收标准

设计落地后必须同时满足：

- 管理员可在渠道页选择 `OpenAI Images Compatible`，配置 `gpt-image` 或 `gemini-image` 上游映射，完成 generations/edits 测试并保存能力例外。
- 下游只调用现有 OpenAI Images API；原生 Gemini endpoint 不会被误路由到 OpenAI Images 渠道。
- 公共模型能力和用户售价只维护一份；同一公共模型的多个渠道无需在 `sd收录` 重复登记协议能力。
- 每个渠道的供应商成本可按 SKU 配置，并可在现有成本目录中审阅、激活、查询和审计。
- `manual` 保持现有 Priority/Weight 行为；`lowest_cost` 将合格请求优先路由到预计成本最低的渠道，同价时使用 Priority/Weight。
- 严格成本模式、最低毛利过滤、重试和已接受请求保护均有效。
- 图像 `n`、成本乘积和配额转换不会产生负扣费；异常带有管理员可见审计标记。
- 现有 Seedance 配置刷新流程不被图像配置改变。

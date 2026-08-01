# Seedance 公共模型边界设计

## 目标

new-api 已接入的 GPT、GPT Image、Claude、Gemini、DeepSeek、GLM 等非 Seedance 模型继续按现有配置在模型广场、模型列表 API 和 Relay 中公开并可调用。

Seedance 家族单独应用公共产品边界。终端用户只看到并调用以下三个 Doubao 官方模型 ID：

- `doubao-seedance-2-0-260128`
- `doubao-seedance-2-0-fast-260128`
- `doubao-seedance-2-0-mini-260615`

其他 Seedance 渠道模型、旧 Mini ID 和实际路由目标属于内部实现，不得通过公共读取接口或直接模型调用暴露。

## 根因

配置导入会把公共标准模型和实际 Seedance 上游模型同时写入渠道、能力、映射、成本规则及路由目标。系统原本没有区分 Seedance 公共产品 ID 与内部路由 ID。前一版修复把这个边界错误地扩展成全局模型白名单，导致非 Seedance 模型也被隐藏和拒绝。

本次修正必须把边界限定到 Seedance 家族，不能改变其他模型的公开、所有者、定价或路由语义。

## 方案

### 前端仅隐藏 Seedance 渠道模型

不采用。该方案无法保护 `/api/pricing`、`/v1/models`、用户模型接口和直接 Relay 请求。

### 全局公共模型白名单

不采用。系统接入的 GPT、Claude、Gemini、DeepSeek、GLM 等模型本来就是公共产品；全局白名单会造成现有功能回归，并要求每次接入新家族时修改代码目录。

### Seedance 家族专用边界

采用。Seedance 身份以路由策略中的 canonical 模型与 upstream target 关系为主，模型名匹配作为历史兼容兜底：

- 非 Seedance 模型保持现有公开与调用行为。
- 三个精确 Doubao Seedance 官方 ID 公开并可调用。
- 三个公开 Seedance 路由策略引用的所有 upstream model 都视为内部 ID，即使名称不包含 `seedance`（例如 `4sdance431`、`videos-fast`、`video-2.0-pro`）。
- 其他包含 `seedance` 的模型 ID 也视为内部或旧 ID，在公共读取边界隐藏，在 Relay 渠道选择前拒绝。

该边界只处理模型 ID，不删除或改写任何内部库存。

## 模型边界

`pkg/modelrouting.CanonicalModels` 继续作为三个公开 Seedance ID 的唯一代码来源。公共判断分成两个稳定概念：

- `IsPublicSeedanceModel`：精确判断三个官方 Seedance ID。
- `SetHiddenSeedanceModels`：由路由策略缓存刷新当前 Seedance upstream model 集合，使配置导入新增别名无需修改代码。
- `IsHiddenSeedanceModel`：判断一个模型是否为已登记的 Seedance upstream model、名称匹配的历史 Seedance ID，且不在公开目录。
- `FilterPublicModels`：保留输入中的全部非 Seedance 模型和三个公开 Seedance ID，过滤其他 Seedance ID，去重并保持输入顺序。

以下内容继续保留为内部数据：

- `channels.models` 中的公共模型和上游模型。
- `channels.model_mapping` 中的公共模型到上游模型映射。
- `abilities` 中用于渠道选择的全部能力。
- 成本规则、路由目标、配置导入映射和历史审计中的上游模型。
- 旧 Mini ID `doubao-seedance-2-0-mini-260128`，仅用于内部历史兼容与规范化。

## 对外读取

### 模型广场

`GET /api/pricing` 在现有分组过滤后应用 Seedance 公共投影：

- 所有非 Seedance 定价项保持原顺序、供应商、图标、所有者和端点。
- 三个公开 Seedance 定价项固定显示为 Doubao 产品身份。
- 其他 Seedance 定价项不返回。
- `vendors` 返回公开定价项实际引用的非 Seedance 供应商，并在存在公开 Seedance 时加入 Doubao。
- `supported_endpoint` 汇总全部保留定价项需要的端点。

内部 `model.GetPricing()` 继续维护完整数据，公共投影不得原地修改共享切片。

### 模型列表 API

OpenAI、Anthropic、Gemini 兼容列表、用户可用模型接口、Dashboard 模型元数据和单模型查询都应用同一过滤规则：

- 非 Seedance 模型保留现有 owner。
- 三个公开 Seedance 的 owner 固定为 `doubao`。
- 内部 Seedance 和旧 Mini ID 不返回。
- Token 模型限制只能在以上规则内进一步缩小结果。

管理员渠道管理、模型元数据管理、成本核算和配置导入接口仍可查看完整内部模型集合。

## 公共请求校验

公共 Relay 只拒绝 `IsHiddenSeedanceModel`：

1. 三个官方 Seedance ID继续进入利润感知路由和上游模型映射。
2. 内部 Seedance 和旧 Mini ID 在渠道选择前返回协议兼容的 `model_not_found`。
3. 非 Seedance 模型不经过该产品边界，继续执行原有分组权限、Token 限制、计费和渠道选择。
4. 错误响应不包含渠道 ID、渠道名、候选模型或成本规则信息。

任务查询、内容下载等不携带新模型选择的操作不重复校验。

## 数据流

```text
内部渠道、abilities、映射、成本规则、路由目标
        |
        +--> 管理员和内部路由：完整可见
        |
        +--> 公共响应
        |      +--> 非 Seedance：原样保留
        |      +--> Seedance：仅三个 Doubao 官方 ID
        |
        +--> 公共 Relay
               +--> 非 Seedance：原流程
               +--> 三个官方 Seedance：标准路由
               +--> 其他 Seedance：model_not_found
```

## 兼容性与回退

- GPT、GPT Image、Claude、Gemini、DeepSeek、GLM 等现有公共模型不受影响。
- 三个 Doubao Seedance 官方 ID 保持兼容。
- 直接使用 Seedance 上游模型 ID 或旧 Mini ID 的客户端需迁移到对应官方 ID。
- 不修改数据库结构，继续兼容 SQLite、MySQL 和 PostgreSQL。
- 不改变管理员导入、维护和诊断内部模型的能力。
- 回退代码并重建服务即可恢复旧行为，不需要数据迁移。

## 验证策略

- 单元测试证明普通模型保留、三个官方 Seedance 保留、内部 Seedance 和旧 Mini ID 被过滤。
- 定价测试证明普通模型保留原供应商，公开 Seedance 投影为 Doubao，内部 Seedance 不出现。
- OpenAI、Anthropic、Gemini、用户模型和 Dashboard 列表同时覆盖普通模型与 Seedance 边界。
- 单模型查询允许普通模型和官方 Seedance，拒绝内部 Seedance。
- Relay 测试证明普通模型不被公共边界提前拒绝，内部 Seedance 在渠道选择前返回 `model_not_found`。
- 浏览器验收模型广场仍显示 GPT、Claude、Gemini、DeepSeek、GLM 等模型，Seedance 只出现三个官方 ID。
- 运行全量 Go 测试、前端测试、类型检查、Lint、生产构建和差异检查。

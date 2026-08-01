# 使用概览

new-api 是一个 AI API 网关，将 40+ 上游 AI 服务商（OpenAI、Anthropic、Google、Azure、AWS Bedrock 等）聚合在统一接口之后，并提供用户管理、计费、限流与管理后台。

## 双协议接入

new-api 同时兼容两种主流 API 风格，**代码无需任何改动**即可接入：

- **OpenAI 风格**：`/v1/chat/completions`、`/v1/embeddings`、`/v1/images/generations` 等，适用于绝大多数客户端。
- **Anthropic 风格**：`/v1/messages`，适用于 Claude 系列模型与原生 Claude SDK。

只需把上游官方地址替换为 new-api 的 Base URL，请求格式、参数、返回结构与官方完全一致。

## 四步接入

1. **注册账号**：在控制台注册并登录。
2. **创建令牌**：在「令牌」页面创建一个 API Key。
3. **选择协议**：根据所用模型，确认使用 OpenAI 风格还是 Anthropic 风格接口。
4. **配置客户端**：把 Base URL 与 API Key 填入你的客户端，即可开始调用。

## 接下来

- 完成[快速接入](/docs/quickstart)跑通第一个请求。
- 在[接口地址](/docs/endpoints)查看完整的端点列表。
- 前往[模型与计费](/docs/pricing)了解可用模型与价格。

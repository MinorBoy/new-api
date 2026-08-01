# 错误码参考

请求失败时，响应体遵循 OpenAI 错误格式：

```json
{
  "error": {
    "message": "详细错误信息",
    "type": "new_api_error",
    "code": "insufficient_user_quota"
  }
}
```

`code` 字段是机器可读的错误码，下表列出常见取值与含义。`type` 标识错误来源（`new_api_error` / `openai_error` / `claude_error` / `upstream_error` 等）。

## 请求类错误

| code                       | HTTP 状态 | 含义与排查                                 |
| -------------------------- | --------- | ------------------------------------------ |
| `invalid_request`          | 400       | 请求参数不合法，检查请求体字段。           |
| `bad_request_body`         | 400       | 请求体无法解析（JSON 格式错误）。          |
| `read_request_body_failed` | 400       | 读取请求体失败，通常是网络中断或体积超限。 |
| `convert_request_failed`   | 400       | 请求格式转换失败，确认所选端点与协议匹配。 |
| `sensitive_words_detected` | 400       | 触发敏感词检测。                           |
| `access_denied`            | 403       | 无权访问该资源或接口。                     |

## 模型与路由类错误

| code                             | HTTP 状态 | 含义与排查                                           |
| -------------------------------- | --------- | ---------------------------------------------------- |
| `model_not_found`                | 404       | 模型名拼写错误，或当前分组未开放该模型。             |
| `no_compatible_route`            | 404       | 没有可用渠道路由该请求，请联系管理员或检查渠道配置。 |
| `compatible_channel_unavailable` | 503       | 兼容渠道暂不可用，可稍后重试。                       |
| `invalid_api_type`               | 400       | 渠道 API 类型无效。                                  |
| `routing_policy_error`           | 400       | 路由策略校验失败。                                   |

## 计费与配额类错误

| code                             | HTTP 状态 | 含义与排查                             |
| -------------------------------- | --------- | -------------------------------------- |
| `insufficient_user_quota`        | 429       | 用户额度不足，请充值。                 |
| `pre_consume_token_quota_failed` | 429       | 预扣费失败（额度不足以覆盖预估消耗）。 |
| `model_price_error`              | 500       | 模型价格配置异常，请联系管理员。       |
| `count_token_failed`             | 500       | Token 计数失败。                       |

## 渠道类错误（`channel:*`）

渠道类错误指向上游服务调用阶段：

| code                              | HTTP 状态 | 含义                     |
| --------------------------------- | --------- | ------------------------ |
| `channel:no_available_key`        | 503       | 渠道没有可用的上游密钥。 |
| `channel:invalid_key`             | 401       | 上游密钥无效或已过期。   |
| `channel:model_mapped_error`      | 400       | 模型映射配置错误。       |
| `channel:aws_client_error`        | 400       | AWS Bedrock 客户端错误。 |
| `channel:param_override_invalid`  | 400       | 渠道参数覆盖配置无效。   |
| `channel:header_override_invalid` | 400       | 渠道请求头覆盖配置无效。 |
| `channel:response_time_exceeded`  | 504       | 上游响应超时。           |

## 上游响应类错误

| code                        | HTTP 状态 | 含义                              |
| --------------------------- | --------- | --------------------------------- |
| `do_request_failed`         | 502       | 向上游发起请求失败（网络层）。    |
| `read_response_body_failed` | 502       | 读取上游响应体失败。              |
| `bad_response_status_code`  | 502       | 上游返回非 2xx 状态码。           |
| `bad_response`              | 502       | 上游响应无法解析。                |
| `bad_response_body`         | 502       | 上游响应体格式异常。              |
| `empty_response`            | 502       | 上游返回空响应。                  |
| `prompt_blocked`            | 400       | 上游拒绝了该 Prompt（安全策略）。 |
| `aws_invoke_error`          | 502       | AWS Bedrock 调用失败。            |

## 内部错误

| code                    | HTTP 状态 | 含义                 |
| ----------------------- | --------- | -------------------- |
| `get_channel_failed`    | 500       | 获取渠道信息失败。   |
| `gen_relay_info_failed` | 500       | 生成中转上下文失败。 |
| `json_marshal_failed`   | 500       | JSON 序列化失败。    |
| `query_data_error`      | 500       | 数据库查询失败。     |
| `update_data_error`     | 500       | 数据库更新失败。     |

> 遇到 `5xx` 或内部错误时，请求**不会扣费**。详见 [计费规则](/docs/billing-rules)。

## 下一步

- 想了解计费、缓存折扣？见 [计费规则](/docs/billing-rules)。
- 想跑通第一个请求？见 [快速接入](/docs/quickstart)。

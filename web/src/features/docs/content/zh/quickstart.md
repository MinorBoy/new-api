# 快速接入

本页带你在 5 分钟内完成第一个 API 请求。

## 第一步：获取 API Key

1. 登录控制台，进入「令牌」页面。
2. 点击「添加令牌」，填写名称后保存。
3. 复制生成的令牌（形如 `sk-xxxxxxxx`），这就是你的 API Key。

> 令牌即 API Key，请妥善保管；如果泄露，请立即重置。

## 第二步：确定 Base URL

将官方上游地址替换为 new-api 的地址。对于本部署，Base URL 为：

```
https://<你的域名>/v1
```

- 调用 OpenAI 风格接口：`https://<你的域名>/v1`
- 调用 Anthropic 风格接口：`https://<你的域名>/v1`（同样以 `/v1` 为根，端点用 `/v1/messages`）

## 第三步：发送第一个请求

下面以 `gpt-4o-mini` 为例，用 cURL 发起聊天请求。相邻的代码块会显示为语言切换标签，可在 cURL 与 Python 之间切换：

```curl
curl https://<你的域名>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      { "role": "user", "content": "请用一句话介绍你自己。" }
    ]
  }'
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<你的域名>/v1",
    api_key="<YOUR_API_KEY>",
)

resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "请用一句话介绍你自己。"}],
)
print(resp.choices[0].message.content)
```

成功响应示例：

```json
{
  "choices": [
    {
      "message": { "role": "assistant", "content": "..." }
    }
  ],
  "usage": { "prompt_tokens": 12, "completion_tokens": 28, "total_tokens": 40 }
}
```

## 第四步：查看余额与用量

- 在「钱包」页面查看剩余额度。
- 在「使用日志」中查看每次请求的 token 消耗与计费明细。

## 常见问题

- **401 Unauthorized**：API Key 错误或已被吊销，请检查令牌。
- **404 model_not_found**：模型名拼写错误，或当前分组未开放该模型，前往「[模型与计费](/docs/pricing)」确认。
- **429 insufficient_user_quota**：额度不足，请充值。

更多错误码见[错误码参考](/docs/error-codes)。

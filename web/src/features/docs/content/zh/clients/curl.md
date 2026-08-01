# cURL

cURL 适合快速测试接口连通性，无需安装额外依赖。

## 对话补全

```curl
curl https://<你的域名>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <你的_API_KEY>" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      { "role": "system", "content": "你是一个简洁的助手" },
      { "role": "user", "content": "什么是 API 网关？" }
    ],
    "stream": false
  }'
```

## 流式输出

把 `stream` 设为 `true` 即可逐块接收响应：

```curl
curl https://<你的域名>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <你的_API_KEY>" \
  -N \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{ "role": "user", "content": "数到 5" }],
    "stream": true
  }'
```

`-N` 关闭输出缓冲，让你实时看到流式数据块。

## 文本向量化

```curl
curl https://<你的域名>/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <你的_API_KEY>" \
  -d '{
    "model": "text-embedding-3-small",
    "input": "需要向量化的文本"
  }'
```

## Anthropic 风格（Claude）

调用 Claude 系列模型时，使用 `/v1/messages` 端点，鉴权方式不变：

```curl
curl https://<你的域名>/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <你的_API_KEY>" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [
      { "role": "user", "content": "用一句话解释递归" }
    ]
  }'
```

## 下一步

- 更完整的端点列表见 [接口地址](/docs/endpoints)。
- 偏好 Python？见 [Python](/docs/clients/python) 教程。

# Python

使用官方 `openai` SDK 即可接入，只需修改 `base_url` 与 `api_key`。

## 安装

```bash
pip install openai
```

## 对话补全

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<你的域名>/v1",
    api_key="<你的_API_KEY>",
)

resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[
        {"role": "system", "content": "你是一个简洁的助手"},
        {"role": "user", "content": "什么是 API 网关？"},
    ],
)
print(resp.choices[0].message.content)
```

## 流式输出

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<你的域名>/v1",
    api_key="<你的_API_KEY>",
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "数到 5"}],
    stream=True,
)
for chunk in stream:
    delta = chunk.choices[0].delta.content
    if delta:
        print(delta, end="", flush=True)
```

## 文本向量化

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<你的域名>/v1",
    api_key="<你的_API_KEY>",
)

resp = client.embeddings.create(
    model="text-embedding-3-small",
    input="需要向量化的文本",
)
print(resp.data[0].embedding[:8])
```

## 使用环境变量

推荐将密钥放在环境变量中，避免硬编码：

```bash
export OPENAI_API_KEY="<你的_API_KEY>"
export OPENAI_BASE_URL="https://<你的域名>/v1"
```

```python
from openai import OpenAI

# 不传参时，SDK 会自动读取上面的环境变量
client = OpenAI()
```

## Anthropic 风格（Claude）

若使用官方 `anthropic` SDK，将 `base_url` 指向 new-api 即可：

```bash
pip install anthropic
```

```python
import anthropic

client = anthropic.Anthropic(
    base_url="https://<你的域名>",
    api_key="<你的_API_KEY>",
)

message = client.messages.create(
    model="claude-3-5-sonnet-20241022",
    max_tokens=1024,
    messages=[{"role": "user", "content": "用一句话解释递归"}],
)
print(message.content[0].text)
```

## 下一步

- 完整端点列表见 [接口地址](/docs/endpoints)。
- 偏好命令行？见 [cURL](/docs/clients/curl) 教程。

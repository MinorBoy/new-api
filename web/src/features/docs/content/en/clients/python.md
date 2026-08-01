# Python

Use the official `openai` SDK — just change `base_url` and `api_key`.

## Install

```bash
pip install openai
```

## Chat completion

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<your-domain>/v1",
    api_key="<YOUR_API_KEY>",
)

resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[
        {"role": "system", "content": "You are a concise assistant."},
        {"role": "user", "content": "What is an API gateway?"},
    ],
)
print(resp.choices[0].message.content)
```

## Streaming

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<your-domain>/v1",
    api_key="<YOUR_API_KEY>",
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Count to 5."}],
    stream=True,
)
for chunk in stream:
    delta = chunk.choices[0].delta.content
    if delta:
        print(delta, end="", flush=True)
```

## Embeddings

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<your-domain>/v1",
    api_key="<YOUR_API_KEY>",
)

resp = client.embeddings.create(
    model="text-embedding-3-small",
    input="Text to embed.",
)
print(resp.data[0].embedding[:8])
```

## Using environment variables

Prefer environment variables over hardcoding keys:

```bash
export OPENAI_API_KEY="<YOUR_API_KEY>"
export OPENAI_BASE_URL="https://<your-domain>/v1"
```

```python
from openai import OpenAI

# When called with no arguments, the SDK reads the env vars above.
client = OpenAI()
```

## Anthropic style (Claude)

If you use the official `anthropic` SDK, point `base_url` at new-api:

```bash
pip install anthropic
```

```python
import anthropic

client = anthropic.Anthropic(
    base_url="https://<your-domain>",
    api_key="<YOUR_API_KEY>",
)

message = client.messages.create(
    model="claude-3-5-sonnet-20241022",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Explain recursion in one sentence."}],
)
print(message.content[0].text)
```

## Next steps

- Full endpoint list: [Endpoints](/docs/endpoints).
- Prefer the command line? See the [cURL](/docs/clients/curl) guide.

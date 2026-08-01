# Quickstart

This page walks you through your first API request. It takes about 5 minutes.

## Step 1: Get your API key

1. Log in to the console and open the **Tokens** page.
2. Click **Add Token**, enter a name, and save.
3. Copy the generated token (it looks like `sk-xxxxxxxx`) — that is your API key.

> A token is your API key. Keep it secret; reset it immediately if it leaks.

## Step 2: Determine the Base URL

Replace the official upstream URL with the new-api URL. For this deployment, the Base URL is:

```
https://<your-domain>/v1
```

- OpenAI-style endpoints: `https://<your-domain>/v1`
- Anthropic-style endpoints: also rooted at `/v1`, using the `/v1/messages` path

## Step 3: Send your first request

Here is a chat completion with `gpt-4o-mini` using cURL. Adjacent fenced blocks collapse into a language switcher — click to toggle between cURL and Python:

```curl
curl https://<your-domain>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      { "role": "user", "content": "Introduce yourself in one sentence." }
    ]
  }'
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<your-domain>/v1",
    api_key="<YOUR_API_KEY>",
)

resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Introduce yourself in one sentence."}],
)
print(resp.choices[0].message.content)
```

A successful response looks like:

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

## Step 4: Check balance and usage

- View remaining quota on the **Wallet** page.
- Inspect token consumption and billing details per request under **Usage Logs**.

## Troubleshooting

- **401 Unauthorized**: the API key is wrong or revoked. Check your token.
- **404 model_not_found**: the model name is misspelled, or your group doesn't have access. Confirm on [Models & Pricing](/docs/pricing).
- **429 insufficient_user_quota**: out of quota — top up your balance.

See [Error Codes](/docs/error-codes) for the full reference.

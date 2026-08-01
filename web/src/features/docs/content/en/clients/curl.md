# cURL

cURL is ideal for quickly testing endpoint connectivity with no extra dependencies.

## Chat completion

```curl
curl https://<your-domain>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      { "role": "system", "content": "You are a concise assistant." },
      { "role": "user", "content": "What is an API gateway?" }
    ],
    "stream": false
  }'
```

## Streaming

Set `stream` to `true` to receive the response in chunks:

```curl
curl https://<your-domain>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -N \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{ "role": "user", "content": "Count to 5." }],
    "stream": true
  }'
```

`-N` disables output buffering so you see stream chunks in real time.

## Embeddings

```curl
curl https://<your-domain>/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -d '{
    "model": "text-embedding-3-small",
    "input": "Text to embed."
  }'
```

## Anthropic style (Claude)

For Claude models, use the `/v1/messages` endpoint with the same auth scheme:

```curl
curl https://<your-domain>/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [
      { "role": "user", "content": "Explain recursion in one sentence." }
    ]
  }'
```

## Next steps

- For the full endpoint list, see [Endpoints](/docs/endpoints).
- Prefer Python? See the [Python](/docs/clients/python) guide.

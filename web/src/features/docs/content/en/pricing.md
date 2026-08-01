# Models & Pricing

new-api supports hundreds of models across 40+ upstream providers. For the complete, **live** model list, group availability, and prices, visit the [Model Square](/pricing).

## Billing overview

Each model is billed along one of these dimensions — exact ratios are shown live in the Model Square:

- **Token-based billing**: input (prompt) tokens and output (completion) tokens are priced separately.
- **Per-request billing**: some image, video, and audio models charge a fixed price per request.
- **Group ratios**: different user groups can apply different discount ratios. Switch the group at the top of the Model Square to see the effect.

## Cache billing

Some upstreams support **prompt caching** (e.g. OpenAI GPT-5 family cache reads, Anthropic prompt caching, Gemini context caching). Hitting the cache can dramatically cut the cost of repeated context.

### How three token types are billed

The input tokens of a request are split into three parts, each billed at a different ratio:

| Token type | Description | Billing ratio |
|---|---|---|
| **Cache read** (cached_tokens) | Input tokens served from the upstream cache, e.g. repeated system prompts or long document prefixes | **Cache-read ratio** (cache_ratio), typically far below the input ratio (e.g. 0.5) |
| **Cache creation** | Tokens additionally billed when first writing the cache (supported by some upstreams, e.g. Anthropic) | **Cache-creation ratio** (create_cache_ratio), typically slightly above the input ratio (e.g. 1.25) |
| **Uncached input** | Regular input tokens that did not hit the cache | Normal **input ratio** |

> Ratio meaning: at a 0.5 cache-read ratio, cached tokens are billed at 50% of the normal input price.

### Worked example of a cache hit

Take `gpt-5.6-sol`. Suppose a request has:

- 10000 input tokens total, of which **8000 hit the cache** (repeated system prompt + conversation-history prefix)
- 500 output tokens
- Cache-read ratio 0.5, input ratio 1.0, output ratio 4.0

```
cost = 8000 × 0.5  (cache read)
     + 2000 × 1.0  (uncached input)
     + 500  × 4.0  (output)
     = 8000
```

Without caching, all 10000 input tokens bill at the input ratio = 10000, so **caching saves 20%**. The longer and more repetitive the context, the greater the saving.

### How to check cache ratios

Each model's cache ratios are shown in its card detail on the [Model Square](/pricing). Expanding a model card shows:

- Whether caching is supported (presence of a `cache_ratio` field)
- The cache-read ratio (`cache_ratio`)
- The cache-creation ratio (`create_cache_ratio`, only some models)

Models with no cache ratio do not support prompt caching; all input tokens bill at the normal input ratio.

### Enabling caching

Callers usually **need to do nothing extra** — as long as the upstream model supports caching, repeated prefix context hits the cache automatically. Recommendations:

- Put **fixed content** (system prompts, tool definitions, long documents) at the **start** of the `messages` array, with changing content after, to maximize the cache-hit rate.
- For Anthropic models, explicitly mark blocks to cache with `cache_control` in the request.

See [Billing Rules](/docs/billing-rules) for pre-deduction, settlement, and the no-charge-on-failure policy.

## How to read prices

1. Open the [Model Square](/pricing).
2. Switch to your user group at the top (e.g. the default group).
3. Each model card shows:
   - Input price (per million tokens)
   - Output price (per million tokens)
   - Context window and capability tags
   - Supported endpoints

Click any card to expand detailed billing, code samples, and supported parameters.

## Billing units

- For token-billed models, quota is `input tokens × input ratio + output tokens × output ratio`.
- Prices are in CNY; 1 CNY = 500,000 quota units (subject to system configuration).

## Groups and availability

Not every model is available to every group. If a model shows as unavailable in the Model Square, it may be because:

- No upstream channel for that model is configured for your group.
- Your account's group doesn't have access to that model.

Contact an admin to adjust group configuration. See [Billing Rules](/docs/billing-rules) for billing, cache discounts, and the no-charge-on-failure policy.

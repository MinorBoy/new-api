# Billing Rules

This page explains how new-api bills requests, applies cache discounts, and handles failures.

## Billing formula

Chat-style models are billed by token. The total consumption is:

```
cost = inputTokens × inputRatio + outputTokens × outputRatio
```

- **Input (prompt) and output (completion) tokens are priced separately**; output is usually more expensive.
- Ratios are shown live in the [Model Square](/pricing).

Models marked "per-request" charge a fixed fee per request regardless of tokens.

## Pre-deduction and settlement

To prevent quota overdraft, every request uses **two-phase billing**:

1. **Pre-deduction**: before the request starts, quota is frozen based on the estimated maximum cost. If the quota is too low, the request fails immediately with `pre_consume_token_quota_failed` / `insufficient_user_quota`.
2. **Settlement**: after the request completes, the actual cost is settled — refunds for overestimates, additional charges for underestimates.

## No charge on failure

Requests are **not charged** when:

- They fail before reaching the upstream (auth failure, model not found, etc.).
- The upstream returns a non-2xx status (`bad_response_status_code`, `do_request_failed`, etc.).
- An internal `5xx` error occurs.

Pre-deducted quota is fully refunded during settlement. Only requests that succeed and return a valid response are charged.

## Cache discounts

Some upstreams support **prompt caching** (e.g. Anthropic prompt caching):

- Input tokens that hit the cache enjoy a lower cache ratio.
- Creating the cache (first write) may incur an additional cache-creation fee.
- See each model's detail page in the [Model Square](/pricing) for exact ratios.

Higher cache hit rates mean lower per-request costs. Enable caching for requests with large fixed context (system prompts, long documents).

## Group ratios

Admins can configure different discount ratios per user group. When you switch groups in the [Model Square](/pricing), prices change accordingly.

## Rate limits

Each API key is subject by default to (exact thresholds are admin-configured and visible on the **Tokens** edit page):

- Number of requests per time window.
- Number of successful requests per time window.
- Total tokens per time window.

Exceeding a limit returns `429` with a message identifying the limit type. Note the difference: insufficient quota returns `insufficient_user_quota`, while a rate limit returns a dedicated rate-limit message.

## Next steps

- Billing questions? Match against [Error Codes](/docs/error-codes).
- Want lower costs? Compare models in [Models & Pricing](/docs/pricing).

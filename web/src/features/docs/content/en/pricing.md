# Models & Pricing

new-api supports hundreds of models across 40+ upstream providers. For the complete, **live** model list, group availability, and prices, visit the [Model Square](/pricing).

## Public Seedance model IDs

End users can see and call only these three official Doubao Seedance model IDs:

- `doubao-seedance-2-0-260128`
- `doubao-seedance-2-0-fast-260128`
- `doubao-seedance-2-0-mini-260615`

This restriction applies only to the Seedance family. Other integrated models, including GPT, GPT Image, Claude, Gemini, DeepSeek, and GLM, remain available according to the active group configuration. Internal Seedance channel models and routing targets are not exposed. Calling any other Seedance model ID returns `model_not_found`.

## Billing overview

Each model is billed along one of these dimensions — exact ratios are shown live in the Model Square:

- **Token-based billing**: input (prompt) tokens and output (completion) tokens are priced separately.
- **Per-request billing**: some image, video, and audio models charge a fixed price per request.
- **Group ratios**: different user groups can apply different discount ratios. Switch the group at the top of the Model Square to see the effect.

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

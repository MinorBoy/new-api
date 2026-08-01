# Overview

new-api is an AI API gateway that aggregates 40+ upstream AI providers (OpenAI, Anthropic, Google, Azure, AWS Bedrock, and more) behind a unified interface, with user management, billing, rate limiting, and an admin dashboard.

## Dual-protocol access

new-api supports the two dominant API styles simultaneously, so **your code requires no changes** to integrate:

- **OpenAI style**: `/v1/chat/completions`, `/v1/embeddings`, `/v1/images/generations`, etc. — works with most clients.
- **Anthropic style**: `/v1/messages` — for the Claude family of models and the native Claude SDK.

Simply replace the official upstream URL with the new-api Base URL. Request formats, parameters, and response shapes are identical to the official APIs.

## Get started in four steps

1. **Register**: sign up and log in to the console.
2. **Create a token**: create an API Key on the **Tokens** page.
3. **Choose a protocol**: decide whether your model uses the OpenAI or Anthropic style endpoint.
4. **Configure your client**: paste the Base URL and API Key into your client and start calling.

## Next steps

- Walk through the [Quickstart](/docs/quickstart) to make your first request.
- Browse the full endpoint list under [Endpoints](/docs/endpoints).
- Check [Models & Pricing](/docs/pricing) for available models and prices.

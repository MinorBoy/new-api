# Error Codes

When a request fails, the response body follows the OpenAI error shape:

```json
{
  "error": {
    "message": "human-readable detail",
    "type": "new_api_error",
    "code": "insufficient_user_quota"
  }
}
```

The `code` field is a machine-readable identifier; the table below lists the common values. The `type` field identifies the error origin (`new_api_error` / `openai_error` / `claude_error` / `upstream_error`, etc.).

## Request errors

| code                       | HTTP | Meaning / how to fix                                                           |
| -------------------------- | ---- | ------------------------------------------------------------------------------ |
| `invalid_request`          | 400  | Invalid request parameters — check the request body.                           |
| `bad_request_body`         | 400  | The request body could not be parsed (malformed JSON).                         |
| `read_request_body_failed` | 400  | Failed to read the request body — usually a network drop or oversized payload. |
| `convert_request_failed`   | 400  | Request format conversion failed — make sure the endpoint and protocol match.  |
| `sensitive_words_detected` | 400  | Triggered a sensitive-word filter.                                             |
| `access_denied`            | 403  | No permission to access this resource or endpoint.                             |

## Model and routing errors

| code                             | HTTP | Meaning / how to fix                                                          |
| -------------------------------- | ---- | ----------------------------------------------------------------------------- |
| `model_not_found`                | 404  | The model name is misspelled, or your group doesn't have access.              |
| `no_compatible_route`            | 404  | No channel can route this request — contact an admin or check channel config. |
| `compatible_channel_unavailable` | 503  | The compatible channel is temporarily unavailable; retry shortly.             |
| `invalid_api_type`               | 400  | The channel's API type is invalid.                                            |
| `routing_policy_error`           | 400  | A routing policy check failed.                                                |

## Billing and quota errors

| code                             | HTTP | Meaning / how to fix                                              |
| -------------------------------- | ---- | ----------------------------------------------------------------- |
| `insufficient_user_quota`        | 429  | Out of quota — top up your balance.                               |
| `pre_consume_token_quota_failed` | 429  | Pre-deduction failed (quota too low to cover the estimated cost). |
| `model_price_error`              | 500  | Model price is misconfigured — contact an admin.                  |
| `count_token_failed`             | 500  | Token counting failed.                                            |

## Channel errors (`channel:*`)

Channel errors occur during the upstream call phase:

| code                              | HTTP | Meaning                                    |
| --------------------------------- | ---- | ------------------------------------------ |
| `channel:no_available_key`        | 503  | The channel has no available upstream key. |
| `channel:invalid_key`             | 401  | The upstream key is invalid or expired.    |
| `channel:model_mapped_error`      | 400  | The model mapping config is wrong.         |
| `channel:aws_client_error`        | 400  | AWS Bedrock client error.                  |
| `channel:param_override_invalid`  | 400  | The channel parameter override is invalid. |
| `channel:header_override_invalid` | 400  | The channel header override is invalid.    |
| `channel:response_time_exceeded`  | 504  | The upstream response timed out.           |

## Upstream response errors

| code                        | HTTP | Meaning                                              |
| --------------------------- | ---- | ---------------------------------------------------- |
| `do_request_failed`         | 502  | Failed to send the request upstream (network layer). |
| `read_response_body_failed` | 502  | Failed to read the upstream response body.           |
| `bad_response_status_code`  | 502  | Upstream returned a non-2xx status.                  |
| `bad_response`              | 502  | The upstream response could not be parsed.           |
| `bad_response_body`         | 502  | The upstream response body is malformed.             |
| `empty_response`            | 502  | Upstream returned an empty response.                 |
| `prompt_blocked`            | 400  | Upstream rejected the prompt (safety policy).        |
| `aws_invoke_error`          | 502  | AWS Bedrock invocation failed.                       |

## Internal errors

| code                    | HTTP | Meaning                             |
| ----------------------- | ---- | ----------------------------------- |
| `get_channel_failed`    | 500  | Failed to load channel information. |
| `gen_relay_info_failed` | 500  | Failed to build the relay context.  |
| `json_marshal_failed`   | 500  | JSON serialization failed.          |
| `query_data_error`      | 500  | A database query failed.            |
| `update_data_error`     | 500  | A database update failed.           |

> On any `5xx` or internal error, the request is **not charged**. See [Billing Rules](/docs/billing-rules).

## Next steps

- Billing questions? See [Billing Rules](/docs/billing-rules).
- Getting started? See [Quickstart](/docs/quickstart).

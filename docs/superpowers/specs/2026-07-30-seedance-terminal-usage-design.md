# Seedance Terminal Usage Design

## Goal

Every successful Seedance task returned through the Ark task API includes the official public usage shape:

```json
{
  "usage": {
    "completion_tokens": 108900,
    "total_tokens": 108900
  }
}
```

Valid upstream usage remains authoritative. When an upstream billed per request or per duration omits usage, or returns an unusable all-zero usage, new-api computes the public usage locally from the Seedance video-token formula.

## Approaches Considered

1. Mutate the stored upstream polling payload. This would make every later projection see usage, but it would make the administrator's upstream-response audit no longer represent the real upstream payload.
2. Compute usage independently on every task query. This preserves the upstream audit, but reference-video metadata would need to be fetched repeatedly and old URLs could expire.
3. Persist only the aggregate reference-video duration at submission and inject calculated usage into the public projection. This preserves audit fidelity, avoids retaining new media details, and makes task reads deterministic.

Approach 3 is selected.

## Public Contract

- Preserve a valid upstream `usage.completion_tokens` and `usage.total_tokens` exactly.
- For a successful task without valid usage, return:
  - `completion_tokens`: output-video tokens.
  - `total_tokens`: input-reference-video tokens plus output-video tokens.
- Do not add `input_tokens`, `output_tokens`, or other non-Ark fields.
- Failed, queued, and running responses keep their existing response shape.
- The same projection is used by single-task and task-list responses.

## Formula

The formula is:

```text
tokens = duration_seconds * output_width * output_height * output_fps / 1024
```

Input reference videos use their aggregate duration. Output uses the actual duration, resolution, and frame rate returned by the provider when present, then falls back to the validated request snapshot and the shared Seedance resolution profile.

Input and output meters are ceil-rounded independently. `total_tokens` is the ceiling of the exact combined value, matching the existing profit-routing implementation and avoiding floating-point drift. Existing token bounds remain enforced.

## Data Flow

1. The routing parser supplies reference-video URLs and count.
2. Before upstream dispatch, the existing video metadata client resolves and aggregates their durations once. Requests without reference videos do not call the metadata service.
3. Only the aggregate duration in milliseconds is copied into `TaskBillingContext`; source URLs and media metadata are not persisted by this feature.
4. On a successful Ark task projection, the response builder first validates upstream usage.
5. If usage is absent or unusable, the response builder calculates local usage from the persisted input duration plus output facts and injects the official two-field object.
6. The original upstream polling body and administrator audit response remain unchanged.

## Error Handling

- A reference-video request whose duration cannot be resolved is rejected before upstream dispatch, because silently treating its input duration as zero would violate the mandatory usage contract and under-report tokens.
- Unsupported resolution, invalid frame rate, invalid duration, negative values, or token overflow do not produce fabricated usage.
- Existing successful historical tasks without a persisted reference duration can still receive output-only calculated usage when they contain no reference video. Historical tasks known to contain reference video but lacking its duration retain their upstream usage; the system does not guess.

## Testing

- Unit tests cover output-only and reference-video calculations, exact rounding, and bounds.
- Response tests cover preserving valid upstream usage and filling missing/all-zero usage with the official shape.
- Submission tests cover persisting only aggregate input duration and failing before dispatch when reference metadata is unavailable.
- Existing task query, polling, billing, cost-accounting, and privacy tests must continue to pass.

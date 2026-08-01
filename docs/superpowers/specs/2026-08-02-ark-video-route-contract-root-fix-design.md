# ARK Video Route Contract Root Fix Design

## Goal

Prevent imported Seedance route declarations from selecting an upstream task adaptor that will reject the same request under its verified provider protocol. Keep provider behavior fail-closed: capabilities without protocol evidence remain unavailable instead of being implemented by guessing request fields.

## Root Causes

1. Imported `reference_limits` are independent maxima, but the material-matrix E2E submitted every maximum at once. Values such as `933` therefore became 15 simultaneous media inputs even where the provider contract has a lower combined limit.
2. Routing matches only resolution, duration, input mode, independent media counts, and real-person support. It cannot represent combined media limits or provider-specific model and group rules.
3. Configuration import validates schema and routing shape, but does not compare a route target with the selected channel adaptor's verified contract.
4. Several imported rows conflict with verified contracts:
   - Cangyuan and Paipu are text-only until real multimodal protocol evidence exists.
   - CLMM does not accept audio or 1080p and its imported model names do not satisfy the verified CLMM model contract.
   - Dimensio imports unsupported direct model names and 480p/4K targets.
   - Secure groups have different input modes, minimum media, combined limits, duration ranges, and model/resolution matrices.
   - 4stoken is encoded as OpenAI type `1` in the legacy source despite having a dedicated type `209`.
   - 8yes has unresolved material limits and no verified task adaptor or cost rules.
5. Expected-rejection branches in the E2E allowed incompatible route declarations to look accepted at the suite level.

## Authority And Safety Boundary

The verified provider adaptor contract is authoritative for runtime behavior. Imported configuration may narrow that contract, but may not widen it. Missing protocol evidence is treated as unsupported.

This change does not add guessed Cangyuan, Paipu, CLMM, Dimensio, Secure, or 8yes request fields. Upstream HTTP remains mocked in E2E, while route selection, adaptor conversion, task persistence, billing, logging, and cost accounting use production code.

## Architecture

### Static Channel Route Contract

Add a small provider-neutral route contract in `types` and a lookup callback in `service`, following the existing cost-capability lookup pattern. Relay owns channel-specific protocol knowledge and registers the lookup implementation during startup and tests.

The contract validates a concrete route target against:

- channel type and channel settings;
- mapped upstream model;
- output resolutions and duration bounds;
- declared input modes and reference minima/maxima;
- provider-specific combined material constraints.

Validation returns stable issue codes and messages suitable for configuration import and routing policy APIs. A missing contract for ordinary channels preserves current behavior. Dedicated Seedance task channels must return an explicit contract.

### Publish And Policy Gates

`SaveRoutingPolicy` validates every target through the registered contract before persistence. Config import publishing uses the same path-independent validator while creating route rows. Incompatible targets remain disabled and publishing reports a deterministic conflict instead of creating a route that fails only after client submission.

Legacy normalization maps `CH-4STOKEN` from type `1` to dedicated type `209` at the config-import boundary. No equivalent guess is made for 8yes; its unresolved rows remain non-publishable.

### Runtime Defense

Adaptor request validation remains in place as defense in depth. The route contract and adaptor tests are table-driven from the same documented rules so drift is detected. Runtime routing must return `no_compatible_route` for facts outside a target's declared and provider-verified intersection rather than selecting a known-incompatible channel.

### E2E Matrix Semantics

Each imported target receives a provider-valid representative request. Material codes are covered as declared boundaries, not automatically combined into one payload:

- text baseline;
- image maximum where supported;
- video maximum where supported;
- audio maximum where supported;
- documented legal combined boundary;
- one-over-boundary negative cases for shared constraints.

Targets that conflict statically with the verified channel protocol are asserted as blocked configuration findings. They do not produce tasks, usage logs, or cost attempts. Valid targets complete submit, polling, settlement, logs, and cost accounting.

## Provider Rules

- 4stoken: dedicated type `209`; current ARK content protocol and imported models remain available.
- Lucen: current ARK generation profile and mapped resolution checks remain available.
- MegaByAI: current 9/3/3 individual bounds and documented media-duration checks remain available.
- Cangyuan: text input only; media maxima must be zero.
- Paipu: text input only; mapped resolution suffix must agree with route resolution.
- CLMM: no audio, only 480p/720p, 5-15 seconds for ordinary models, maximum 9 images, 3 videos, and 12 combined; mapped model must satisfy the verified prefix/control grammar.
- Dimensio: only registered models, 720p generally, and 1080p only for `jimeng-video-seedance-2.0-vip`; maximum 9 images, 3 videos, 3 audios, and 12 combined.
- Secure discount: at least one image, no strict last-frame mode, video+audio maximum 3, 4-15 seconds, and model/resolution matrix enforced.
- Secure overseas: maximum 12 combined media, 4-15 seconds, and model/resolution matrix enforced. Reference-video total duration remains runtime metadata validation.
- Secure enterprise: `video-2.0-pro`, 720p, 5-15 seconds, and no strict last-frame mode.
- 8yes: blocked until channel type, material limits, upstream protocol, and cost rules are verified.

## Data And Compatibility

No database migration is required. Existing routing constraint JSON remains readable. Validation is applied when policies are created, updated, enabled, or imported. Existing persisted enabled routes are not silently rewritten; an audit reports incompatible rows so administrators can disable or repair them explicitly.

All database operations continue to support SQLite, MySQL, and PostgreSQL through GORM.

## Tests

1. Contract unit tests cover every affected channel and exact issue codes.
2. Routing policy tests prove incompatible targets fail before persistence.
3. Config import tests prove legacy 4stoken normalization and incompatible target blocking.
4. Adaptor parity tests prove accepted contract representatives pass adaptor validation.
5. Material matrix E2E removes expected provider rejection branches, records blocked configuration separately, and completes all valid targets through task, usage, quota, and cost tables.
6. Persistent MySQL seed is rerun and the report records valid successes, blocked targets, material coverage, costs, and zero placeholder prices.

## Acceptance

- No route target known to violate a verified channel protocol is enabled.
- No E2E success count includes an expected adaptor rejection.
- No unsupported multimodal behavior is fabricated.
- Every successful task has one settled cost attempt using the imported CNY rule and selected variant.
- Provider HTTP is the only mocked boundary.
- Focused tests, the full Seedance E2E set, `git diff --check`, and relevant Go package tests pass before commit.

# ysr Branch Rules

## Upstream Integration

- By default, do not merge `origin/main` or local `main` into `ysr`.
- An upstream merge into `ysr` is permitted only when the repository owner explicitly requests that merge in the current task.
- Do not infer permission to merge from new upstream commits, a request to continue work, or completion of another task.
- An approved merge must leave local `main` unchanged unless the repository owner explicitly requests otherwise.

## Channel Type IDs

- ysr-specific channel types must use the reserved range `200-299`.
- Do not allocate new ysr channel types in upstream or legacy ranges.
- Any change to an existing ysr channel type ID must include a transactional data migration for both `channels.type` and persisted task platform values, plus regression coverage.

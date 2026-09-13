# OpenAI Image 2.5 Model Matrix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `gpt-image-2.5-flare` 与 `gpt-image-2.5-sunburst` 在现有 OpenAI Images 九组合能力、成本、售价、路由和模型广场链路中可独立配置，同时保持 `gpt-image-2` 兼容。

**Architecture:** 继续使用现有 `ImageModelCatalog` 的模型键隔离配置。启动迁移从已配置的 `gpt-image-2` 深拷贝两个新模型的完整 generations 目录条目；前端和成本同步按目录模型动态遍历，不引入新的协议或 SKU 维度。克隆仅复制能力与已有售价，不创建任何供应商成本规则；管理员可随后独立修改新模型售价和渠道成本。

**Tech Stack:** Go 1.22, Gin/GORM, React 19, TypeScript, Bun, Vitest/Playwright。

---

### Task 1: Add regression coverage for the two Image 2.5 models

**Files:**
- Modify: `setting/image_setting/catalog_test.go`
- Modify: `service/image_catalog_migration_test.go`

- [ ] **Step 1: Write failing catalog test**

Add a table-driven test for `EnsureOpenAIImage25Models` that requires each clone to have `openai_images` v1, the same enabled generations matrix and all nine `gen-<tier>-<quality>` SKU keys, while preserving independent maps.

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `go test ./setting/image_setting -run TestImage25ModelsHaveFullMatrix -count=1`

Expected: FAIL because `EnsureOpenAIImage25Models` is not yet defined.

- [ ] **Step 3: Add the minimal shared fixture builder in test code**

Build a `gpt-image-2` source entry in the test and assert the two model names are added without overwriting a pre-existing entry.

- [ ] **Step 4: Run the focused test again**

Run: `go test ./setting/image_setting -run TestImage25ModelsHaveFullMatrix -count=1`

Expected: PASS, proving the clone helper creates isolated model entries.

- [ ] **Step 5: Commit the regression fixtures**

```bash
git add setting/image_setting/catalog_test.go service/image_catalog_migration_test.go
git commit -m "test: cover image 2.5 model matrices"
```

### Task 2: Make image pricing and cost synchronization explicitly model-complete

**Files:**
- Modify: `setting/image_setting/catalog.go`
- Modify: `service/image_catalog_migration.go`
- Modify: `common/model.go`
- Test: `setting/image_setting/catalog_test.go`
- Test: `service/image_catalog_migration_test.go`
- Test: `common/model_test.go`

- [ ] **Step 1: Add failing startup migration test**

Extend startup migration coverage to require both new models after a catalog containing `gpt-image-2`, and require that no extra cost rules are created beyond the source legacy-rule migration.

- [ ] **Step 2: Run the focused tests and verify the model isolation failure**

Run: `go test ./service -run TestMigrateImageCatalogAtStartupCopiesCostsAndIsIdempotent -count=1`.

Expected: the new assertions fail until startup migration persists the two model entries.

- [ ] **Step 3: Implement only the missing model-aware behavior**

Implement `EnsureOpenAIImage25Models` with a deep model clone and call it from `MigrateImageCatalogAtStartup`; persist the changed catalog in the existing transaction. Add both names to `common.ImageGenerationModels` so endpoint inference exposes `image-generation`. Do not create cost rules for the cloned models and do not overwrite administrator-supplied entries.

- [ ] **Step 4: Run the focused backend tests**

Run: `go test ./setting/image_setting ./service ./common -run 'Test(EnsureOpenAIImage25Models|MigrateImageCatalogAtStartup|TestImage25ModelsUseImageGenerationEndpoint)' -count=1`.

Expected: PASS with isolated model entries and unchanged legacy costs.

- [ ] **Step 5: Commit the backend model isolation changes**

```bash
git add setting/image_setting/catalog.go service/image_catalog_migration.go setting/image_setting/catalog_test.go service/image_catalog_migration_test.go
git commit -m "feat: add image 2.5 catalog models"
```

### Task 3: Update channel matrix and pricing workbench model handling

**Files:**
- Test: `web/src/features/system-settings/models/image-pricing-catalog.test.ts`
- Test: `web/src/features/channels/lib/channel-form.test.ts`

- [ ] **Step 1: Add failing UI utility tests**

Require existing dynamic helpers to preserve independent overrides for all three model names, including an explicit empty matrix, and require `flattenImagePricingCatalog` to produce 27 rows for three models with model-qualified IDs.

- [ ] **Step 2: Run the focused frontend tests and verify failure**

Run from `web`: `bun test src/features/channels/lib/channel-form.test.ts src/features/system-settings/models/image-pricing-catalog.test.ts`

Expected: FAIL only if a regression has introduced a single-model assumption; otherwise record the existing dynamic behavior and keep production code unchanged.

- [ ] **Step 3: Implement dynamic model-aware UI behavior**

Keep the existing model-derived rendering and serialization. Add only the smallest fix exposed by the focused tests; no new hard-coded model list is needed because the channel form already iterates `currentModelsArray` and the pricing workbench already iterates catalog models.

- [ ] **Step 4: Run focused frontend tests and typecheck**

Run from `web`: `bun test src/features/channels/lib/channel-form.test.ts src/features/system-settings/models/image-pricing-catalog.test.ts`; then `bun run typecheck`.

Expected: all focused tests pass and typecheck exits 0.

- [ ] **Step 5: Commit the frontend changes**

No production commit is needed when the focused tests confirm the existing dynamic UI behavior.

### Task 4: Add end-to-end coverage and verify the complete compatibility surface

**Files:**
- Modify: `web/e2e/image-pricing-workbench.pw.ts`
- Modify: `docs/api/image-generation.md`

- [ ] **Step 1: Add failing E2E assertions**

Extend the pricing workbench flow to assert both Image 2.5 model names, model-qualified SKU rows, and independent sale-price edits. Use the existing channel drawer coverage or manual browser acceptance for matrix save/reload because no dedicated image channel E2E file exists.

- [ ] **Step 2: Run the E2E tests and verify the expected failure**

Run from `web`: `bunx playwright test e2e/image-pricing-workbench.pw.ts --reporter=line`

Expected: FAIL before the UI/data changes are wired into the running application.

- [ ] **Step 3: Update the API documentation**

Document that `model` selects one of the configured catalog models, that `size` is classified by total pixels into 1K/2K/4K, and that `size` omitted/`auto` bills as 1K. State that the three models have independent channel capability and price/cost configuration.

- [ ] **Step 4: Run the complete verification suite**

Run from repository root:

```bash
go test ./...
cd web
bun test
bun run typecheck
bunx oxlint .
bun run build
```

Expected: every command exits 0; record any pre-existing unrelated failures explicitly instead of masking them.

- [ ] **Step 5: Inspect the final diff and commit**

Run: `git diff --check`, `git status --short`, and `git diff HEAD~4..HEAD --stat`.

Commit documentation/E2E changes with:

```bash
git add web/e2e docs/api/image-generation.md
git commit -m "test: verify image 2.5 matrix routing and pricing"
```

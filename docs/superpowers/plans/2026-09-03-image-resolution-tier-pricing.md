# 图像分辨率档位计费实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 OpenAI Images 的售价、渠道能力和供应商成本从具体宽高 SKU 统一为 `1k/2k/4k × low/medium/high` 档位，并按请求总像素自动路由。

**Architecture:** 在 `setting/image_setting` 增加有界的总像素分档解析，目录以档位 SKU 保存售价；`service` 和 `middleware` 使用解析后的稳定档位键执行能力与成本路由，原始 `size` 仍由适配器透传。管理端提供九宫格编辑器和渠道能力矩阵，保留旧 SKU 兼容读取及显式迁移冲突报告。

**Tech Stack:** Go 1.22、Gin、GORM、Testify、React 19、TypeScript、React Testing Library、Vitest、Bun、Docker Compose。

---

## 文件与职责映射

- 修改 `setting/image_setting/catalog.go`：档位常量、档位 SKU 生成、size 解析、目录校验与旧键兼容读取。
- 修改 `setting/image_setting/catalog_test.go`：总像素边界、auto/default、旧键迁移和目录校验回归。
- 修改 `relaykit/dto/openai_image.go` 与 `relay/helper/valid_request.go`：移除仅针对统一图像请求的具体尺寸限制，保留非图像历史模型校验，并将 `auto` 归一化规则接入统一图像流程。
- 修改 `service/image_profile.go`、`service/image_routing.go`、`service/image_pricing.go`：按档位执行能力、成本变体、路由和汇总。
- 修改 `middleware/distributor.go`、`controller/image_routing_preview.go`：解析统一图像请求并在预览中返回档位 SKU。
- 新增/修改 `service/*image*_test.go`、`middleware/distributor_image_test.go`、`controller/image_pricing_test.go`：保护 API、路由、成本和迁移契约。
- 修改 `web/src/features/system-settings/models/image-pricing-catalog.ts`、`image-pricing-workbench.tsx`：九宫格售价编辑、档位说明、校验和保存。
- 修改 `web/src/features/channels/types.ts`、`web/src/features/channels/lib/channel-form.ts`、`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`：图像能力矩阵表单、成本输入和自动生成绑定。
- 新增对应 `__tests__/` 测试文件：前端九宫格交互与渠道矩阵行为。
- 修改 `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`：新增界面文案翻译。
- 新增 `docs/superpowers/specs/2026-09-03-image-resolution-tier-pricing-design.md` 已提交；实施完成后补充迁移/运维说明（简体中文）。

## Task 1: 建立总像素档位解析契约

**Files:**
- Modify: `setting/image_setting/catalog.go`
- Test: `setting/image_setting/catalog_test.go`

- [ ] **Step 1: Write failing tests**

新增表格测试覆盖：空值与 `auto` 返回 `1k`；`1024x1024`、`2048x2048`、`2880x2880` 命中边界；典型矩形按乘积归类；超过 4K、零值、负值、非数字、乘法溢出返回参数错误。

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./setting/image_setting -run 'Test.*Resolution|Test.*Resolve'`

Expected: FAIL because no total-pixel resolver or tier SKU exists.

- [ ] **Step 3: Implement minimal resolver and tier key**

实现 `ResolutionTier`、`ResolveResolutionTier(size string) (tier, normalizedSize, error)` 和 `BuildTierSKUKey(endpoint, tier, quality)`。使用 `math/bits` 或有界除法检查乘法溢出；`size==""`/`auto` 返回 `1k`；档位阈值为 `1048576`、`4194304`、`8294400`。保持旧 `BuildSKUKey` 仅供兼容读取。

- [ ] **Step 4: Make catalog validation and Resolve use tier SKUs**

给 `SKU` 增加 `Tier string `json:"tier"`` 字段并要求新目录使用 `gen-1k-low` 等稳定键；`Resolve` 对具体 size 调用解析器，返回 `ResolvedSKU.Tier` 和档位 SKU。验证每个启用端点的默认档位 SKU 存在，旧目录仍可读取。

- [ ] **Step 5: Run tests**

Run: `go test ./setting/image_setting -run 'Test.*Resolution|Test.*Resolve'`

Expected: PASS。

- [ ] **Step 6: Commit**

`git add setting/image_setting && git commit -m "feat: add image resolution tiers"`

## Task 2: 统一请求验证、计费和成本变体

**Files:**
- Modify: `relay/helper/valid_request.go`
- Modify: `relaykit/dto/openai_image.go`
- Modify: `service/image_profile.go`
- Modify: `service/image_routing.go`
- Modify: `service/image_pricing.go`
- Modify: `relay/helper/price.go`
- Test: `relay/helper/openai_image_request_test.go`, `service/image_model_routing_test.go`, `service/image_pricing_test.go`

- [ ] **Step 1: Add failing contract tests**

验证统一 `gpt-image-*` 请求的 `1500x1500` 命中 2K、`3000x2000` 命中 4K、缺省/`auto` 命中 1K；超过 4K 和非法尺寸在进入计费前返回 400；原始 size 仍保留给上游请求构造。验证 `gen-2k-high` 是成本变体和售价查找键。

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./relay/helper ./service -run 'Test.*Image|Test.*Routing|Test.*Pricing'`

Expected: FAIL with old exact-size SKU/size whitelist behavior。

- [ ] **Step 3: Implement normalized image request context**

在 `service.ImageRequestContext`/`ResolvedSKU` 中保存 `Tier`；统一图像路径调用档位解析。仅保留 `dall-e` 等非统一模型的历史固定尺寸校验。`size=auto` 在发送上游前按现有适配器默认处理，不把字符串写入需要具体尺寸的上游字段。

- [ ] **Step 4: Switch routing and pricing keys**

`EvaluateImageChannel`、`imageCompatibilityExclusion`、`BuildImageRouteDecision`、`ImageSKUPriceHelper`、`ListImagePricingCostSummary` 全部使用 `ResolvedSKU.SKUKey` 的档位键；成本规则查找保持 channel + mapped upstream model +档位键。缺能力/缺成本不降级。

- [ ] **Step 5: Preserve historical compatibility**

实现旧具体尺寸键到档位键的读取 fallback，仅用于历史目录和日志；新解析优先档位键。补充冲突检测 API/服务，金额不一致时返回可审计冲突而不覆盖。

- [ ] **Step 6: Run focused backend tests**

Run: `go test ./relay/helper ./service ./middleware ./controller -run 'Test.*Image|Test.*Routing|Test.*Pricing'`

Expected: PASS。

- [ ] **Step 7: Commit**

`git add relay service middleware controller setting && git commit -m "feat: route and bill images by resolution tier"`

## Task 3: 迁移工具与目录版本升级

**Files:**
- Modify: `setting/image_setting/catalog.go`
- Create: `service/image_catalog_migration.go`
- Test: `service/image_catalog_migration_test.go`

- [ ] **Step 1: Write migration tests**

覆盖旧键到档位键的三档映射、相同成本合并、不同成本生成冲突、超出 4K/无法解析报告异常、迁移后历史键可读取。

- [ ] **Step 2: Run migration tests and verify failure**

Run: `go test ./service -run 'TestImageCatalogMigration'`

Expected: FAIL because migration service is absent。

- [ ] **Step 3: Implement deterministic migration service**

新增 `ImageCatalogMigrationInput`、`ImageCatalogMigrationResult`、`ImageCatalogMigrationConflict` 和 `MigrateImageCatalog`，按 model、endpoint、quality、渠道、上游模型聚合旧 SKU；使用总像素解析器映射；一致成本只保留一条；冲突项包含渠道、上游模型、端点、目标档位、旧 SKU 和金额。不得自动覆盖不一致成本。

- [ ] **Step 4: Add admin-safe endpoint or scheduled migration hook**

遵循现有配置导入/审阅模式，提供预览、冲突列表和显式发布动作；迁移前后保留旧目录快照，失败不改变当前生效目录。

- [ ] **Step 5: Run tests and commit**

Run: `go test ./service -run 'TestImageCatalogMigration'`

`git add setting service && git commit -m "feat: migrate legacy image skus"`

## Task 4: 全局售价九宫格

**Files:**
- Modify: `web/src/features/system-settings/models/image-pricing-catalog.ts`
- Modify: `web/src/features/system-settings/models/image-pricing-workbench.tsx`
- Test: `web/src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`

- [ ] **Step 1: Write failing UI tests**

使用用户视角断言：每个模型/端点显示 1K、2K、4K 三行和 low/medium/high 三列；单元格直接编辑后保存；非法价格显示错误并阻止保存；页面不出现具体尺寸白名单输入。

- [ ] **Step 2: Run test and verify failure**

Run from `web`: `bunx vitest run src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`

Expected: FAIL because current workbench flattens concrete size SKUs。

- [ ] **Step 3: Implement tier catalog adapter**

将 `ImageCatalogSKU`/`ImagePricingRow` 改为 `tier` 字段，按端点和三档质量补齐九宫格行；保留旧目录读取时的迁移映射；售价更新只写新档位键。显示每档总像素上限与成本/毛利状态。

- [ ] **Step 4: Implement interaction and states**

支持单格直接编辑、保存、刷新、错误、未配置成本、冲突提示；通过 `useTranslation()` 添加所有文案。不得让用户编辑 JSON 作为常规入口。

- [ ] **Step 5: Run tests and checks**

Run: `bunx vitest run src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`; `bun run typecheck`; `bun run lint`。

Expected: PASS with no type/lint errors。

- [ ] **Step 6: Commit**

`git add web/src/features/system-settings/models web/src/i18n/locales && git commit -m "feat: simplify image pricing tier editor"`

## Task 5: 渠道能力矩阵与成本配置

**Files:**
- Modify: `web/src/features/channels/types.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Modify: `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- Create: `web/src/features/channels/components/__tests__/image-capability-matrix.test.tsx`
- Modify: `model/channel.go`
- Modify: `service/image_profile.go`

- [ ] **Step 1: Write failing UI tests**

断言选择图像模型后出现九宫格；按行全选勾选三种质量；勾选单格显示成本输入；缺成本阻止提交；非图像渠道不显示矩阵；协议绑定为空时自动生成默认 profile。

- [ ] **Step 2: Run test and verify failure**

Run: `bunx vitest run src/features/channels/components/__tests__/image-capability-matrix.test.tsx`

Expected: FAIL because channel editor currently exposes only JSON profile。

- [ ] **Step 3: Add typed form state**

定义 `ImageCapabilityMatrix`（endpoint、tier、quality、enabled、costUSD），从现有 channel settings 兼容读取；保存时生成档位能力覆盖和成本规则草稿，避免在主表单中拼接手写 JSON。

- [ ] **Step 4: Add matrix UI and validation**

在图像渠道区域渲染三行三列矩阵，提供行全选、单格勾选、成本输入、冲突/缺失成本提示；提交前验证每个启用格的成本是非负固定小数。

- [ ] **Step 5: Wire backend persistence**

复用现有 `POST /api/cost-accounting/rules`、`PUT /api/cost-accounting/rules/:id`、`POST /api/cost-accounting/rules/:id/validate` 和 `POST /api/cost-accounting/rules/:id/activate` API，按 `channel + mapped upstream model + gen/edit-tier-quality` 创建或更新草稿；`model/channel.go` 和 `service/image_profile.go` 校验生成的能力覆盖；协议绑定继续由系统默认值生成；不删除历史兼容字段，迁移成功后再停用旧具体尺寸变体。

- [ ] **Step 6: Run frontend checks and commit**

Run: `bunx vitest run src/features/channels/components/__tests__/image-capability-matrix.test.tsx`; `bun run typecheck`; `bun run lint`。

`git add web/src/features/channels web/src/i18n/locales && git commit -m "feat: configure image channel capability matrix"`

## Task 6: 全量验证与容器验收

**Files:**
- Modify only files required by failing tests or i18n fixes.
- Test: existing backend/frontend test suites and browser acceptance artifacts.

- [ ] **Step 1: Run backend focused suites**

Run: `go test ./setting/image_setting ./relay/helper ./service ./middleware ./controller`

- [ ] **Step 2: Run frontend verification**

Run from `web`: `bun run typecheck`; `bun run lint`; `bun run build`; `bunx vitest run src/features/system-settings/models src/features/channels/components`。

- [ ] **Step 3: Rebuild local containers**

Run: `docker compose -f docker-compose.local.yml up -d --build new-api`

Expected: new-api healthy and `/api/status` succeeds; do not restart unrelated MySQL/Redis containers。

- [ ] **Step 4: Browser acceptance**

登录本机管理端，验证全局九宫格售价编辑、渠道九宫格能力、缺成本阻止保存、空协议自动填充；使用测试 Key 发起缺省 `size`、`auto`、1K/2K/4K 边界和多渠道请求，核对响应、用量日志、用户售价、供应商成本、档位 SKU 和实际渠道。

- [ ] **Step 5: Review diff and commit acceptance notes**

Run: `git status --short`; `git diff --check`; `git log -5 --oneline`。保留用户原有未跟踪的 `web/test-results/`，不要提交或删除。

## Verification checklist

- [ ] 规格中的每一项都有对应任务。
- [ ] 任何 `size` 白名单配置均未引入；仅做格式、有界乘法和 4K 总像素校验。
- [ ] 缺省/`auto` 统一 1K；边界按总像素判定，4K 上限固定为 `2880 × 2880` 的乘积。
- [ ] 路由匹配键始终包含公共模型、端点、档位和 quality；没有隐式降级。
- [ ] 旧 SKU 冲突不会自动覆盖，历史记录仍可读。
- [ ] 计费路径使用集中化有界转换，不产生负费用。
- [ ] 后端、前端、容器和浏览器验收均有最新输出后才能声明完成。

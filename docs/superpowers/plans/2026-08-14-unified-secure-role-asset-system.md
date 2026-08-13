# 统一 Secure 角色素材系统实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 为下游用户提供统一的图片角色素材 API 和素材库，并将项目内 asset-* 安全转换为 Secure 企业 Video 的 asset-local-*。

**Architecture:** 新增 model.Asset 与 model.AssetProviderBinding 两个持久化模型；service.AssetService 负责用户归属、幂等、状态机和渠道绑定；service.AssetProviderAdapter 隔离 Secure API。素材视频请求在 TokenAuth 后、Distribute 前解析并锁定创建素材时的 Secure 渠道，Secure 适配器在构建上游请求时把本地素材 ID 转换为上游引用。前端新增独立素材库页面，管理员在系统设置中配置默认 Secure enterprise 渠道。

**Tech Stack:** Go 1.22、Gin、GORM v2、SQLite/MySQL/PostgreSQL、React 19、TypeScript、TanStack Query、Base UI、i18next、Bun。

---

### Task 1: 素材数据模型与数据库迁移

**Files:**
- Create: model/asset.go
- Create: model/asset_test.go
- Modify: model/main.go 的顺序迁移和快速迁移列表

- [ ] Step 1: 先写测试。使用临时 SQLite 验证 Asset 持久化用户归属、AssetProviderBinding 持久化上游 ID，以及同一用户的 (user_id, idempotency_key_hash) 唯一约束；同一幂等哈希允许不同用户使用。
- [ ] Step 2: 运行 go test ./model -run Asset -count=1，确认因模型不存在而失败。
- [ ] Step 3: 创建 model/asset.go。定义 Asset（公开字符串 ID、user_id、created_by_token_id、image 类型、source_url、pending/processing/active/failed/unknown 状态、错误、幂等哈希和 bigint 时间字段）和 AssetProviderBinding（asset_id、provider、channel_id、upstream_asset_id、upstream_status、可恢复的上游幂等键、凭证指纹、检查时间和错误）。
- [ ] Step 4: 为 Asset 增加公开 ID、用户/状态索引；为绑定增加 asset/provider/channel 唯一组合和 upstream ID 唯一索引，使用 GORM tags，不写方言 SQL。
- [ ] Step 5: 将两个模型加入 model/main.go 的顺序和并发 AutoMigrate 路径；运行 go test ./model -run Asset -count=1，提交 feat: add role asset persistence models。

---

### Task 2: Secure 素材适配器

**Files:**
- Create: service/asset_provider.go
- Create: service/secure_asset_provider.go
- Create: service/secure_asset_provider_test.go

- [ ] Step 1: 用 httptest 写失败测试，验证 /v1/asset/create 的 JSON url、Authorization、Content-Type、Idempotency-Key；验证 /v1/asset/get 的 JSON id、HTTP 错误、缺少 result.id 和未知状态。
- [ ] Step 2: 运行 go test ./service -run SecureAssetProvider -count=1，确认接口不存在时失败。
- [ ] Step 3: 定义 AssetProviderCredential、CreateAssetRequest、ProviderAsset、AssetProviderAdapter；Create 和 Get 都接收 context、凭证和请求参数。
- [ ] Step 4: 实现 SecureAssetProvider，只调用 POST /v1/asset/create 与 POST /v1/asset/get，使用有限超时 HTTP client、Authorization Bearer、common.Marshal/common.DecodeJson；把 Pending/Processing 映射 processing，Active 映射 active，Failed 映射 failed，Unknown 映射 unknown。
- [ ] Step 5: 运行 focused service tests，提交 feat: add Secure role asset provider。

---

### Task 3: AssetService、URL 安全、幂等和按需刷新

**Files:**
- Create: service/asset_url.go
- Create: service/asset_url_test.go
- Create: service/asset_service.go
- Create: service/asset_service_test.go

- [ ] Step 1: 写 URL 测试，拒绝 data、file、asset scheme、localhost、回环和 RFC1918 内网地址；允许公网 HTTPS。写服务测试覆盖默认渠道缺失/禁用/非 enterprise/多 Key、创建入库、同用户同 key 重试、不同参数冲突、不同用户隔离、上游临时查询失败保持原状态、仅 active 可生成引用。
- [ ] Step 2: 运行 go test ./service -run RoleAssetURL -count=1 与 go test ./service -run Asset -count=1，确认失败。
- [ ] Step 3: 实现 ValidateRoleAssetURL，使用 net/url 和现有 SSRF 防护；不下载图片，不接受 multipart/Base64。
- [ ] Step 4: 实现默认 Secure enterprise 渠道解析，要求 ChannelTypeSecure、SecureVideoGroupEnterprise、启用、单 Key；使用 channel.GetBaseURL/GetKeys，凭证指纹使用 common.GenerateHMAC。
- [ ] Step 5: 实现 AssetService.Create/Get/List/Refresh。创建先写 processing 占位及绑定，再调用上游并回写；保留同一上游幂等键以恢复超时；所有查询使用 user_id + asset_id；视图不暴露上游 ID、渠道 ID、API Key。运行测试并提交 feat: add role asset service。

---

### Task 4: 素材 API 控制器和路由

**Files:**
- Create: controller/asset.go
- Create: controller/asset_test.go
- Modify: router/api-router.go

- [ ] Step 1: 写 Gin 测试覆盖 POST/GET/refresh/list 路由、TokenAuth、幂等头透传、越权 asset_not_found，以及响应不包含 asset-local、API Key 或 channel_id。
- [ ] Step 2: 实现 controller/asset.go，创建请求只允许 type=image 和公网 url；从 TokenAuth context 读取 user_id/token_id；统一返回 asset_not_found、asset_invalid_url、asset_idempotency_conflict 等错误。
- [ ] Step 3: 在 SetApiRouter 注册独立 /api/v3/assets group，使用 TokenAuth 和现有限流；不要把该 group 放入 SeedanceRequestConvert。
- [ ] Step 4: 提供可注入 AssetService/provider 的生产构造和测试替身；运行 go test ./controller ./router -run Asset -count=1，提交 feat: expose role asset APIs。

---

### Task 5: 默认素材渠道配置

**Files:**
- Create: controller/asset_settings.go
- Create: controller/asset_settings_test.go
- Modify: router/api-router.go
- Modify: model/option.go
- Modify: web/src/features/system-settings/api.ts
- Modify: web/src/features/system-settings/types.ts
- Modify: web/src/features/system-settings/operations/index.tsx
- Modify: web/src/features/system-settings/operations/section-registry.tsx
- Create: web/src/features/system-settings/operations/secure-asset-section.tsx
- Create: web/src/features/system-settings/operations/__tests__/secure-asset-section.test.tsx

- [ ] Step 1: 增加 secure_asset.default_channel_id 选项，默认空；root-only GET/PUT /api/asset-settings/secure 只返回 Secure enterprise 候选的 id/name/status，不返回 Key。
- [ ] Step 2: 先写测试：过滤非 Secure、discount/overseas、禁用和多 Key；拒绝不存在渠道；保存后持久化选项。
- [ ] Step 3: 使用 GORM 查询和 model.UpdateOption 保存，严格校验启用 Secure enterprise 单 Key；将设置路由放在 RootAuth 下。
- [ ] Step 4: 在 operations settings 增加 Secure role asset section；使用现有 system-options/settings 组件模式，所有文本走 t() 并同步七个 locale。
- [ ] Step 5: 运行 go test ./controller ./model -run AssetSettings -count=1 和 cd web; bun test src/features/system-settings/operations/__tests__/secure-asset-section.test.tsx，提交 feat: configure default Secure asset channel。

---

### Task 6: 视频请求素材识别与固定渠道

**Files:**
- Create: middleware/video_asset_routing.go
- Create: middleware/video_asset_routing_test.go
- Modify: router/video-router.go
- Modify: service/asset_service.go

- [ ] Step 1: 写测试覆盖一个/多个 asset://asset-*、普通公网图不改变路由、角色素材和公网图混用、未知/越权/非 active、跨渠道、超过 9 个，以及成功设置 specific_channel_id。
- [ ] Step 2: 在 Seedance POST tasks middleware 顺序中放置 TokenAuth 后、Distribute 前的 VideoAssetRouting；用 common.UnmarshalBodyReusable 读取请求，不能消耗后续 relay body。
- [ ] Step 3: 只识别 image_url + reference_image 的 asset scheme；调用 AssetService 强制刷新和归属校验；成功将绑定 channel ID 写入 ContextKeyTokenSpecificChannelId，把项目到上游的映射放入请求 context。
- [ ] Step 4: 扩展 Distribute 的 specific channel 分支，素材锁定时不走 affinity、随机候选或跨渠道 retry；禁用/凭证变化/不兼容直接返回 asset_channel_unavailable。
- [ ] Step 5: 运行 go test ./middleware -run VideoAsset -count=1，提交 feat: lock video routing to role asset channel。

---

### Task 7: Secure enterprise 请求转换

**Files:**
- Modify: relay/channel/task/newapivideo/secure_request.go
- Modify: relay/channel/task/newapivideo/native.go
- Modify: relay/channel/task/newapivideo/profile.go
- Create: relay/channel/task/newapivideo/secure_asset_request_test.go

- [ ] Step 1: 添加失败测试：多个 project asset 映射为多个 asset-local 上游引用；请求增加 use_person_character=true 和 extra_images；普通公网图片仍旧走 image_url；角色模式混入公网图和超过 9 个拒绝。
- [ ] Step 2: 在 secureEnterpriseRequest 增加 use_person_character；建立只接收已通过用户、Active 和渠道校验的 project-to-upstream 映射，不在全局 validMediaURL 放开 asset scheme。
- [ ] Step 3: Secure enterprise 有角色素材时只允许受控 asset-local 引用，自动设置 use_person_character=true；无角色素材保留现有公网 image_url/extra_images；保持 video-2.0-pro、720p、5-15 秒、16:9/9:16/1:1、9 图和 3 音频约束。
- [ ] Step 4: 运行 go test ./relay/channel/task/newapivideo -run 'Secure|Asset' -count=1，提交 feat: translate project role assets for Secure enterprise video。

---

### Task 8: Secure 渠道密钥和删除保护

**Files:**
- Modify: controller/channel.go
- Create: controller/channel_asset_guard_test.go
- Modify: model/asset.go

- [ ] Step 1: 写测试覆盖有绑定素材时删除渠道、修改 Secure Key、切换多 Key/enterprise 分组均拒绝；非敏感字段和禁用仍可保存；无绑定渠道行为不变。
- [ ] Step 2: 在删除和更新写库前用 GORM Count 查询绑定；只有 Key、Secure group 或多 Key 相关字段发生变化时拒绝。
- [ ] Step 3: 保留 RootAuth/权限语义，返回明确业务错误；运行 go test ./controller -run ChannelAssetGuard -count=1，提交 feat: protect Secure channels bound to assets。

---

### Task 9: 前端素材库

**Files:**
- Create: web/src/routes/_authenticated/assets/index.tsx
- Create: web/src/features/assets/index.tsx
- Create: web/src/features/assets/api.ts
- Create: web/src/features/assets/types.ts
- Create: web/src/features/assets/components/asset-create-form.tsx
- Create: web/src/features/assets/components/asset-list.tsx
- Create: web/src/features/assets/hooks/use-assets.ts
- Create: web/src/features/assets/__tests__/assets.test.tsx
- Modify: web/src/hooks/use-sidebar-data.ts
- Modify: web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json

- [ ] Step 1: 写组件测试覆盖素材库导航、image/url 创建、自动 Idempotency-Key、2 秒 processing 轮询、终态停止、预览、复制 asset://asset-*、不渲染 asset-local 或 API Key。
- [ ] Step 2: API client 提供 list/create/get/refresh；创建用 crypto.randomUUID 发送 Idempotency-Key；React Query 在 processing 时 refetchInterval=2000，终态返回 false。
- [ ] Step 3: 新增 TanStack file route，让构建生成 routeTree.gen.ts；侧边栏使用 Lucide Images 图标；页面复用 Card/Table/Button/Input，显示项目 ID、预览、状态、时间和复制操作，不提供上游删除。
- [ ] Step 4: 运行 cd web; bun test src/features/assets/__tests__/assets.test.tsx; bun run typecheck; bun run build，提交 feat: add role asset library。

---

### Task 10: E2E、文档和完整验证

**Files:**
- Create: e2e/secure_asset_e2e_test.go
- Create: docs/secure-role-assets.md
- Modify: web/src/features/docs/lib/api-endpoints.ts
- Modify: web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json

- [ ] Step 1: 用 httptest Secure server 和测试数据库验证创建、Secure create/get、asset-* 入库、Active 刷新、Ark 生成固定渠道、use_person_character 和 asset-local 引用；覆盖幂等、跨用户、多个素材、混合拒绝、跨渠道、9 图上限和超时恢复。
- [ ] Step 2: 新增简体中文 docs/secure-role-assets.md，记录接口、状态、Ark 引用格式、API Key 归属、固定渠道、无删除接口和错误码；把接口加入内置 API 文档。
- [ ] Step 3: 运行 go test ./model ./service ./controller ./middleware ./relay/channel/task/newapivideo ./router -count=1; go test ./e2e -run SecureAsset -count=1; cd web; bun run typecheck; bun run build; git diff --check; git status --short。
- [ ] Step 4: 提交 test: verify Secure role asset lifecycle，并输出验收报告，列出 API、数据表、默认渠道、请求转换、状态映射、固定渠道规则、测试证据和真实上游风险。


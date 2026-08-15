# 素材库 API Key 隔离实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让素材库和视频素材引用严格绑定当前 API Key，并要求网页用户先选择已启用 API Key 才能查看或创建素材。

**Architecture:** 素材接口只信任 Bearer API Key 鉴权产生的 `user_id + token_id`，服务层对创建、列表、查询、刷新和视频引用统一执行 Key 级隔离。前端通过登录会话加载已启用 Key 和所选 Key 的完整值，再用该 Key 调用素材接口；React Query 缓存键包含 Key ID，视频素材选择器复用同一身份。

**Tech Stack:** Go 1.22、Gin、GORM v2、testify、React 19、TypeScript、TanStack Query、Base UI、Tailwind CSS、Bun、Node test runner、i18next

---

## 文件结构

- 修改 `service/asset_service.go`：定义素材 Key 身份错误，按 `user_id + token_id` 约束服务方法，并提供旧幂等散列兼容。
- 修改 `service/asset_service_test.go`：保护未绑定拒绝、跨 Key 隔离、列表过滤和幂等兼容契约。
- 修改 `controller/asset.go`、`controller/asset_test.go`：把鉴权上下文的 `token_id` 传入所有素材服务方法并映射错误状态。
- 修改 `middleware/video_asset_routing.go`、`middleware/video_asset_routing_test.go`：视频素材解析时透传当前 API Key ID。
- 修改 `e2e/secure_asset_e2e_test.go`：让既有素材完整链路测试使用显式 Key ID。
- 修改 `web/src/features/keys/api.ts`：提供两个页面共用的已启用 Key 分页加载、完整 Key 获取和 API Key 请求配置。
- 新建 `web/src/features/keys/__tests__/asset-auth.test.ts`：验证 Key 分页筛选和 Bearer 请求配置。
- 修改 `web/src/features/video-generation/api.ts`、`web/src/features/video-generation/__tests__/api.test.ts`：改用共享 Key 能力，删除视频域内重复实现。
- 修改 `web/src/features/assets/api.ts`、`web/src/features/assets/__tests__/list-assets.test.ts`：所有素材请求显式接收完整 API Key。
- 修改 `web/src/features/assets/index.tsx`：增加 API Key 选择和未选择、加载、错误、切换状态。
- 新建 `web/src/features/assets/__tests__/api-key-selection.test.tsx`：覆盖素材页选择、禁用、请求身份和缓存隔离。
- 修改 `web/src/features/assets/__tests__/copy-id.test.tsx`、`web/src/features/assets/__tests__/image-preview.test.tsx`：适配带 Key ID 的缓存键。
- 修改 `web/src/features/video-generation/components/asset-picker.tsx`、`reference-image-editor.tsx`：按当前视频 API Key 加载素材。
- 修改视频素材相关测试：覆盖无 Key 不加载和切换 Key 清空选择。
- 临时创建并最终删除 `web/scripts/add-missing-keys.mjs`：按项目流程写入七语言文案。
- 由脚本修改 `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`：新增素材 Key 选择文案。

## Task 1：服务层 API Key 隔离与幂等兼容

**Files:**
- Modify: `service/asset_service.go`
- Test: `service/asset_service_test.go`

- [ ] **Step 1: 编写未绑定与跨 Key 隔离失败测试**

在 `service/asset_service_test.go` 增加确定性测试，复用现有 fixture：

```go
func TestAssetServiceRequiresTokenIdentity(t *testing.T) {
	assetService, _ := prepareAssetService(t, processingAssetProvider())
	_, err := assetService.Create(context.Background(), 42, 0, AssetCreateInput{
		Type: model.AssetTypeImage,
		URL:  "https://8.8.8.8/character.png",
	})
	require.Error(t, err)
	assert.Equal(t, AssetErrorTokenRequired, AssetErrorCode(err))
}

func TestAssetServiceScopesListAndLookupToToken(t *testing.T) {
	provider := processingAssetProvider()
	assetService, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)
	created, err := assetService.Create(context.Background(), 42, 7, AssetCreateInput{
		Type: model.AssetTypeImage,
		URL:  "https://8.8.8.8/character.png",
	})
	require.NoError(t, err)

	items, total, err := assetService.List(context.Background(), 42, 8, AssetListInput{})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, total)

	_, err = assetService.Get(context.Background(), 42, 8, created.ID)
	require.Error(t, err)
	assert.Equal(t, AssetErrorNotFound, AssetErrorCode(err))
}

func TestAssetServiceHidesLegacyUnboundAssets(t *testing.T) {
	assetService, db := prepareAssetService(t, processingAssetProvider())
	require.NoError(t, db.Create(&model.Asset{
		ID: "asset-00000000000000000000000000000001", UserID: 42,
		CreatedByTokenID: 0, Type: model.AssetTypeImage,
		SourceURL: "https://8.8.8.8/legacy.png", Status: model.AssetStatusActive,
		CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp(),
	}).Error)

	items, total, err := assetService.List(context.Background(), 42, 7, AssetListInput{})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, total)
}
```

- [ ] **Step 2: 运行服务测试并确认失败**

```bash
go test ./service -run 'TestAssetService(RequiresTokenIdentity|ScopesListAndLookupToToken|HidesLegacyUnboundAssets)' -count=1
```

Expected: FAIL，原因是 `AssetErrorTokenRequired` 尚不存在，且 `List`、`Get` 尚未接收 `tokenID`。

- [ ] **Step 3: 实现服务方法的 Key 级数据范围**

在 `service/asset_service.go` 增加：

```go
const AssetErrorTokenRequired = "asset_token_required"

func requireAssetToken(tokenID int) error {
	if tokenID > 0 {
		return nil
	}
	return &AssetServiceError{
		Code: AssetErrorTokenRequired,
		Err:  fmt.Errorf("API key authentication is required"),
	}
}
```

在 `Create`、`List`、`Refresh` 和 `ResolveActiveReferences` 入口首先调用 `requireAssetToken`。公开方法签名统一为：

```go
func (service *AssetService) Get(ctx context.Context, userID int, tokenID int, assetID string) (*AssetView, error)
func (service *AssetService) List(ctx context.Context, userID int, tokenID int, input AssetListInput) ([]AssetView, int64, error)
func (service *AssetService) Refresh(ctx context.Context, userID int, tokenID int, assetID string) (*AssetView, error)
func (service *AssetService) ResolveActiveReferences(ctx context.Context, userID int, tokenID int, assetIDs []string) ([]AssetReferenceBinding, error)
```

列表和所有权查询必须包含 Key 条件：

```go
query := service.db.WithContext(ctx).Model(&model.Asset{}).
	Where("user_id = ? AND created_by_token_id = ?", userID, tokenID)

err := service.db.WithContext(ctx).
	Where("id = ? AND user_id = ? AND created_by_token_id = ?", strings.TrimSpace(assetID), userID, tokenID).
	First(&asset).Error
```

`Get` 调用带 `tokenID` 的 `Refresh`，`ResolveActiveReferences` 调用带 `tokenID` 的 `Refresh` 和 `ownedAssetWithBinding`。

- [ ] **Step 4: 编写每 Key 幂等和旧散列兼容测试**

增加两个契约测试：

```go
func TestAssetServiceScopesIdempotencyToToken(t *testing.T) {
	provider := processingAssetProvider()
	assetService, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)
	input := AssetCreateInput{Type: model.AssetTypeImage, URL: "https://8.8.8.8/character.png", IdempotencyKey: "shared-request"}

	first, err := assetService.Create(context.Background(), 42, 7, input)
	require.NoError(t, err)
	second, err := assetService.Create(context.Background(), 42, 8, input)
	require.NoError(t, err)

	assert.NotEqual(t, first.ID, second.ID)
	assert.Len(t, provider.createCalls, 2)
}

func TestAssetServiceResumesLegacyTokenIdempotencyHash(t *testing.T) {
	provider := processingAssetProvider()
	assetService, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)
	existing, err := assetService.Create(context.Background(), 42, 7, AssetCreateInput{
		Type: model.AssetTypeImage,
		URL:  "https://8.8.8.8/character.png",
	})
	require.NoError(t, err)
	legacyHash := common.GenerateHMAC("role-asset-idempotency:legacy-request")
	require.NoError(t, db.Model(&model.Asset{}).
		Where("id = ?", existing.ID).
		Update("idempotency_key_hash", legacyHash).Error)
	require.NoError(t, db.Model(&model.AssetProviderBinding{}).
		Where("asset_id = ?", existing.ID).
		Updates(map[string]any{"upstream_asset_id": nil, "upstream_status": "Pending"}).Error)
	provider.createCalls = nil

	retried, err := assetService.Create(context.Background(), 42, 7, AssetCreateInput{
		Type:           model.AssetTypeImage,
		URL:            "https://8.8.8.8/character.png",
		IdempotencyKey: "legacy-request",
	})

	require.NoError(t, err)
	assert.Equal(t, existing.ID, retried.ID)
	assert.Len(t, provider.createCalls, 1)
}
```

fixture 在初次创建后显式覆盖数据库中的幂等散列并清空上游 ID，确保重试确实经过旧散列回退和恢复创建路径。

- [ ] **Step 5: 实现每 Key 幂等散列与旧格式回退**

新散列包含 Token ID：

```go
newHash := common.GenerateHMAC(
	"role-asset-idempotency:" + strconv.Itoa(tokenID) + ":" + idempotencyKey,
)
legacyHash := common.GenerateHMAC("role-asset-idempotency:" + idempotencyKey)
```

查找顺序固定为先查 `newHash`，再以相同 `user_id + created_by_token_id` 查 `legacyHash`。创建冲突后的恢复查询使用同样顺序。只有命中相同 Token 的记录时才调用 `resumeCreation`。

- [ ] **Step 6: 运行服务测试并确认通过**

```bash
go test ./service -run 'TestAssetService' -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交服务层改动**

```bash
git add service/asset_service.go service/asset_service_test.go
git commit -m "feat(assets): isolate assets by API key"
```

## Task 2：控制器与视频路由传递 Token 身份

**Files:**
- Modify: `controller/asset.go`
- Modify: `controller/asset_test.go`
- Modify: `middleware/video_asset_routing.go`
- Modify: `middleware/video_asset_routing_test.go`
- Modify: `e2e/secure_asset_e2e_test.go`

- [ ] **Step 1: 编写控制器 Token 透传和错误映射失败测试**

扩展 `fakeAssetControllerService`，记录 `Get`、`List`、`Refresh` 收到的 Token ID；更新 `TestAssetControllerListAndRefresh` 并增加：

```go
assert.Equal(t, 7, fake.listToken)
assert.Equal(t, 7, fake.refreshToken)

fake.getErr = &service.AssetServiceError{
	Code: service.AssetErrorTokenRequired,
	Err:  errors.New("API key authentication is required"),
}
controller.Get(ctx)
assert.Equal(t, http.StatusUnauthorized, recorder.Code)
```

- [ ] **Step 2: 编写视频路由 Token 透传失败测试**

让 `fakeVideoAssetService` 记录 `seenToken`，并在 `videoAssetContext` 设置 `ctx.Set("token_id", 7)`：

```go
func (fake *fakeVideoAssetService) ResolveActiveReferences(
	_ context.Context,
	_ int,
	tokenID int,
	assetIDs []string,
) ([]service.AssetReferenceBinding, error) {
	fake.seenToken = tokenID
	fake.seen = append(fake.seen, assetIDs...)
	return fake.refs, fake.err
}
```

- [ ] **Step 3: 运行控制器和中间件测试并确认失败**

```bash
go test ./controller ./middleware -run 'Asset' -count=1
```

Expected: FAIL，原因是接口和调用方尚未传递 `tokenID`。

- [ ] **Step 4: 实现控制器和视频路由透传**

更新 `AssetControllerService`，并在四个 handler 中使用 `c.GetInt("token_id")`。错误映射增加：

```go
case service.AssetErrorTokenRequired:
	status = http.StatusUnauthorized
```

更新视频服务接口和调用：

```go
ResolveActiveReferences(ctx context.Context, userID int, tokenID int, assetIDs []string) ([]service.AssetReferenceBinding, error)

resolved, err := assetService.ResolveActiveReferences(
	c.Request.Context(),
	c.GetInt("id"),
	c.GetInt("token_id"),
	assetIDs,
)
```

`videoAssetErrorStatus` 将 `AssetErrorTokenRequired` 映射为 HTTP 401。

- [ ] **Step 5: 更新 E2E 测试签名**

`e2e/secure_asset_e2e_test.go` 中所有调用都使用创建素材时的 Token ID `7`：

```go
refs, err := assetService.ResolveActiveReferences(context.Background(), 1001, 7, []string{created.ID})
view, err := assetService.Get(context.Background(), 1001, 7, created.ID)
_, err = assetService.Get(context.Background(), 1002, 7, created.ID)
```

- [ ] **Step 6: 运行后端相关测试并确认通过**

```bash
go test ./controller ./middleware ./service ./e2e -run 'Asset|SecureAsset' -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交身份透传改动**

```bash
git add controller/asset.go controller/asset_test.go middleware/video_asset_routing.go middleware/video_asset_routing_test.go e2e/secure_asset_e2e_test.go
git commit -m "feat(assets): enforce token identity across routes"
```

## Task 3：共享前端 API Key 凭证能力与素材 API

**Files:**
- Modify: `web/src/features/keys/api.ts`
- Create: `web/src/features/keys/__tests__/asset-auth.test.ts`
- Modify: `web/src/features/video-generation/api.ts`
- Modify: `web/src/features/video-generation/__tests__/api.test.ts`
- Modify: `web/src/features/assets/api.ts`
- Modify: `web/src/features/assets/__tests__/list-assets.test.ts`

- [ ] **Step 1: 编写共享 Key 加载和请求配置失败测试**

在 `asset-auth.test.ts` 验证分页、状态筛选和请求配置：

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getApiKeyRequestConfig, loadEnabledApiKeyPages } from '../api'

describe('API key authenticated requests', () => {
  test('builds a request that cannot fall back to dashboard auth', () => {
    assert.deepEqual(getApiKeyRequestConfig('sk-selected'), {
      authToken: 'sk-selected',
      skipAuthRefresh: true,
      skipErrorHandler: true,
    })
  })

  test('loads every page and keeps only enabled keys', async () => {
    const pages: number[] = []
    const makeKey = (id: number, status = 1): ApiKey => ({
      id,
      name: `Key ${id}`,
      key: `sk-${id}`,
      status,
      remain_quota: 0,
      used_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      created_time: 0,
      accessed_time: 0,
      group: 'default',
      cross_group_retry: false,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
    })

    const keys = await loadEnabledApiKeyPages(async ({ p = 1 }) => {
      pages.push(p)
      return {
        success: true,
        data: {
          items: [makeKey(p, p === 2 ? 2 : 1)],
          total: 201,
          page: p,
          page_size: 100,
        },
      }
    })

    assert.deepEqual(pages, [1, 2, 3])
    assert.deepEqual(keys.map((key) => key.id), [1, 3])
  })
})
```

- [ ] **Step 2: 编写素材 API Bearer 身份失败测试**

更新 `list-assets.test.ts`，调用 `listAssets('sk-selected', { page: 2, pageSize: 12 })`，并断言：

```ts
assert.equal(config.authToken, 'sk-selected')
assert.equal(config.skipAuthRefresh, true)
assert.deepEqual(config.params, { type: 'image', page: 2, page_size: 12 })
```

增加 `createAsset`、`refreshAsset` 用例，断言三类请求都携带相同 Key 且不会回退到登录会话。

- [ ] **Step 3: 运行前端 API 测试并确认失败**

```bash
cd web
bun test --parallel=1 src/features/keys/__tests__/asset-auth.test.ts src/features/assets/__tests__/list-assets.test.ts src/features/video-generation/__tests__/api.test.ts
```

Expected: FAIL，原因是共享函数和带 Key 的素材 API 签名尚不存在。

- [ ] **Step 4: 实现共享 Key 工具并迁移视频 API**

在 `web/src/features/keys/api.ts` 导出：

```ts
export function getApiKeyRequestConfig(apiKey: string): ApiRequestConfig {
  return {
    authToken: apiKey,
    skipAuthRefresh: true,
    skipErrorHandler: true,
  }
}

export async function loadEnabledApiKeyPages(
  fetchPage: (params: GetApiKeysParams) => Promise<GetApiKeysResponse>
): Promise<ApiKey[]> {
  const firstPage = await fetchPage({ p: 1, size: 100 })
  if (!firstPage.success || !firstPage.data) return []
  const pageSize = firstPage.data.page_size || 100
  const pageCount = Math.ceil(firstPage.data.total / pageSize)
  const remainingPages = await Promise.all(
    Array.from({ length: Math.max(0, pageCount - 1) }, (_, index) =>
      fetchPage({ p: index + 2, size: pageSize })
    )
  )
  return [
    ...firstPage.data.items,
    ...remainingPages.flatMap((response) => response.data?.items ?? []),
  ].filter((key) => key.status === 1)
}

export async function getEnabledApiKeys(): Promise<ApiKey[]> {
  return loadEnabledApiKeyPages(getApiKeys)
}

export async function getApiKeyValue(id: number): Promise<string> {
  const response = await fetchTokenKey(id)
  if (!response.success || !response.data?.key) {
    throw new Error(response.message || 'Unable to load API key')
  }
  return response.data.key
}
```

从 `video-generation/api.ts` 删除重复函数，改为导入共享函数；以薄包装保留 `getVideoApiKeys` 和 `getVideoApiKeyValue` 导出，避免扩大调用方变更。

- [ ] **Step 5: 实现所有素材 API 的显式 Key 参数**

`web/src/features/assets/api.ts` 使用共享请求配置：

```ts
export async function listAssets(
  apiKey: string,
  params: AssetListParams = {}
): Promise<AssetListResponse> {
  const response = await api.get<AssetListResponse>('/api/v3/assets', {
    ...getApiKeyRequestConfig(apiKey),
    params: {
      type: 'image',
      page: params.page ?? 1,
      page_size: params.pageSize ?? 20,
    },
  })
  return response.data
}

export async function createAsset(apiKey: string, url: string): Promise<AssetResponse>
export async function refreshAsset(apiKey: string, asset: Asset): Promise<AssetResponse>
```

`createAsset` 与 `refreshAsset` 都把 `getApiKeyRequestConfig(apiKey)` 传给 Axios；创建请求同时保留随机 `Idempotency-Key`。

- [ ] **Step 6: 运行 API 测试并确认通过**

```bash
cd web
bun test --parallel=1 src/features/keys/__tests__/asset-auth.test.ts src/features/assets/__tests__/list-assets.test.ts src/features/video-generation/__tests__/api.test.ts
```

Expected: PASS。

- [ ] **Step 7: 提交共享凭证和 API 改动**

```bash
git add web/src/features/keys/api.ts web/src/features/keys/__tests__/asset-auth.test.ts web/src/features/video-generation/api.ts web/src/features/video-generation/__tests__/api.test.ts web/src/features/assets/api.ts web/src/features/assets/__tests__/list-assets.test.ts
git commit -m "refactor(web): share API key request identity"
```

## Task 4：素材库页面 API Key 选择交互

**Files:**
- Modify: `web/src/features/assets/index.tsx`
- Create: `web/src/features/assets/__tests__/api-key-selection.test.tsx`
- Modify: `web/src/features/assets/__tests__/copy-id.test.tsx`
- Modify: `web/src/features/assets/__tests__/image-preview.test.tsx`

- [ ] **Step 1: 编写未选择 Key 的页面行为失败测试**

在 `api-key-selection.test.tsx` 使用 happy-dom、QueryClient 和 Axios adapter 挂载 `Assets`：

```ts
test('does not load assets and disables creation until an API key is selected', async () => {
  const mounted = await mountAssets()
  assert.equal(mounted.assetRequests.length, 0)
  assert.equal(mounted.container.querySelector('input[aria-label="Public image URL"]')?.hasAttribute('disabled'), true)
  assert.equal(mounted.container.querySelector('button[type="submit"]')?.hasAttribute('disabled'), true)
  assert.match(mounted.container.textContent ?? '', /Select an API key to view assets/)
})
```

- [ ] **Step 2: 编写选择、切换和创建身份失败测试**

同一测试文件增加：

```ts
test('loads only the selected key cache and creates with that key', async () => {
  const mounted = await mountAssets()
  await mounted.selectKey('7')
  await mounted.waitForAssetRequests(1)
  assert.equal(mounted.assetRequests[0]?.authToken, 'sk-seven')

  await mounted.submitURL('https://example.com/character.png')
  await mounted.waitForAssetRequests(2)
  assert.equal(mounted.assetRequests[1]?.method, 'post')
  assert.equal(mounted.assetRequests[1]?.authToken, 'sk-seven')
  await mounted.unmount()
})

test('switches to an isolated asset cache when another key is selected', async () => {
  const mounted = await mountAssets()
  await mounted.selectKey('7')
  await mounted.waitForText('asset-for-seven')

  await mounted.selectKey('8')
  assert.doesNotMatch(mounted.container.textContent ?? '', /asset-for-seven/)
  await mounted.waitForText('asset-for-eight')

  assert.deepEqual(
    mounted.assetRequests.filter((request) => request.method === 'get').map((request) => request.authToken),
    ['sk-seven', 'sk-eight']
  )
  await mounted.unmount()
})
```

`mountAssets` 在同一测试文件中用 happy-dom、QueryClient 和 Axios adapter 实现，并暴露上述用户操作方法；`selectKey` 派发原生 `change` 事件，`submitURL` 派发输入事件后点击 submit，`waitForText` 和 `waitForAssetRequests` 通过 React `act` 等待明确状态，不使用固定 sleep。adapter 对 `/api/token/7/key`、`/api/token/8/key`、`/api/v3/assets` 返回显式 fixture，并把 `method` 与 `authToken` 记录到 `assetRequests`。不读取组件内部 state，也不断言完整 Tailwind class。

- [ ] **Step 3: 运行页面交互测试并确认失败**

```bash
cd web
bun test --parallel=1 src/features/assets/__tests__/api-key-selection.test.tsx
```

Expected: FAIL，当前页面没有 Key 选择且会立即请求全部用户素材。

- [ ] **Step 4: 实现页面选择、查询和 mutation 状态**

在 `Assets` 中加入：

```ts
const [selectedKeyId, setSelectedKeyId] = useState('')
const apiKeysQuery = useQuery({
  queryKey: ['assets', 'api-keys'],
  queryFn: getEnabledApiKeys,
  staleTime: 30_000,
})
const selectedKey = apiKeysQuery.data?.find(
  (apiKey) => String(apiKey.id) === selectedKeyId
)
const apiKeyValueQuery = useQuery({
  queryKey: ['assets', 'api-key-value', selectedKeyId],
  queryFn: () => getApiKeyValue(Number(selectedKeyId)),
  enabled: Boolean(selectedKey),
  staleTime: 30_000,
})
const selectedApiKey = apiKeyValueQuery.data ?? ''
const assetQueryKey = ['role-assets', selectedKeyId]
```

`loadAssets(apiKey)`、创建和刷新都接收完整 Key。素材查询设置 `enabled: Boolean(selectedApiKey)`。创建 mutation 的 variables 包含 `{ apiKey, tokenId, url }`，成功时只失效 `['role-assets', tokenId]`。创建 pending 时禁用 Key 选择框。

- [ ] **Step 5: 实现可访问的选择区和空状态**

在创建卡片之前增加带 `FieldLabel` 的 `NativeSelect`：

```tsx
<NativeSelect
  id='asset-api-key'
  aria-label={t('API key')}
  value={selectedKeyId}
  disabled={apiKeysQuery.isLoading || createMutation.isPending}
  onChange={(event) => setSelectedKeyId(event.target.value)}
>
  <NativeSelectOption value=''>
    {apiKeysQuery.isLoading ? t('Loading API keys...') : t('Select an API key')}
  </NativeSelectOption>
  {apiKeys.map((apiKey) => (
    <NativeSelectOption key={apiKey.id} value={String(apiKey.id)}>
      {`${apiKey.name} · ${apiKey.group || 'default'} · ${apiKey.key}`}
    </NativeSelectOption>
  ))}
</NativeSelect>
```

未选择时显示 `t('Select an API key to view assets.')`。Key 列表为空时显示 `t('No enabled API keys')`。完整 Key 获取失败时显示 `t('Failed to load API key')`，不得回退到无 Key 的素材请求。

公网 URL 输入框和创建按钮的 disabled 条件同时包含 `!selectedApiKey`。现有预览、复制和刷新交互保持不变。

- [ ] **Step 6: 更新现有素材 UI 测试缓存键**

`copy-id.test.tsx` 和 `image-preview.test.tsx` 注入 Key 列表、完整 Key 查询值和带 Key 的素材缓存：

```ts
queryClient.setQueryData(['assets', 'api-keys'], [apiKeyFixture])
queryClient.setQueryData(['assets', 'api-key-value', '7'], 'sk-seven')
queryClient.setQueryData(['role-assets', '7'], assetList)
```

测试通过真实 select 交互选择 Key，不向生产组件增加测试专用 props。

- [ ] **Step 7: 运行素材页面测试并确认通过**

```bash
cd web
bun test --parallel=1 src/features/assets/__tests__
```

Expected: PASS。

- [ ] **Step 8: 提交素材页交互**

```bash
git add web/src/features/assets/index.tsx web/src/features/assets/__tests__
git commit -m "feat(web): require API key selection for assets"
```

## Task 5：视频生成素材选择器绑定当前 API Key

**Files:**
- Modify: `web/src/features/video-generation/index.tsx`
- Modify: `web/src/features/video-generation/components/reference-image-editor.tsx`
- Modify: `web/src/features/video-generation/components/asset-picker.tsx`
- Modify: `web/src/features/video-generation/components/__tests__/asset-picker.test.tsx`
- Modify: `web/src/features/video-generation/components/__tests__/reference-image-editor.test.tsx`
- Modify: `web/src/features/video-generation/__tests__/asset-library-integration.test.tsx`

- [ ] **Step 1: 编写 AssetPicker 无 Key 与 Bearer 请求失败测试**

为 `AssetPickerProps` 规划 `apiKeyId: number | null` 和 `apiKey: string`。更新测试 harness 并新增：

```ts
test('does not request assets without a selected API key', async () => {
  const mounted = await mountPicker({ apiKeyId: null, apiKey: '' })
  assert.deepEqual(mounted.requests, [])
  assert.match(mounted.container.textContent ?? '', /Select an API key to view assets/)
})

test('uses the selected API key when loading assets', async () => {
  const mounted = await mountPicker({ apiKeyId: 7, apiKey: 'sk-seven' })
  assert.equal(mounted.authTokens[0], 'sk-seven')
})
```

- [ ] **Step 2: 编写视频 Key 切换清空素材失败测试**

在 `asset-library-integration.test.tsx` 通过实际 Key select 从 7 切换到 8，先选中一个素材，再断言素材 ID 被清空且新列表请求使用 `sk-eight`：

```ts
assert.deepEqual(formAssetIdsAfterSwitch, [])
assert.equal(assetRequests.at(-1)?.authToken, 'sk-eight')
```

- [ ] **Step 3: 运行视频素材测试并确认失败**

```bash
cd web
bun test --parallel=1 src/features/video-generation/components/__tests__/asset-picker.test.tsx src/features/video-generation/components/__tests__/reference-image-editor.test.tsx src/features/video-generation/__tests__/asset-library-integration.test.tsx
```

Expected: FAIL，现有 AssetPicker 没有 API Key props，且切换 Key 不会清空素材。

- [ ] **Step 4: 实现 Key 身份向素材选择器传递**

`VideoGeneration` 增加一个用于素材列表的完整 Key query，不得在每次翻页重复读取完整 Key。把 `selectedKey?.id ?? null` 和完整 Key 传入 `ReferenceImageEditor`，再传到 `AssetPicker`：

```tsx
<AssetPicker
  apiKeyId={props.apiKeyId}
  apiKey={props.apiKey}
  selectedIds={props.assetIds}
  limit={Math.min(props.imageLimit, ROLE_ASSET_LIMIT)}
  onChange={props.onAssetIdsChange}
/>
```

AssetPicker 查询：

```ts
queryKey: ['video-generation', 'role-assets', props.apiKeyId, page],
queryFn: () => listAssets(props.apiKey, { page, pageSize: ASSET_PAGE_SIZE }),
enabled: Boolean(props.apiKeyId && props.apiKey),
```

- [ ] **Step 5: 切换视频 Key 时清空素材状态**

在 `VideoGeneration` 的 `selectedKeyId` 变化 effect 中执行：

```ts
form.setValue('assetIds', [], { shouldValidate: true })
```

AssetPicker 在 `apiKeyId` 变化时把内部页码重置为 1，不保留旧 Key 的选中素材。

- [ ] **Step 6: 运行视频素材测试并确认通过**

```bash
cd web
bun test --parallel=1 src/features/video-generation/components/__tests__/asset-picker.test.tsx src/features/video-generation/components/__tests__/reference-image-editor.test.tsx src/features/video-generation/__tests__/asset-library-integration.test.tsx
```

Expected: PASS。

- [ ] **Step 7: 提交视频页联动**

```bash
git add web/src/features/video-generation/index.tsx web/src/features/video-generation/components web/src/features/video-generation/__tests__/asset-library-integration.test.tsx
git commit -m "feat(video): scope asset picker to selected API key"
```

## Task 6：七语言文案同步

**Files:**
- Temporarily create/delete: `web/scripts/add-missing-keys.mjs`
- Modify via script: `web/src/i18n/locales/en.json`
- Modify via script: `web/src/i18n/locales/zh.json`
- Modify via script: `web/src/i18n/locales/zh-TW.json`
- Modify via script: `web/src/i18n/locales/fr.json`
- Modify via script: `web/src/i18n/locales/ja.json`
- Modify via script: `web/src/i18n/locales/ru.json`
- Modify via script: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 运行 i18n 基线同步**

```bash
cd web
bun run i18n:sync
```

Expected: 命令成功并生成同步报告；记录与本次新增 Key 无关的既有报告项，不扩大修复范围。

- [ ] **Step 2: 创建规定格式的临时翻译脚本**

按 `.agents/skills/i18n-translate/SKILL.md` 的完整 `stableStringify` 和写入循环创建 `web/scripts/add-missing-keys.mjs`，其中 `newKeys` 为：

```js
const newKeys = {
  en: {
    'Choose an enabled API key before creating or viewing assets.': 'Choose an enabled API key before creating or viewing assets.',
    'Failed to load API key': 'Failed to load API key',
    'No enabled API keys': 'No enabled API keys',
    'Select an API key to view assets.': 'Select an API key to view assets.',
  },
  zh: {
    'Choose an enabled API key before creating or viewing assets.': '请选择已启用的 API Key 后创建或查看素材。',
    'Failed to load API key': '加载 API Key 失败',
    'No enabled API keys': '没有已启用的 API Key',
    'Select an API key to view assets.': '请选择 API Key 后查看素材。',
  },
  'zh-TW': {
    'Choose an enabled API key before creating or viewing assets.': '請先選擇已啟用的 API Key，再建立或查看素材。',
    'Failed to load API key': '載入 API Key 失敗',
    'No enabled API keys': '沒有已啟用的 API Key',
    'Select an API key to view assets.': '請選擇 API Key 後查看素材。',
  },
  fr: {
    'Choose an enabled API key before creating or viewing assets.': 'Choisissez une clé API active pour créer ou consulter des ressources.',
    'Failed to load API key': 'Échec du chargement de la clé API',
    'No enabled API keys': 'Aucune clé API active',
    'Select an API key to view assets.': 'Sélectionnez une clé API pour afficher les ressources.',
  },
  ja: {
    'Choose an enabled API key before creating or viewing assets.': '素材を作成または表示するには、有効な API キーを選択してください。',
    'Failed to load API key': 'API キーを読み込めませんでした',
    'No enabled API keys': '有効な API キーがありません',
    'Select an API key to view assets.': '素材を表示する API キーを選択してください。',
  },
  ru: {
    'Choose an enabled API key before creating or viewing assets.': 'Выберите активный API-ключ, чтобы создавать или просматривать материалы.',
    'Failed to load API key': 'Не удалось загрузить API-ключ',
    'No enabled API keys': 'Нет активных API-ключей',
    'Select an API key to view assets.': 'Выберите API-ключ для просмотра материалов.',
  },
  vi: {
    'Choose an enabled API key before creating or viewing assets.': 'Chọn một API Key đang bật để tạo hoặc xem tư liệu.',
    'Failed to load API key': 'Không thể tải API Key',
    'No enabled API keys': 'Không có API Key nào đang bật',
    'Select an API key to view assets.': 'Chọn API Key để xem tư liệu.',
  },
}
```

- [ ] **Step 3: 应用翻译、同步并删除临时脚本**

```powershell
Set-Location web
node scripts/add-missing-keys.mjs
bun run i18n:sync
Remove-Item -LiteralPath scripts/add-missing-keys.mjs
```

Expected: 七个 locale 各应用四个 Key，最终同步成功，临时脚本不存在。

- [ ] **Step 4: 验证 Key 完整性**

```bash
cd web
rg -n 'Select an API key to view assets|No enabled API keys|Failed to load API key' src/i18n/locales
```

Expected: 每个新 Key 在七个 locale 中各出现一次。

- [ ] **Step 5: 提交翻译**

```bash
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat(i18n): translate asset API key selection"
```

## Task 7：完整验证与本地页面验收

**Files:**
- Verify all modified files

- [ ] **Step 1: 格式化 Go 文件并运行后端测试**

```bash
gofmt -w service/asset_service.go service/asset_service_test.go controller/asset.go controller/asset_test.go middleware/video_asset_routing.go middleware/video_asset_routing_test.go e2e/secure_asset_e2e_test.go
go test ./controller ./middleware ./service ./e2e -run 'Asset|SecureAsset' -count=1
```

Expected: gofmt 无错误，所有相关 Go 测试 PASS。

- [ ] **Step 2: 运行完整前端相关测试**

```bash
cd web
bun test --parallel=1 src/features/keys/__tests__/asset-auth.test.ts src/features/assets/__tests__ src/features/video-generation/__tests__/api.test.ts src/features/video-generation/__tests__/asset-library-integration.test.tsx src/features/video-generation/components/__tests__/asset-picker.test.tsx src/features/video-generation/components/__tests__/reference-image-editor.test.tsx
```

Expected: 所有测试 PASS，无未处理 React `act(...)` 警告。

- [ ] **Step 3: 运行类型、lint、格式和构建检查**

```bash
cd web
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/keys/api.ts src/features/keys/__tests__/asset-auth.test.ts src/features/assets src/features/video-generation/api.ts src/features/video-generation/index.tsx src/features/video-generation/components/asset-picker.tsx src/features/video-generation/components/reference-image-editor.tsx
bun run format:check
bun run build
```

Expected: 四个命令均退出 0；涉及文件无 lint error，生产构建成功。

- [ ] **Step 4: 重建本地服务并验证健康状态**

```bash
docker compose -f docker-compose.local.yml up -d --build new-api
docker compose -f docker-compose.local.yml ps
```

Expected: `new-api-local-new-api-1` 状态为 healthy，`http://localhost:3000/api/status` 返回成功。

- [ ] **Step 5: 浏览器验收素材库**

在已登录会话打开 `http://localhost:3000/assets`，验证：

1. 初始未选择 Key，素材表不显示历史 `created_by_token_id=0` 素材。
2. URL 输入框和创建按钮禁用，页面提示先选择 API Key。
3. 选择已启用 Key 后只加载该 Key 的素材；本地现有 Key 没有绑定素材时显示空列表。
4. 切换另一个 Key 时出现独立加载或空状态，不闪现前一个 Key 数据。
5. 不提交创建表单，避免产生真实上游素材。

- [ ] **Step 6: 浏览器验收视频生成页**

打开 `http://localhost:3000/video-generation`，验证：

1. 未选择视频 API Key 时，素材库模式不发起素材请求。
2. 选择 Key 后，素材选择器只显示该 Key 的素材。
3. 切换 Key 后，已选择素材 ID 被清空。
4. 不创建真实视频任务。

- [ ] **Step 7: 检查差异并提交最终修正**

```bash
git diff --check
git status --short
git log -7 --oneline
```

Expected: `git diff --check` 无输出；工作区只包含预期修正。如验收产生必要修正，重新运行对应测试后提交：

```bash
git add service/asset_service.go service/asset_service_test.go controller/asset.go controller/asset_test.go middleware/video_asset_routing.go middleware/video_asset_routing_test.go e2e/secure_asset_e2e_test.go web/src/features/keys web/src/features/assets web/src/features/video-generation web/src/i18n/locales
git commit -m "fix(assets): finalize API key selection flow"
```

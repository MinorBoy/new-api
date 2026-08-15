# 视频生成页使用素材库 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `/video-generation` 中增加与公网 URL 互斥的角色素材选择模式，并按 Seedance 2.0/2.5 动态执行 `9/3/3`、`30/10/10` 参考媒体限制。

**Architecture:** 保持现有 Ark 视频任务接口不变，在表单状态中独立保存图片来源模式和素材 ID，由请求构建层把素材 ID 转成 `asset://` 引用。新增分页 `AssetPicker` 和组合式 `ReferenceImageEditor`，后端素材中间件同时接受新旧公开素材 ID，但角色素材路由仍固定为 Secure 2.0、最多 9 个。

**Tech Stack:** Go 1.22、Gin、React 19、TypeScript、React Hook Form、Zod、TanStack Query、Base UI、Tailwind CSS、Bun/node:test、happy-dom、Playwright。

---

## 文件结构

- Modify: `middleware/video_asset_routing.go` — 兼容新旧公开素材 ID。
- Modify: `middleware/video_asset_routing_test.go` — 保护 ID 格式、混用和 9 个上限。
- Modify: `web/src/features/video-generation/types.ts` — 增加图片来源和素材 ID 表单字段。
- Modify: `web/src/features/video-generation/lib/defaults.ts` — 默认使用公网 URL，素材选择为空。
- Modify: `web/src/features/video-generation/lib/request.ts` — 模型系列、动态媒体上限和素材请求组装。
- Modify: `web/src/features/video-generation/lib/schema.ts` — 动态数量、素材 ID 和当前模式校验。
- Modify: `web/src/features/video-generation/__tests__/request.test.ts` — 请求与校验行为测试。
- Modify: `web/src/features/video-generation/__tests__/defaults.test.ts` — 新表单默认值测试。
- Modify: `web/src/features/assets/api.ts` — 素材列表分页参数。
- Create: `web/src/features/assets/__tests__/list-assets.test.ts` — 分页 API 参数契约。
- Modify: `web/src/features/video-generation/components/reference-media-editor.tsx` — 接收动态上限和禁用状态。
- Create: `web/src/features/video-generation/components/asset-picker.tsx` — 分页素材网格、状态和多选。
- Create: `web/src/features/video-generation/components/reference-image-editor.tsx` — 图片来源切换及模型可用性。
- Create: `web/src/features/video-generation/components/__tests__/asset-picker.test.tsx` — 素材选择行为。
- Create: `web/src/features/video-generation/components/__tests__/reference-image-editor.test.tsx` — 模式和模型切换行为。
- Modify: `web/src/features/video-generation/index.tsx` — 表单接线、动态计数和请求预览。
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` — 新增七语种 UI 文案。

### Task 1: 兼容可读素材 ID

**Files:**
- Modify: `middleware/video_asset_routing.go:15`
- Modify: `middleware/video_asset_routing_test.go:35`

- [ ] **Step 1: 写入新旧格式都可路由的失败测试**

在 `middleware/video_asset_routing_test.go` 增加 `fmt` 导入和表格测试：

```go
func TestVideoAssetRoutingAcceptsSupportedPublicAssetIDFormats(t *testing.T) {
	tests := []string{
		"asset-00000000000000000000000000000001",
		"asset-20260401123823-6d4x2",
	}

	for _, assetID := range tests {
		t.Run(assetID, func(t *testing.T) {
			fake := &fakeVideoAssetService{refs: []service.AssetReferenceBinding{{
				AssetID: assetID, UpstreamAssetID: "asset-local-one", ChannelID: 77,
			}}}
			ctx, recorder := videoAssetContext(fmt.Sprintf(
				`{"model":"video-2.0-pro","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://%s"}}]}`,
				assetID,
			))

			NewVideoAssetRouting(fake)(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, []string{assetID}, fake.seen)
		})
	}
}
```

- [ ] **Step 2: 运行测试并确认新格式失败**

Run: `go test ./middleware -run TestVideoAssetRoutingAcceptsSupportedPublicAssetIDFormats -count=1`

Expected: FAIL，新格式响应 `400` 且服务没有收到素材 ID。

- [ ] **Step 3: 最小化修改公开素材 ID 正则**

将 `middleware/video_asset_routing.go` 中的正则替换为：

```go
var projectAssetIDPattern = regexp.MustCompile(
	`^asset-(?:[0-9a-f]{32}|[0-9]{14}-[a-z0-9]{5})$`,
)
```

旧 32 位十六进制 ID 继续可读，新建素材的时间戳格式同时可用；不要放宽到任意 `asset-*`。

- [ ] **Step 4: 运行素材路由回归测试**

Run: `go test ./middleware -run TestVideoAssetRouting -count=1`

Expected: PASS；混用、重复、跨渠道、非 Active 和超过 9 个的既有断言保持通过。

- [ ] **Step 5: 提交后端兼容修复**

```bash
git add middleware/video_asset_routing.go middleware/video_asset_routing_test.go
git commit -m "fix(assets): accept readable IDs in video routing"
```

### Task 2: 建立模型能力、表单类型和请求校验

**Files:**
- Modify: `web/src/features/video-generation/types.ts`
- Modify: `web/src/features/video-generation/lib/defaults.ts`
- Modify: `web/src/features/video-generation/lib/request.ts`
- Modify: `web/src/features/video-generation/lib/schema.ts`
- Modify: `web/src/features/video-generation/__tests__/request.test.ts`
- Modify: `web/src/features/video-generation/__tests__/defaults.test.ts`

- [ ] **Step 1: 先扩展测试 fixture 和失败断言**

给所有 `VideoGenerationForm` fixture 增加：

```ts
imageSource: 'url',
assetIds: [],
```

在 `request.test.ts` 增加以下行为测试：

```ts
test('uses model-specific media limits', () => {
  assert.deepEqual(getVideoMediaLimits('doubao-seedance-2-0-260128'), {
    images: 9,
    videos: 3,
    audios: 3,
  })
  assert.deepEqual(getVideoMediaLimits('doubao-seedance-2-5-260628'), {
    images: 30,
    videos: 10,
    audios: 10,
  })
  assert.deepEqual(getVideoMediaLimits('other-model'), {
    images: 9,
    videos: 3,
    audios: 3,
  })
})

test('only the base Seedance 2.0 model supports role assets', () => {
  assert.equal(supportsRoleAssets('doubao-seedance-2-0-260128'), true)
  assert.equal(supportsRoleAssets('doubao-seedance-2-0-fast-260528'), false)
  assert.equal(supportsRoleAssets('doubao-seedance-2-0-mini-260615'), false)
  assert.equal(supportsRoleAssets('doubao-seedance-2-5-260628'), false)
})

test('builds Ark asset references and excludes conflicting image and video URLs', () => {
  const request = buildVideoRequest({
    ...DEFAULT_VIDEO_FORM,
    imageSource: 'asset',
    assetIds: ['asset-20260401123823-6d4x2', 'asset-20260401124109-k8p7m'],
  })

  assert.deepEqual(request.content.filter((item) => item.type === 'image_url'), [
    {
      type: 'image_url',
      role: 'reference_image',
      image_url: { url: 'asset://asset-20260401123823-6d4x2' },
    },
    {
      type: 'image_url',
      role: 'reference_image',
      image_url: { url: 'asset://asset-20260401124109-k8p7m' },
    },
  ])
  assert.equal(request.content.some((item) => item.type === 'video_url'), false)
  assert.equal(request.content.some((item) => item.type === 'audio_url'), true)
})
```

再用 `createVideoGenerationSchema(t).safeParse(...)` 断言：2.0 的第 10 张图片失败、2.5 的第 30 张
通过而第 31 张失败、2.5 的第 10 个视频/音频通过而第 11 个失败、素材模式超过 9 个失败、2.5 使用
素材模式失败、空 prompt 且当前模式没有有效参考时失败。

- [ ] **Step 2: 运行测试并确认类型或导出缺失**

Run: `cd web && bun test src/features/video-generation/__tests__/request.test.ts src/features/video-generation/__tests__/defaults.test.ts`

Expected: FAIL，缺少 `imageSource`、`assetIds`、`getVideoMediaLimits` 和 `supportsRoleAssets`。

- [ ] **Step 3: 增加表单类型和默认值**

在 `types.ts` 增加并接入：

```ts
export type VideoImageSource = 'url' | 'asset'

export type VideoGenerationForm = {
  model: string
  prompt: string
  imageSource: VideoImageSource
  assetIds: string[]
  media: VideoMedia
  resolution: string
  ratio: string
  duration: number
  executionExpiresAfter?: number
  generateAudio: boolean
  watermark: boolean
  returnLastFrame: boolean
  callbackUrl?: string
}
```

在 `DEFAULT_VIDEO_FORM` 中加入 `imageSource: 'url'` 和 `assetIds: []`；克隆默认表单时也复制
`assetIds`，避免重置时共享数组。

- [ ] **Step 4: 实现模型能力和请求组装**

在 `request.ts` 用一个归一化入口实现能力判断：

```ts
export type VideoMediaLimits = Record<keyof VideoMedia, number>

const DEFAULT_VIDEO_MEDIA_LIMITS: VideoMediaLimits = {
  images: 9,
  videos: 3,
  audios: 3,
}

const SEEDANCE_25_MEDIA_LIMITS: VideoMediaLimits = {
  images: 30,
  videos: 10,
  audios: 10,
}

function compactModelName(model: string): string {
  return model.toLowerCase().trim().replaceAll(/[-_.\s]/g, '')
}

export function getVideoMediaLimits(model: string): VideoMediaLimits {
  return compactModelName(model).includes('seedance25')
    ? SEEDANCE_25_MEDIA_LIMITS
    : DEFAULT_VIDEO_MEDIA_LIMITS
}

export function supportsRoleAssets(model: string): boolean {
  const compact = compactModelName(model)
  return (
    compact.includes('seedance20') &&
    !compact.includes('seedance20fast') &&
    !compact.includes('seedance20mini')
  )
}
```

将 `validateMediaLimits` 改为 `validateMediaLimits(model, media)`，使用 `getVideoMediaLimits(model)`。
`buildVideoRequest()` 根据 `imageSource` 二选一：素材模式追加
`assetIds.map((id) => 'asset://' + id)`，并完全忽略 `media.images` 与 `media.videos`；公网 URL 模式保持
现有图片和视频行为；两种模式都追加音频。

- [ ] **Step 5: 将 Schema 改为动态校验**

保留每个 URL 的 HTTP(S) 校验，移除静态 `.max(9/3/3)`，在 `superRefine` 中读取
`getVideoMediaLimits(value.model)` 并逐项增加错误：

```ts
const PROJECT_ASSET_ID_PATTERN =
  /^asset-(?:[0-9a-f]{32}|[0-9]{14}-[a-z0-9]{5})$/

const limits = getVideoMediaLimits(value.model)
for (const kind of ['images', 'videos', 'audios'] as const) {
  if (value.media[kind].length > limits[kind]) {
    context.addIssue({
      code: 'custom',
      path: ['media', kind],
      message: t('{{kind}} cannot exceed {{count}} items', {
        kind: t(MEDIA_LABELS[kind]),
        count: limits[kind],
      }),
    })
  }
}
```

`assetIds` 使用 `z.array(z.string().regex(PROJECT_ASSET_ID_PATTERN, ...)).max(9, ...)`。当
`imageSource === 'asset'` 且 `supportsRoleAssets(model)` 为假时，把错误放在 `imageSource`；计算“至少一个
prompt 或参考”时只统计当前图片模式的数据、音频和公网模式下的视频，不统计隐藏陈旧字段。

- [ ] **Step 6: 运行请求、默认值和类型检查**

Run: `cd web && bun test src/features/video-generation/__tests__/request.test.ts src/features/video-generation/__tests__/defaults.test.ts && bun run typecheck`

Expected: PASS；没有 TypeScript 错误。

- [ ] **Step 7: 提交模型能力和请求层**

```bash
git add web/src/features/video-generation/types.ts web/src/features/video-generation/lib/defaults.ts web/src/features/video-generation/lib/request.ts web/src/features/video-generation/lib/schema.ts web/src/features/video-generation/__tests__/request.test.ts web/src/features/video-generation/__tests__/defaults.test.ts
git commit -m "feat(video): add model-aware asset request state"
```

### Task 3: 给素材列表 API 增加分页参数

**Files:**
- Modify: `web/src/features/assets/api.ts`
- Create: `web/src/features/assets/__tests__/list-assets.test.ts`

- [ ] **Step 1: 写入分页参数失败测试**

在新测试中临时替换 Axios adapter，调用 `listAssets({ page: 2, pageSize: 12 })`，断言请求参数：

```ts
test('passes picker pagination to the asset list endpoint', async () => {
  const originalAdapter = api.defaults.adapter
  let seenParams: unknown
  api.defaults.adapter = async (config) => {
    seenParams = config.params
    return {
      data: { success: true, data: { items: [], total: 0, page: 2, page_size: 12 } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }

  try {
    await listAssets({ page: 2, pageSize: 12 })
    assert.deepEqual(seenParams, { type: 'image', page: 2, page_size: 12 })
  } finally {
    api.defaults.adapter = originalAdapter
  }
})
```

- [ ] **Step 2: 运行测试并确认签名不接受参数**

Run: `cd web && bun test src/features/assets/__tests__/list-assets.test.ts`

Expected: FAIL，`listAssets` 期望 0 个参数。

- [ ] **Step 3: 实现可选分页参数并保持旧调用兼容**

```ts
export type AssetListParams = { page?: number; pageSize?: number }

export async function listAssets(
  params: AssetListParams = {}
): Promise<AssetListResponse> {
  const response = await api.get<AssetListResponse>('/api/v3/assets', {
    params: {
      type: 'image',
      page: params.page ?? 1,
      page_size: params.pageSize ?? 20,
    },
  })
  return response.data
}
```

`Assets` 页面继续调用 `listAssets()`，行为仍为第 1 页、每页 20 条。

- [ ] **Step 4: 运行素材 API 和现有素材页测试**

Run: `cd web && bun test src/features/assets/__tests__/list-assets.test.ts src/features/assets/__tests__/image-preview.test.tsx src/features/assets/__tests__/copy-id.test.tsx`

Expected: PASS。

- [ ] **Step 5: 提交分页 API**

```bash
git add web/src/features/assets/api.ts web/src/features/assets/__tests__/list-assets.test.ts
git commit -m "feat(assets): paginate asset list requests"
```

### Task 4: 实现可访问的分页素材选择器

**Files:**
- Create: `web/src/features/video-generation/components/asset-picker.tsx`
- Create: `web/src/features/video-generation/components/__tests__/asset-picker.test.tsx`

- [ ] **Step 1: 写素材状态、多选、上限和分页失败测试**

使用现有 happy-dom、`QueryClientProvider` 和 `I18nextProvider` 测试模式，给 Axios adapter 返回一条
`active`、一条 `processing` 和一条 `failed` 素材。用受控 Harness 渲染：

```tsx
function Harness() {
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  return (
    <AssetPicker
      selectedIds={selectedIds}
      limit={1}
      onChange={(ids) => {
        changes.push(ids)
        setSelectedIds(ids)
      }}
    />
  )
}
```

断言：

```ts
assert.equal(activeButton.disabled, false)
assert.equal(processingButton.disabled, true)
assert.equal(failedButton.disabled, true)
await act(async () => activeButton.click())
assert.deepEqual(changes, [['asset-20260401123823-6d4x2']])
assert.equal(activeButton.getAttribute('aria-pressed'), 'true')
```

再增加独立用例保护：达到 9 个时未选素材禁用、已选素材仍可取消；请求失败显示重试按钮；空列表显示
`/assets` 入口；点击下一页以 `page=2&page_size=12` 加载并保留 `selectedIds`。

- [ ] **Step 2: 运行测试并确认组件不存在**

Run: `cd web && bun test src/features/video-generation/components/__tests__/asset-picker.test.tsx`

Expected: FAIL，无法导入 `asset-picker`。

- [ ] **Step 3: 实现素材选择器**

组件接口固定为：

```ts
type AssetPickerProps = {
  selectedIds: string[]
  limit: number
  onChange: (ids: string[]) => void
}
```

实现要求：

```ts
const ASSET_PAGE_SIZE = 12
const assetsQuery = useQuery({
  queryKey: ['video-generation', 'role-assets', page],
  queryFn: () => listAssets({ page, pageSize: ASSET_PAGE_SIZE }),
})

function toggleAsset(asset: Asset) {
  if (asset.status !== 'active') return
  if (props.selectedIds.includes(asset.id)) {
    props.onChange(props.selectedIds.filter((id) => id !== asset.id))
    return
  }
  if (props.selectedIds.length >= props.limit) return
  props.onChange([...props.selectedIds, asset.id])
}
```

每个素材使用原生 `button type="button"`，设置 `aria-pressed`、明确的可访问名称和固定缩略图比例。
`Active` 以外状态禁用并显示 Badge；图片 `onError` 后显示图标占位。列表使用响应式网格，素材 ID 允许
换行但不能撑破卡片。底部提供上一页、页码、下一页；加载中显示 Skeleton；错误态显示重试；空态
提供 `<a href="/assets">`，不在选择器内创建素材。

- [ ] **Step 4: 运行选择器测试和类型检查**

Run: `cd web && bun test src/features/video-generation/components/__tests__/asset-picker.test.tsx && bun run typecheck`

Expected: PASS。

- [ ] **Step 5: 提交素材选择器**

```bash
git add web/src/features/video-generation/components/asset-picker.tsx web/src/features/video-generation/components/__tests__/asset-picker.test.tsx
git commit -m "feat(video): add role asset picker"
```

### Task 5: 集成图片来源切换和动态媒体编辑器

**Files:**
- Modify: `web/src/features/video-generation/components/reference-media-editor.tsx`
- Create: `web/src/features/video-generation/components/reference-image-editor.tsx`
- Create: `web/src/features/video-generation/components/__tests__/reference-image-editor.test.tsx`
- Modify: `web/src/features/video-generation/index.tsx`

- [ ] **Step 1: 写模式切换和模型切换失败测试**

渲染受控 `ReferenceImageEditor`，基础版 2.0 使用 `source="url"`。点击“素材库”后断言
`onSourceChange('asset')`；以 `source="asset"` 重新渲染后能看到 `AssetPicker`。把 model 改为
`doubao-seedance-2-5-260628` 后断言组件自动调用 `onSourceChange('url')`，并且素材按钮为禁用状态。

再覆盖键盘可操作性与 `aria-pressed`，确保两个模式按钮的选中状态和视觉状态一致。

- [ ] **Step 2: 运行测试并确认组件不存在**

Run: `cd web && bun test src/features/video-generation/components/__tests__/reference-image-editor.test.tsx`

Expected: FAIL，无法导入 `reference-image-editor`。

- [ ] **Step 3: 让普通媒体编辑器接收动态能力**

把 `ReferenceMediaEditorProps` 改为：

```ts
type ReferenceMediaEditorProps = {
  kind: MediaKind
  values: string[]
  limit: number
  disabled?: boolean
  onChange: (values: string[]) => void
}
```

删除对静态 `VIDEO_MEDIA_LIMITS` 的依赖。计数、新增按钮使用 `props.limit`；禁用时所有输入、删除和
新增按钮均设 `disabled`，容器增加 `aria-disabled`，但仍显示当前内容和限制。

- [ ] **Step 4: 实现 ReferenceImageEditor**

组件接口固定为：

```ts
type ReferenceImageEditorProps = {
  model: string
  source: VideoImageSource
  imageUrls: string[]
  assetIds: string[]
  imageLimit: number
  onSourceChange: (source: VideoImageSource) => void
  onImageUrlsChange: (urls: string[]) => void
  onAssetIdsChange: (ids: string[]) => void
}
```

使用项目现有 `ToggleGroup`：

```tsx
<ToggleGroup
  value={[props.source]}
  onValueChange={(values) => {
    const next = values.find((value) => value !== props.source)
    if (next === 'url' || (next === 'asset' && roleAssetsSupported)) {
      props.onSourceChange(next)
    }
  }}
  variant='outline'
  className='w-full'
>
  <ToggleGroupItem value='url' className='flex-1'>
    {t('Public URLs')}
  </ToggleGroupItem>
  <ToggleGroupItem value='asset' className='flex-1' disabled={!roleAssetsSupported}>
    {t('Asset library')}
  </ToggleGroupItem>
</ToggleGroup>
```

`source === 'url'` 时渲染图片 `ReferenceMediaEditor`，否则渲染 `AssetPicker`。用 `useEffect` 检测
`source === 'asset' && !supportsRoleAssets(model)`，调用 `onSourceChange('url')` 并提示当前模型不支持
角色素材。

- [ ] **Step 5: 在视频页接线并实现清空规则**

在 `index.tsx` 监听 `model`、`imageSource` 和 `assetIds`，计算：

```ts
const model = form.watch('model')
const imageSource = form.watch('imageSource')
const assetIds = form.watch('assetIds')
const mediaLimits = getVideoMediaLimits(model)

function changeImageSource(next: VideoImageSource) {
  if (next === imageSource) return
  if (next === 'asset') {
    form.setValue('media.images', [], { shouldValidate: true })
    form.setValue('media.videos', [], { shouldValidate: true })
  } else {
    form.setValue('assetIds', [], { shouldValidate: true })
  }
  form.setValue('imageSource', next, { shouldValidate: true })
}
```

参考媒体区域改成一个全宽 `ReferenceImageEditor`，下方是视频和音频两列。素材模式给视频编辑器传
`disabled`，音频保持可编辑。页头三个 Badge 改为 `mediaLimits.images/videos/audios`，标题不再硬编码
`Seedance 2.0`。所有 `setValue` 都触发必要校验，请求预览直接反映当前模式。

- [ ] **Step 6: 运行组件、请求和默认值回归测试**

Run: `cd web && bun test src/features/video-generation/__tests__ src/features/video-generation/components/__tests__`

Expected: PASS；模式切换、请求预览基础逻辑和既有任务功能测试均通过。

- [ ] **Step 7: 提交页面集成**

```bash
git add web/src/features/video-generation/components/reference-media-editor.tsx web/src/features/video-generation/components/reference-image-editor.tsx web/src/features/video-generation/components/__tests__/reference-image-editor.test.tsx web/src/features/video-generation/index.tsx
git commit -m "feat(video): integrate asset library selection"
```

### Task 6: 完成七语种文案

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 加载并遵循 `i18n-translate` skill**

实施者必须完整读取 `.agents/skills/i18n-translate/SKILL.md`，用该 skill 要求的临时脚本更新七个
locale，不手工只改单个语言。

- [ ] **Step 2: 扫描本功能新增键并写入全部 locale**

至少覆盖以下英文源键，复用已有键时不重复创建：

```text
Reference media
Public URLs
Asset library
Role assets are available only for the base Seedance 2.0 model.
Only active assets can be selected.
Selected {{count}} of {{limit}}
Open asset library
Retry loading assets
Page {{page}} of {{total}}
{{kind}} cannot exceed {{count}} items
Role assets cannot exceed {{count}} items
Enter a prompt or at least one reference
This model does not support role assets
Select asset {{id}}
```

简体中文使用项目业务术语“素材库、角色素材、参考图片、参考视频、参考音频”；其他语言提供真实
翻译，不把英文值原样复制为占位。

- [ ] **Step 3: 运行 i18n 检查和相关测试**

Run: `cd web && bun run i18n:sync && bun run typecheck`

Expected: 命令成功，七个 locale 都包含新增键，TypeScript 无错误。若同步命令会产生无关大范围改动，
撤销无关生成结果，只保留本功能键，并重新运行检查。

- [ ] **Step 4: 提交翻译**

```bash
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat(i18n): translate video asset picker"
```

### Task 7: 完整验证与浏览器验收

**Files:**
- Verify only; only fix files already in this plan if checks expose a defect.

- [ ] **Step 1: 运行后端定向测试**

Run: `go test ./middleware ./service ./model -run 'Test.*Asset|TestVideoAssetRouting' -count=1`

Expected: PASS。

- [ ] **Step 2: 运行前端功能测试**

Run: `cd web && bun test src/features/video-generation/__tests__ src/features/video-generation/components/__tests__ src/features/assets/__tests__`

Expected: PASS。

- [ ] **Step 3: 运行静态检查和生产构建**

Run: `cd web && bun run typecheck && bun run lint && bun run build`

Expected: 三条命令均以退出码 0 完成；涉及文件没有 lint error。

- [ ] **Step 4: 启动开发服务并用 Playwright 验收桌面端**

若 `3000` 已被当前项目占用，复用现有服务；否则从 `web/` 运行 `bun run dev -- --port 3000`。使用已登录
会话访问 `http://localhost:3000/video-generation`，验证：

1. Seedance 2.0 显示 `9/3/3`，素材库可用。
2. 切到素材库后普通图片和参考视频被清空，音频保留。
3. `Active` 可选择，其他状态禁用；选择计数和 9 个上限正确。
4. 请求 JSON 包含按选择顺序排列的 `asset://asset-*`，没有普通图片 URL 或视频 URL。
5. 切到 Seedance 2.5 后退出素材模式并显示 `30/10/10`。
6. 空状态、失败重试、素材图片加载失败占位和素材库入口可操作。

- [ ] **Step 5: 验收移动端和布局稳定性**

使用约 `390x844` 视口检查分段控件、素材网格、长素材 ID、分页、URL 编辑器和请求预览均不横向溢出
或互相遮挡。保存桌面和移动端截图，并检查素材缩略图确实有非空像素内容；不提交真实付费生成任务。

- [ ] **Step 6: 最终差异和工作区检查**

Run: `git diff --check && git status --short && git log --oneline -8`

Expected: `git diff --check` 无输出；工作区只包含本计划明确允许的改动，提交历史包含各任务提交。

若验证阶段暴露缺陷，回到对应任务补失败测试和最小修复，重新运行该任务及本任务的检查，并使用对应
任务中列出的精确文件集合提交，禁止使用 `git add -A`。

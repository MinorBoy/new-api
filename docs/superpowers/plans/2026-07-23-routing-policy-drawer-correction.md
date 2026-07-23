# Routing Policy Drawer Correction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the incorrect super-resolution routing contract and make the routing-policy drawer use accurate duration semantics, API-backed group selection, project-themed dropdowns, and consistent primary selected states.

**Architecture:** Keep the existing capability matcher and policy persistence model, but reduce `Constraints` to externally observable request capabilities only. The frontend continues to use React Hook Form, Zod, React Query, and the shared Base UI controls; group data comes from the existing `/api/group/` endpoint and is normalized before it reaches a non-free-form Combobox.

**Tech Stack:** Go 1.22+, GORM v2, `common.*` JSON wrappers, testify, React 19, TypeScript, React Query, React Hook Form, Zod, Base UI, Tailwind CSS, Bun, project i18n scripts, in-app browser.

**Design Specs:**
- `docs/superpowers/specs/2026-07-23-routing-policy-drawer-semantics-design.md`
- `docs/superpowers/specs/2026-07-23-seedance-capability-routing-design.md`

---

## File Map

**Create**

- `web/default/scripts/add-missing-keys.mjs`: temporary script required by the project i18n workflow; atomically applies the new drawer translations to all six locales, then is deleted before commit.

**Modify**

- `pkg/modelrouting/types.go`: remove `generation_resolution` and `upscaled` from the public domain/API/cache contract.
- `pkg/modelrouting/validate.go`: remove the obsolete upscale validation code and branches.
- `pkg/modelrouting/validate_test.go`: remove invalid-upscale cases while retaining all observable capability validation.
- `pkg/modelrouting/match_test.go`: model an internally upscaled provider target as an ordinary `1080p` output target.
- `service/routing_policy.go`: stop normalizing the removed generation-resolution field.
- `service/routing_policy_test.go`: prove legacy JSON is accepted and newly saved constraints omit obsolete properties.
- `model/routing_policy_cache_test.go`: prove cache loading ignores obsolete JSON properties.
- `e2e/seedance_capability_routing_e2e_test.go`: remove obsolete fixture arguments and rename assertions to describe observable 1080p output.
- `web/default/src/features/model-routing/types.ts`: remove the obsolete form/API fields, validation, defaults, and serialization paths; add group normalization.
- `web/default/tests/model-routing-types.test.ts`: cover reduced constraint serialization, legacy response parsing, and group normalization.
- `web/default/src/features/model-routing/api.ts`: add the existing admin group endpoint to the feature API.
- `web/default/src/features/model-routing/query-keys.ts`: add a stable React Query key for groups.
- `web/default/src/features/model-routing/components/routing-policy-drawer.tsx`: load groups, render retryable states, replace native selects, and validate the selected group.
- `web/default/src/features/model-routing/components/route-target-editor.tsx`: remove the resolution-mode area, correct the duration label, and apply semantic primary selected styling.
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`: receive translated drawer keys only through the temporary script and `bun run i18n:sync`.
- `docs/superpowers/plans/2026-07-23-seedance-capability-routing.md`: remove the superseded `upscaled`/`generation_resolution` instructions and browser acceptance steps.

**Verify Without Editing**

- `web/default/src/components/ui/select.tsx`: shared themed Select contract.
- `web/default/src/components/ui/combobox.tsx`: shared searchable Combobox contract.
- `web/default/src/components/ui/toggle.tsx`: Base UI selected state exposes `data-[state=on]` and `aria-pressed`.
- `controller/group.go`: `/api/group/` returns `ApiResponse<string[]>`.

---

### Task 1: Remove the Backend Super-Resolution Contract

**Files:**
- Modify: `pkg/modelrouting/types.go`
- Modify: `pkg/modelrouting/validate.go`
- Modify: `pkg/modelrouting/validate_test.go`
- Modify: `pkg/modelrouting/match_test.go`
- Modify: `service/routing_policy.go`
- Modify: `service/routing_policy_test.go`
- Modify: `model/routing_policy_cache_test.go`

- [ ] **Step 1: Write the legacy-JSON compatibility regression tests**

Add this test to `service/routing_policy_test.go`. It must use the project JSON wrappers and prove both halves of the compatibility contract:

```go
func TestRoutingConstraintsIgnoreLegacyUpscaleProperties(t *testing.T) {
	legacy := `{
		"output_resolutions":["1080p"],
		"generation_resolution":"720p",
		"upscaled":true,
		"durations":{"min":4,"max":15},
		"aspect_ratios":["16:9"],
		"reference_limits":{"images":4,"videos":3,"audios":1},
		"supports_real_person":true
	}`

	var constraints modelrouting.Constraints
	require.NoError(t, common.UnmarshalJsonStr(legacy, &constraints))
	assert.Equal(t, []string{"1080p"}, constraints.OutputResolutions)

	encoded, err := common.Marshal(constraints)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "generation_resolution")
	assert.NotContains(t, string(encoded), "upscaled")
}
```

Extend `TestSaveRoutingPolicyNormalizesAndPublishesCompleteReplacement` with persisted and API-view assertions:

```go
var persisted model.RouteTarget
require.NoError(t, model.DB.Where("policy_id = ?", saved.ID).First(&persisted).Error)
assert.NotContains(t, persisted.Constraints, "generation_resolution")
assert.NotContains(t, persisted.Constraints, "upscaled")

encodedView, err := common.Marshal(saved)
require.NoError(t, err)
assert.NotContains(t, string(encodedView), "generation_resolution")
assert.NotContains(t, string(encodedView), "upscaled")
```

Add `TestRoutingPolicyCacheLoadsLegacyUpscaleProperties` to `model/routing_policy_cache_test.go` by inserting a target with the same legacy JSON, calling `model.InitRoutingPolicyCache()`, and asserting the loaded target matches a `1080p` fact but not a `720p` fact:

```go
snapshot, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
require.True(t, ok)

matched := modelrouting.Evaluate(snapshot, matchingFactsForResolution("1080p"))
assert.Contains(t, matched.CompatibleByChannel, 11)

notMatched := modelrouting.Evaluate(snapshot, matchingFactsForResolution("720p"))
assert.NotContains(t, notMatched.CompatibleByChannel, 11)
```

Use an existing local fact fixture if one is present; otherwise define this test-local helper in the same file:

```go
func matchingFactsForResolution(resolution string) modelrouting.Facts {
	return modelrouting.Facts{
		OutputResolution: resolution,
		DurationSeconds:  10,
		AspectRatio:      "16:9",
	}
}
```

- [ ] **Step 2: Run the new tests and confirm the reduced contract is not implemented**

```powershell
go test ./service ./model -run 'TestRoutingConstraintsIgnoreLegacyUpscaleProperties|TestRoutingPolicyCacheLoadsLegacyUpscaleProperties' -count=1
```

Expected: FAIL because marshaling the current typed structure still emits `upscaled` and the cache fixture still exposes the obsolete fields.

- [ ] **Step 3: Reduce the domain type and validation surface**

Replace `Constraints` in `pkg/modelrouting/types.go` with exactly this structure:

```go
type Constraints struct {
	OutputResolutions  []string           `json:"output_resolutions"`
	Durations          DurationConstraint `json:"durations"`
	AspectRatios       []string           `json:"aspect_ratios,omitempty"`
	ReferenceLimits    ReferenceLimits    `json:"reference_limits"`
	SupportsRealPerson *bool              `json:"supports_real_person"`
}
```

Delete `ValidationInvalidUpscale` from `pkg/modelrouting/validate.go`. Delete the complete branch beginning with `generationResolution :=` in `validateConstraints`; validation must return `nil` immediately after reference-limit validation.

Delete this normalization line from `service/routing_policy.go`:

```go
target.Constraints.GenerationResolution = strings.ToLower(strings.TrimSpace(target.Constraints.GenerationResolution))
```

- [ ] **Step 4: Correct the matcher and validation tests**

In `pkg/modelrouting/match_test.go`, rename `TestEvaluateTargetsUsesOutputResolutionForUpscale` to `TestEvaluateTargetsUsesConfiguredOutputResolution` and reduce the target constraint to:

```go
Constraints: modelrouting.Constraints{
	OutputResolutions:  []string{"1080p"},
	Durations:          modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)},
	AspectRatios:       []string{"16:9", "9:16"},
	ReferenceLimits:    modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
	SupportsRealPerson: &supportsRealPerson,
},
```

Keep the assertions that `1080p` matches and `720p` does not. In `pkg/modelrouting/validate_test.go`, delete only the three cases named `upscale has multiple outputs`, `upscale generation equals output`, and `native target has generation resolution`; retain every resolution, duration, ratio, material-limit, default-route, and overlap case.

- [ ] **Step 5: Format and run the backend regression suite**

```powershell
gofmt -w pkg/modelrouting/types.go pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go pkg/modelrouting/match_test.go service/routing_policy.go service/routing_policy_test.go model/routing_policy_cache_test.go
go test ./pkg/modelrouting ./service ./model -run 'Routing|Constraints|ValidatePolicy|EvaluateTargets' -count=1
```

Expected: PASS. `rg -n 'GenerationResolution|Upscaled|ValidationInvalidUpscale' pkg/modelrouting service/routing_policy.go` returns no matches.

- [ ] **Step 6: Commit the backend contract correction**

```powershell
git add pkg/modelrouting service/routing_policy.go service/routing_policy_test.go model/routing_policy_cache_test.go
git commit -m "fix(routing): remove super-resolution contract"
```

---

### Task 2: Remove Obsolete Frontend Fields and Serialization

**Files:**
- Modify: `web/default/src/features/model-routing/types.ts`
- Modify: `web/default/tests/model-routing-types.test.ts`

- [ ] **Step 1: Rewrite the frontend contract tests before implementation**

Remove `generation_resolution` and `upscaled` from every form fixture in `web/default/tests/model-routing-types.test.ts`. Replace the invalid-upscale test with a legacy-response compatibility test:

```ts
test('ignores legacy super-resolution properties in API responses', () => {
  const response = routingPolicyResponseSchema.parse({
    success: true,
    data: {
      id: 7,
      group_name: '分组A',
      model: 'doubao-seedance-2-0-260128',
      enabled: true,
      defaults: {
        output_resolution: '1080p',
        duration_seconds: 10,
        aspect_ratio: '16:9',
      },
      targets: [
        {
          id: 21,
          channel_id: 12,
          channel_name: 'A1_copy',
          name: 'Provider 1080p output',
          upstream_model: 'lec-feituo-seedance-2-0-my-upscaled-1080p',
          target_priority: 110,
          enabled: true,
          constraints: {
            output_resolutions: ['1080p'],
            generation_resolution: '720p',
            upscaled: true,
            durations: { min: 4, max: 15 },
            aspect_ratios: [],
            reference_limits: { images: 4, videos: 3, audios: 1 },
            supports_real_person: true,
          },
        },
      ],
      created_at: 1,
      updated_at: 2,
    },
  })

  expect(response.data.targets[0]?.constraints).toEqual({
    output_resolutions: ['1080p'],
    durations: { min: 4, max: 15 },
    aspect_ratios: [],
    reference_limits: { images: 4, videos: 3, audios: 1 },
    supports_real_person: true,
  })
})
```

Strengthen the write request test with:

```ts
expect(payload.targets[0]?.constraints).not.toHaveProperty('upscaled')
expect(payload.targets[0]?.constraints).not.toHaveProperty(
  'generation_resolution'
)
```

- [ ] **Step 2: Run the contract test and confirm it fails**

```powershell
bun test tests/model-routing-types.test.ts
```

Run from `web/default`. Expected: FAIL because the response schema still returns `upscaled` and the form schema still expects both obsolete fields.

- [ ] **Step 3: Reduce all frontend schemas and conversion paths**

In `types.ts`, make `routeTargetFormSchema` a plain object without the upscale `superRefine`:

```ts
export const routeTargetFormSchema = z.object({
  id: z.number().int().positive().optional(),
  channel_id: z.number().int().positive('Channel is required'),
  channel_name: z.string(),
  name: z.string().trim().min(1, 'Target name is required'),
  upstream_model: z.string().trim().min(1, 'Upstream model is required'),
  target_priority: z.number().int(),
  enabled: z.boolean(),
  output_resolutions: z
    .array(resolutionSchema)
    .min(1, 'At least one output resolution is required'),
  durations: durationConstraintFormSchema,
  aspect_ratios: z.array(aspectRatioSchema),
  reference_limits: referenceLimitsSchema,
  supports_real_person: z.enum(['unknown', 'yes', 'no']),
})
```

Replace `routeConstraintsApiSchema` with:

```ts
export const routeConstraintsApiSchema = z.object({
  output_resolutions: z.array(resolutionSchema).min(1),
  durations: durationConstraintApiSchema,
  aspect_ratios: z.array(aspectRatioSchema).default([]),
  reference_limits: referenceLimitsSchema,
  supports_real_person: z.boolean().nullable(),
})
```

Delete both obsolete properties from `createEmptyTarget`, `toWriteRequest`, and `fromPolicyResponse`. Do not add replacement note, mode, or generated-resolution fields.

- [ ] **Step 4: Run the focused frontend tests and static search**

```powershell
bun test tests/model-routing-types.test.ts
rg -n 'generation_resolution|upscaled' src/features/model-routing tests/model-routing-types.test.ts
```

Run from `web/default`. Expected: test PASS and `rg` returns no matches.

- [ ] **Step 5: Commit the frontend contract correction**

```powershell
git add web/default/src/features/model-routing/types.ts web/default/tests/model-routing-types.test.ts
git commit -m "fix(web): remove super-resolution routing fields"
```

---

### Task 3: Add API-Backed Group Selection

**Files:**
- Modify: `web/default/src/features/model-routing/types.ts`
- Modify: `web/default/src/features/model-routing/api.ts`
- Modify: `web/default/src/features/model-routing/query-keys.ts`
- Modify: `web/default/src/features/model-routing/components/routing-policy-drawer.tsx`
- Modify: `web/default/tests/model-routing-types.test.ts`

- [ ] **Step 1: Write deterministic group normalization tests**

Import `normalizeRoutingGroups` in `model-routing-types.test.ts` and add:

```ts
test('normalizes, deduplicates, sorts, and filters routing groups', () => {
  expect(
    normalizeRoutingGroups(
      [' group-b ', 'auto', 'Group-A', 'group-b', '', 'AUTO'],
      ''
    )
  ).toEqual(['Group-A', 'group-b'])
})

test('preserves the current group when it is missing from the API', () => {
  expect(normalizeRoutingGroups(['default', 'vip'], 'legacy-group')).toEqual([
    'default',
    'legacy-group',
    'vip',
  ])
})
```

- [ ] **Step 2: Run the tests and confirm the helper is missing**

```powershell
bun test tests/model-routing-types.test.ts
```

Expected: FAIL because `normalizeRoutingGroups` is not exported.

- [ ] **Step 3: Implement group response parsing and normalization**

Add to `types.ts` after `apiSuccessSchema`:

```ts
export const routingGroupResponseSchema = apiSuccessSchema.extend({
  data: z.array(z.string()),
})

export function normalizeRoutingGroups(
  groups: string[],
  currentGroup: string
): string[] {
  const normalized = new Map<string, string>()
  for (const value of groups) {
    const group = value.trim()
    if (group === '' || group.toLowerCase() === 'auto') {
      continue
    }
    const key = group.toLocaleLowerCase()
    if (!normalized.has(key)) {
      normalized.set(key, group)
    }
  }

  const current = currentGroup.trim()
  if (current !== '' && current.toLowerCase() !== 'auto') {
    normalized.set(current.toLocaleLowerCase(), current)
  }

  return [...normalized.values()].sort((left, right) =>
    left.localeCompare(right, undefined, { sensitivity: 'base' })
  )
}
```

Add to `api.ts`:

```ts
export async function listRoutingGroups() {
  const response = await api.get('/api/group/')
  return routingGroupResponseSchema.parse(response.data)
}
```

Import `routingGroupResponseSchema` from `./types`. Add to `query-keys.ts`:

```ts
groups: () => [...routingPolicyQueryKeys.all, 'groups'] as const,
```

- [ ] **Step 4: Load groups only while the drawer is open**

In `routing-policy-drawer.tsx`, import `Combobox`, `listRoutingGroups`, and `normalizeRoutingGroups`. Add the query beside `candidatesQuery`:

```ts
const groupsQuery = useQuery({
  queryKey: routingPolicyQueryKeys.groups(),
  queryFn: listRoutingGroups,
  enabled: props.open,
})
const groupOptions = normalizeRoutingGroups(
  groupsQuery.data?.data ?? [],
  groupName
).map((group) => ({ value: group, label: group }))
```

Replace the free-form `Input` with the shared searchable control:

```tsx
<Combobox
  options={groupOptions}
  value={field.value}
  onValueChange={(value) => field.onChange(value ?? '')}
  placeholder={
    groupsQuery.isLoading ? t('Loading groups...') : t('Select group')
  }
  searchPlaceholder={t('Search groups...')}
  emptyText={t('No groups available')}
  allowCustomValue={false}
  className='w-full'
/>
```

Below the control, render a retryable error without clearing `field.value`:

```tsx
{groupsQuery.isError && (
  <div className='flex items-center justify-between gap-2'>
    <p className='text-destructive text-xs'>{t('Failed to load groups')}</p>
    <Button
      type='button'
      variant='ghost'
      size='sm'
      onClick={() => void groupsQuery.refetch()}
    >
      {t('Retry')}
    </Button>
  </div>
)}
```

Before `saveMutation.mutate(values)` in `handleSubmit`, reject new free-form or stale values after a successful load:

```ts
const selectableGroups = normalizeRoutingGroups(
  groupsQuery.data?.data ?? [],
  isEditing || props.copyingPolicy ? values.group_name : ''
)
if (!groupsQuery.isSuccess || !selectableGroups.includes(values.group_name)) {
  form.setError('group_name', {
    type: 'validate',
    message: groupsQuery.isError
      ? 'Failed to load groups'
      : 'Group is required',
  })
  return
}
```

This preserves an unavailable group when editing or copying, but a new policy can only select a currently returned group.

- [ ] **Step 5: Run unit and type checks**

```powershell
bun test tests/model-routing-types.test.ts
bun run typecheck
```

Run from `web/default`. Expected: both exit 0.

- [ ] **Step 6: Commit group loading**

```powershell
git add web/default/src/features/model-routing/api.ts web/default/src/features/model-routing/query-keys.ts web/default/src/features/model-routing/types.ts web/default/src/features/model-routing/components/routing-policy-drawer.tsx web/default/tests/model-routing-types.test.ts
git commit -m "feat(web): load routing policy groups"
```

---

### Task 4: Replace Native Selects and Remove the Resolution-Mode UI

**Files:**
- Modify: `web/default/src/features/model-routing/components/routing-policy-drawer.tsx`
- Modify: `web/default/src/features/model-routing/components/route-target-editor.tsx`

- [ ] **Step 1: Replace drawer-native selects with the shared Select**

Delete the `NativeSelect` import. Import:

```ts
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
```

Replace the canonical model field with:

```tsx
<Select value={field.value} onValueChange={field.onChange}>
  <FormControl>
    <SelectTrigger className='w-full font-mono text-xs'>
      <SelectValue />
    </SelectTrigger>
  </FormControl>
  <SelectContent align='start'>
    {CANONICAL_SEEDANCE_MODELS.map((model) => (
      <SelectItem key={model} value={model} className='font-mono text-xs'>
        {model}
      </SelectItem>
    ))}
  </SelectContent>
</Select>
```

Replace the default resolution field with:

```tsx
<Select value={field.value} onValueChange={field.onChange}>
  <FormControl>
    <SelectTrigger className='w-full'>
      <SelectValue />
    </SelectTrigger>
  </FormControl>
  <SelectContent align='start'>
    {OUTPUT_RESOLUTIONS.map((resolution) => (
      <SelectItem key={resolution} value={resolution}>
        {resolution}
      </SelectItem>
    ))}
  </SelectContent>
</Select>
```

Replace the default aspect-ratio field with:

```tsx
<Select value={field.value} onValueChange={field.onChange}>
  <FormControl>
    <SelectTrigger className='w-full'>
      <SelectValue />
    </SelectTrigger>
  </FormControl>
  <SelectContent align='start'>
    {ASPECT_RATIOS.map((ratio) => (
      <SelectItem key={ratio} value={ratio}>
        {ratio}
      </SelectItem>
    ))}
  </SelectContent>
</Select>
```

Do not introduce duplicate option arrays.

- [ ] **Step 2: Delete the complete obsolete editor section**

In `route-target-editor.tsx`, delete:

- the `NativeSelect` import;
- the `targets.${index}.upscaled` `FormField` labeled `Resolution mode`;
- the conditional `targets.${index}.generation_resolution` field;
- every `Native`, `Upscaled`, `Generation resolution`, and `Select resolution` call site.

The output-resolution checkbox group remains the only resolution capability editor.

- [ ] **Step 3: Verify no native routing dropdown remains**

```powershell
rg -n 'NativeSelect|Resolution mode|Generation resolution|Upscaled|t\(.Native.' src/features/model-routing
bun run typecheck
```

Run from `web/default`. Expected: `rg` returns no matches and typecheck exits 0.

- [ ] **Step 4: Commit the themed dropdown conversion**

```powershell
git add web/default/src/features/model-routing/components/routing-policy-drawer.tsx web/default/src/features/model-routing/components/route-target-editor.tsx
git commit -m "fix(web): theme routing policy dropdowns"
```

---

### Task 5: Correct Duration Copy and Primary Selected States

**Files:**
- Modify: `web/default/src/features/model-routing/components/route-target-editor.tsx`

- [ ] **Step 1: Add one scoped selected-state class contract**

Import `cn` from `@/lib/utils` and add this module constant below `numericValue`:

```ts
const ROUTING_SELECTED_CLASS =
  'data-[state=on]:border-primary data-[state=on]:bg-primary/15 data-[state=on]:text-primary dark:data-[state=on]:bg-primary/20'
```

Apply it with `cn('flex-1', ROUTING_SELECTED_CLASS)` to all duration-mode and real-person `ToggleGroupItem` controls. Apply `className={ROUTING_SELECTED_CLASS}` to every aspect-ratio `ToggleGroupItem`.

- [ ] **Step 2: Highlight selected resolution labels with semantic tokens**

Replace the output-resolution label class with:

```tsx
className={cn(
  'flex min-h-9 cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
  field.value.includes(resolution) &&
    'border-primary bg-primary/10 text-primary dark:bg-primary/15'
)}
```

Do not change shared Checkbox or Switch internals; their checked states already use project semantic tokens.

- [ ] **Step 3: Correct the duration label only in the routing editor**

Change:

```tsx
<FormLabel>{t('Duration')}</FormLabel>
```

to:

```tsx
<FormLabel>{t('Allowed generation durations')}</FormLabel>
```

Keep `Duration values`, `Minimum`, `Maximum`, and the numeric input `aria-label` unchanged because those strings describe the nested controls rather than the section meaning.

- [ ] **Step 4: Run formatting, type, and lint checks**

```powershell
bun run format
bun run typecheck
bun run lint
```

Run from `web/default`. Expected: all exit 0; selected routing controls contain `data-[state=on]` primary classes and no negative letter spacing.

- [ ] **Step 5: Commit the semantic styling correction**

```powershell
git add web/default/src/features/model-routing/components/route-target-editor.tsx
git commit -m "fix(web): clarify routing target capability controls"
```

---

### Task 6: Add Six-Locale Drawer Copy Through the Required Script

**Files:**
- Create then delete: `web/default/scripts/add-missing-keys.mjs`
- Modify through script: `web/default/src/i18n/locales/en.json`
- Modify through script: `web/default/src/i18n/locales/zh.json`
- Modify through script: `web/default/src/i18n/locales/fr.json`
- Modify through script: `web/default/src/i18n/locales/ja.json`
- Modify through script: `web/default/src/i18n/locales/ru.json`
- Modify through script: `web/default/src/i18n/locales/vi.json`

- [ ] **Step 1: Run the existing sync report before locale changes**

```powershell
bun run i18n:sync
Get-Content -Raw 'src/i18n/locales/_reports/_sync-report.json'
```

Run from `web/default`. Expected: the report identifies the newly referenced drawer keys as missing until the script is applied.

- [ ] **Step 2: Create the sanctioned locale update script**

Create `scripts/add-missing-keys.mjs` with this complete content:

```js
import fs from 'node:fs/promises'
import path from 'node:path'

const LOCALES_DIR = path.resolve('src/i18n/locales')

function stableStringify(value) {
  return JSON.stringify(value, null, 2) + '\n'
}

const newKeys = {
  en: {
    'Allowed generation durations': 'Allowed generation durations',
    'Failed to load groups': 'Failed to load groups',
    'Loading groups...': 'Loading groups...',
    'No groups available': 'No groups available',
    'Search groups...': 'Search groups...',
    'Select group': 'Select group',
  },
  zh: {
    'Allowed generation durations': '允许生成时长',
    'Failed to load groups': '分组加载失败',
    'Loading groups...': '正在加载分组...',
    'No groups available': '暂无可用分组',
    'Search groups...': '搜索分组...',
    'Select group': '选择分组',
  },
  fr: {
    'Allowed generation durations': 'Durées de génération autorisées',
    'Failed to load groups': 'Échec du chargement des groupes',
    'Loading groups...': 'Chargement des groupes...',
    'No groups available': 'Aucun groupe disponible',
    'Search groups...': 'Rechercher des groupes...',
    'Select group': 'Sélectionner un groupe',
  },
  ja: {
    'Allowed generation durations': '許可する生成時間',
    'Failed to load groups': 'グループを読み込めませんでした',
    'Loading groups...': 'グループを読み込み中...',
    'No groups available': '利用可能なグループがありません',
    'Search groups...': 'グループを検索...',
    'Select group': 'グループを選択',
  },
  ru: {
    'Allowed generation durations': 'Допустимая длительность генерации',
    'Failed to load groups': 'Не удалось загрузить группы',
    'Loading groups...': 'Загрузка групп...',
    'No groups available': 'Нет доступных групп',
    'Search groups...': 'Поиск групп...',
    'Select group': 'Выберите группу',
  },
  vi: {
    'Allowed generation durations': 'Thời lượng tạo được phép',
    'Failed to load groups': 'Không thể tải nhóm',
    'Loading groups...': 'Đang tải nhóm...',
    'No groups available': 'Không có nhóm khả dụng',
    'Search groups...': 'Tìm kiếm nhóm...',
    'Select group': 'Chọn nhóm',
  },
}

const deleteKeys = [
  'Generation resolution',
  'Native targets cannot set a generation resolution',
  'Upscaled',
  'Upscaled targets require one distinct generation resolution',
]

async function main() {
  let totalChanged = 0

  for (const [locale, translations] of Object.entries(newKeys)) {
    const filePath = path.join(LOCALES_DIR, `${locale}.json`)
    const json = JSON.parse(await fs.readFile(filePath, 'utf8'))
    let changed = 0

    for (const [key, value] of Object.entries(translations)) {
      if (json.translation[key] === value) {
        continue
      }
      json.translation[key] = value
      changed++
    }

    for (const key of deleteKeys) {
      if (!Object.prototype.hasOwnProperty.call(json.translation, key)) {
        continue
      }
      delete json.translation[key]
      changed++
    }

    if (changed > 0) {
      json.translation = Object.fromEntries(
        Object.entries(json.translation).sort(([left], [right]) =>
          left.localeCompare(right)
        )
      )
      await fs.writeFile(filePath, stableStringify(json), 'utf8')
    }

    console.log(`${locale}: ${changed} translations changed`)
    totalChanged += changed
  }

  console.log(`Total: ${totalChanged} translations changed`)
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
```

The four deleted keys are routing-only after Task 4; `Native` itself is retained because other features use it.

- [ ] **Step 3: Apply, synchronize, and remove the temporary script**

```powershell
node scripts/add-missing-keys.mjs
bun run i18n:sync
Remove-Item -LiteralPath 'scripts/add-missing-keys.mjs'
```

Expected: all six locales contain the six keys, sorted consistently, and the temporary script is absent.

- [ ] **Step 4: Verify obsolete routing-only keys were safely removed**

Run:

```powershell
rg -n "t\('(Generation resolution|Upscaled|Native targets cannot set a generation resolution|Upscaled targets require one distinct generation resolution)'\)" src
Get-ChildItem src/i18n/locales -Filter '*.json' | ForEach-Object { rg -n '"(Generation resolution|Native targets cannot set a generation resolution|Upscaled|Upscaled targets require one distinct generation resolution)"' $_.FullName }
```

Expected: both searches return no matches. The pre-plan source audit already confirmed the four keys are routing-only after Task 4.

- [ ] **Step 5: Verify locale consistency and frontend quality**

```powershell
Get-ChildItem src/i18n/locales -Filter '*.json' | ForEach-Object { rg -n 'Allowed generation durations|Failed to load groups|Loading groups\.\.\.|No groups available|Search groups\.\.\.|Select group' $_.FullName }
bun run format:check
bun run copyright:check
bun run typecheck
bun run lint
bun run build
```

Expected: every command exits 0 and every new key appears once in each supported locale.

- [ ] **Step 6: Commit translations**

```powershell
git add web/default/src/i18n/locales
git commit -m "fix(i18n): clarify routing policy drawer copy"
```

---

### Task 7: Correct E2E Fixtures and Superseded Documentation

**Files:**
- Modify: `e2e/seedance_capability_routing_e2e_test.go`
- Modify: `docs/superpowers/plans/2026-07-23-seedance-capability-routing.md`

- [ ] **Step 1: Simplify the E2E target helper**

Change the helper signature from:

```go
func capabilityTarget(channelID int, upstreamModel string, priority int, resolutions []string, durations modelrouting.DurationConstraint, ratios []string, references modelrouting.ReferenceLimits, supportsRealPerson, upscaled bool, generationResolution string) service.RouteTargetWriteRequest
```

to:

```go
func capabilityTarget(channelID int, upstreamModel string, priority int, resolutions []string, durations modelrouting.DurationConstraint, ratios []string, references modelrouting.ReferenceLimits, supportsRealPerson bool) service.RouteTargetWriteRequest
```

Build constraints with only:

```go
Constraints: modelrouting.Constraints{
	OutputResolutions:  resolutions,
	Durations:          durations,
	AspectRatios:       ratios,
	ReferenceLimits:    references,
	SupportsRealPerson: common.GetPointer(supportsRealPerson),
},
```

Remove the trailing obsolete Boolean and string arguments from every call. The target using `lec-feituo-seedance-2-0-my-upscaled-1080p` must pass `[]string{"1080p"}` and no other resolution metadata.

- [ ] **Step 2: Rename observable-behavior test cases**

Rename `standard upscaled 1080` to `standard provider 1080 output`. Keep the request at `resolution: "1080p"` and keep the assertion that the provider target is selected. Do not add any request parameter or matcher assertion about upscaling.

- [ ] **Step 3: Remove superseded instructions from the original implementation plan**

In `docs/superpowers/plans/2026-07-23-seedance-capability-routing.md`:

- change the frozen contract from the old upscale rule to: `provider-internal generation details are not routing fields; targets match only output_resolutions`;
- delete `GenerationResolution` and `Upscaled` from every Go/TypeScript code block;
- replace the 1080 target matrix description with `output 1080p` only;
- remove browser steps that ask the administrator to configure `native` or `upscaled` modes;
- update the rollout checklist so the upstream name may describe internal processing, but the configured capability remains `1080p`.

Do not change unrelated routing architecture, retry, privacy, billing, or rollout decisions.

- [ ] **Step 4: Run E2E and documentation searches**

```powershell
gofmt -w e2e/seedance_capability_routing_e2e_test.go
go test ./e2e -run 'TestSeedanceCapabilityRouting' -count=1 -v
rg -n 'GenerationResolution|generation_resolution|Upscaled|upscaled=true|automatic upscale|自动超分' pkg/modelrouting service/routing_policy.go e2e/seedance_capability_routing_e2e_test.go web/default/src/features/model-routing web/default/tests/model-routing-types.test.ts docs/superpowers/specs/2026-07-23-seedance-capability-routing-design.md docs/superpowers/plans/2026-07-23-seedance-capability-routing.md
```

Expected: E2E PASS. `rg` may match the literal upstream model ID containing `upscaled`, and descriptive text saying that provider-internal processing is non-routing metadata; it must not find a field, request parameter, form mode, validation code, or configuration instruction.

- [ ] **Step 5: Commit corrected fixtures and documentation**

```powershell
git add e2e/seedance_capability_routing_e2e_test.go docs/superpowers/plans/2026-07-23-seedance-capability-routing.md
git commit -m "test(routing): correct provider resolution semantics"
```

---

### Task 8: Full Verification and Browser Acceptance

**Files:**
- Verify: all files changed by Tasks 1-7
- Do not commit: `artifacts/`

- [ ] **Step 1: Run the complete targeted backend suite**

```powershell
go test ./pkg/modelrouting ./model ./service ./middleware ./controller ./router ./relay/... ./e2e -count=1
```

Expected: PASS with no external paid request.

- [ ] **Step 2: Run the complete frontend suite**

```powershell
bun test tests/model-routing-types.test.ts
bun run format:check
bun run copyright:check
bun run typecheck
bun run lint
bun run build
```

Run from `web/default`. Expected: all commands exit 0.

- [ ] **Step 3: Verify the existing local QA server**

Open `http://127.0.0.1:3005/models/routing`. If it no longer serves the current worktree, start the backend and frontend on free ports and keep the frontend API URL aligned:

```powershell
go run . --port 3004
```

```powershell
$env:VITE_REACT_APP_SERVER_URL='http://127.0.0.1:3004'
bun run dev -- --port 3005
```

Expected: the routing page loads from the corrected branch without console or API errors.

- [ ] **Step 4: Verify desktop dark-theme behavior in the in-app browser**

At 1440x900, open an existing policy and verify:

1. Group is a searchable project-themed dropdown populated from `/api/group/`; `auto` is absent.
2. The current group remains visible when it is absent from a simulated latest group response.
3. Loading, empty, and failed group states are coherent; the failed state has a working Retry button.
4. Canonical model, default resolution, default aspect ratio, and channel controls use the same themed popup, hover, focus, and selected colors.
5. No `Native`, `Upscaled`, `Generation resolution`, or automatic-super-resolution area exists.
6. The target section label is `允许生成时长` under Chinese locale.
7. Resolution, duration mode, aspect ratio, real-person state, switches, and checkboxes use the semantic blue-accented primary state in dark mode.
8. Save, reopen, and confirm a target named `lec-feituo-seedance-2-0-my-upscaled-1080p` round-trips with only `output_resolutions: ["1080p"]`.

Capture `artifacts/routing-policy-drawer-corrected-desktop.png`; do not stage it.

- [ ] **Step 5: Verify 390x844 mobile layout**

Confirm the drawer scrolls independently, footer actions remain reachable, dropdown popups fit the viewport, long model IDs wrap or truncate without covering adjacent controls, selected-state backgrounds remain legible, and no horizontal overflow appears.

Capture `artifacts/routing-policy-drawer-corrected-mobile.png`; do not stage it.

- [ ] **Step 6: Review the final diff and untracked files**

```powershell
git status --short
git diff --check
git log --oneline -8
git diff 65c989b76..HEAD --stat
git diff 65c989b76..HEAD -- pkg/modelrouting service/routing_policy.go web/default/src/features/model-routing e2e/seedance_capability_routing_e2e_test.go docs/superpowers
```

Expected: no whitespace errors, only the intended backend/frontend/tests/docs changes, protected project and organization identifiers remain untouched, and `artifacts/` is the only untracked path.

- [ ] **Step 7: Record final verification without creating a redundant commit**

If all verification passes and `git status --short` shows only `?? artifacts/`, report the branch and commits. Do not create an empty verification commit and do not stage screenshots.

---

## Completion Criteria

- `modelrouting.Constraints`, API responses, cache snapshots, newly persisted JSON, frontend schemas, and form payloads contain no `upscaled` or `generation_resolution` property.
- Legacy JSON with those unknown properties still loads and routes solely by `output_resolutions`.
- A provider model whose ID mentions upscaling is configured as an ordinary `1080p` target and matches only a user `1080p` request.
- The drawer group field is API-backed, searchable, excludes `auto`, preserves a missing existing value, and exposes loading, empty, error, and retry states.
- Every dropdown in the drawer uses the shared themed Select/Combobox controls.
- The drawer says `允许生成时长` in Chinese, contains no super-resolution mode, and uses semantic primary selected states in light and dark themes.
- Backend, frontend, E2E, build, i18n, desktop, and mobile verification all pass.

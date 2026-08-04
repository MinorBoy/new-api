# 任务审计数据悬停预览与日志紧凑布局实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 为任务日志的三列审计数据增加完整 JSON 悬停预览和保留点击详情能力，同时只收紧任务日志与绘图日志的桌面表格密度。

**架构：** 任务审计单元格复用现有 Base UI `HoverCard`、`TaskAuditDataDialog` 和 JSON 格式化结果；悬停/聚焦负责快速预览，按压负责打开 Dialog。日志密度提取为用量日志模块内的稳定分类配置，由 `UsageLogsTable` 同时传给表头和数据行，不修改通用 DataTable 默认样式。

**技术栈：** React 19、TypeScript、TanStack Table、Base UI Preview Card、Tailwind CSS、Bun、Node test runner、happy-dom

**设计依据：** `docs/superpowers/specs/2026-08-04-task-audit-hover-preview-compact-logs-design.md`

---

## 文件职责

- `web/src/features/usage-logs/components/columns/task-logs-columns.tsx`：渲染三列审计数据入口，协调 Hover Card 与详情 Dialog 状态。
- `web/src/features/usage-logs/components/dialogs/task-audit-data-dialog.tsx`：提供稳定的审计数据格式化函数和现有完整详情 Dialog。
- `web/src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx`：保护三列入口、聚焦预览、完整 JSON、复制入口、点击详情和空数据降级。
- `web/src/features/usage-logs/lib/table-density.ts`：按日志分类提供稳定的桌面表格密度配置。
- `web/src/features/usage-logs/lib/__tests__/table-density.test.ts`：保护任务/绘图紧凑、普通日志不变的布局契约。
- `web/src/features/usage-logs/components/usage-logs-table.tsx`：把分类密度配置应用到表头和桌面数据行。

### 任务 1：为三列审计数据增加悬停与聚焦预览

**文件：**

- 修改：`web/src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx`
- 修改：`web/src/features/usage-logs/components/columns/task-logs-columns.tsx`
- 修改：`web/src/features/usage-logs/components/dialogs/task-audit-data-dialog.tsx`

- [ ] **步骤 1：先写聚焦预览和点击详情失败测试**

在现有测试中增加单个审计单元格挂载夹具，使用管理员的“请求数据”列渲染：

```tsx
const requestDataColumn = columns.find(
  (column) =>
    'accessorKey' in column && column.accessorKey === 'user_request_data'
)
assert.equal(typeof requestDataColumn?.cell, 'function')

const content = requestDataColumn.cell({ row: { original: log } } as never)
await act(async () => {
  root.render(<I18nextProvider i18n={i18n}>{content}</I18nextProvider>)
})
```

聚焦“查看”按钮后，要求 Preview Card 展示格式化后的完整 JSON 和可访问复制按钮：

```ts
const trigger = container.querySelector<HTMLButtonElement>(
  'button[title="View"]'
)
assert.ok(trigger)

await act(async () => trigger.focus())

const preview = document.querySelector('[data-slot="hover-card-content"]')
assert.ok(preview)
assert.match(preview.textContent ?? '', /"model": "seedance"/)
assert.ok(
  preview.querySelector('button[aria-label="Copy to clipboard"]')
)
```

点击同一按钮后要求出现标题为“Request Data”的 Dialog；再增加空字符串数据测试，断言只显示 `-`，不渲染按钮。

- [ ] **步骤 2：运行测试并确认按预期失败**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx
```

工作目录：`web/`

预期：失败，因为当前按钮没有 Preview Card，聚焦后不存在完整 JSON 预览。

- [ ] **步骤 3：共享格式化结果并实现 Hover Card**

将详情文件中的格式化函数导出：

```ts
export function formatTaskAuditData(data: unknown): string {
  if (typeof data === 'string') {
    try {
      return JSON.stringify(JSON.parse(data), null, 2)
    } catch {
      return data
    }
  }

  return JSON.stringify(data, null, 2) ?? ''
}
```

在 `TaskAuditDataCell` 中使用 `useMemo` 生成一次 `formattedData`，并增加受控 Hover Card。`trigger-press` 不打开预览，点击路径只打开现有 Dialog：

```tsx
const triggerId = useId()
const [previewOpen, setPreviewOpen] = useState(false)
const [dialogOpen, setDialogOpen] = useState(false)
const formattedData = useMemo(() => formatTaskAuditData(data), [data])
const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })

<HoverCard
  open={previewOpen}
  triggerId={triggerId}
  onOpenChange={(open, eventDetails) => {
    if (eventDetails.reason !== 'trigger-press') setPreviewOpen(open)
  }}
>
  <HoverCardTrigger
    id={triggerId}
    delay={250}
    closeDelay={150}
    render={
      <button
        type='button'
        className='text-foreground inline-flex max-w-full items-center gap-1 text-xs hover:underline focus-visible:underline'
        onClick={() => {
          setPreviewOpen(false)
          setDialogOpen(true)
        }}
        title={t('View')}
      />
    }
  >
    <Eye className='size-3 shrink-0' aria-hidden='true' />
    <span className='truncate'>{t('View')}</span>
  </HoverCardTrigger>
  <HoverCardContent
    align='start'
    className='w-[min(35rem,calc(100vw-2rem))] overflow-hidden p-0'
  >
    <div className='border-border flex h-10 items-center justify-between gap-3 border-b px-3'>
      <span className='truncate text-sm font-medium'>{t(title)}</span>
      <Button
        variant='ghost'
        size='sm'
        className='size-8 shrink-0 p-0'
        onClick={() => copyToClipboard(formattedData)}
        title={t('Copy to clipboard')}
        aria-label={t('Copy to clipboard')}
      >
        {copiedText === formattedData ? (
          <Check className='size-4 text-green-600' />
        ) : (
          <Copy className='size-4' />
        )}
      </Button>
    </div>
    <ScrollArea className='max-h-[min(26rem,calc(100vh-8rem))]'>
      <pre className='overflow-wrap-anywhere min-w-0 p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {formattedData}
      </pre>
    </ScrollArea>
  </HoverCardContent>
</HoverCard>
```

`TaskAuditDataDialog` 改为接收 `formattedData`，避免悬停层和 Dialog 重复格式化；标题、说明、复制和滚动行为保持现状。

- [ ] **步骤 4：运行测试并确认交互通过**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx
bun test --parallel=1 src/features/usage-logs/components/__tests__/task-logs-mobile-card.test.tsx
```

工作目录：`web/`

预期：审计列测试和移动端卡片测试全部通过；移动端数据展示契约不变。

- [ ] **步骤 5：提交审计数据预览变更**

```powershell
git add web/src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx web/src/features/usage-logs/components/columns/task-logs-columns.tsx web/src/features/usage-logs/components/dialogs/task-audit-data-dialog.tsx
git commit -m "feat: 优化任务审计数据查看交互"
```

### 任务 2：收紧任务日志和绘图日志桌面表格密度

**文件：**

- 新建：`web/src/features/usage-logs/lib/table-density.ts`
- 新建：`web/src/features/usage-logs/lib/__tests__/table-density.test.ts`
- 修改：`web/src/features/usage-logs/components/usage-logs-table.tsx`

- [ ] **步骤 1：先写分类密度失败测试**

测试要求 `task` 与 `drawing` 使用相同紧凑配置，`common` 保持当前普通密度：

```ts
const taskDensity = getUsageLogsTableDensity('task')
const drawingDensity = getUsageLogsTableDensity('drawing')
const commonDensity = getUsageLogsTableDensity('common')

assert.equal(taskDensity.rowClassName, '!h-13')
assert.equal(drawingDensity.rowClassName, '!h-13')
assert.match(taskDensity.getColumnClassName('status', 'header') ?? '', /px-1\.5/)
assert.match(taskDensity.getColumnClassName('status', 'cell') ?? '', /py-2\.5/)
assert.equal(commonDensity.rowClassName, '')
assert.equal(commonDensity.getColumnClassName('status', 'cell'), 'py-2')
```

同时断言审计数据表头允许自然换行，避免长标题继续撑大列宽：

```ts
assert.match(
  taskDensity.getColumnClassName('upstream_response_data', 'header') ?? '',
  /whitespace-normal/
)
```

- [ ] **步骤 2：运行测试并确认模块尚不存在**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/lib/__tests__/table-density.test.ts
```

工作目录：`web/`

预期：失败，提示无法导入 `../table-density`。

- [ ] **步骤 3：实现稳定密度配置并接入列表**

新建模块，返回稳定对象和稳定列 class 回调，避免每次渲染破坏 `DataTableRow` 的 memo：

```ts
const taskAuditColumnIds = new Set([
  'user_request_data',
  'upstream_response_data',
  'user_response_data',
])

const compactColumnClassName: DataTableColumnClassName = (columnId, kind) => {
  if (kind === 'header') {
    return cn(
      'px-1.5',
      taskAuditColumnIds.has(columnId) &&
        'h-auto min-h-10 whitespace-normal py-2 leading-tight'
    )
  }
  return 'px-1.5 py-2.5'
}

const commonColumnClassName: DataTableColumnClassName = (_columnId, kind) =>
  kind === 'cell' ? 'py-2' : undefined

const compactDensity = {
  rowClassName: '!h-13',
  getColumnClassName: compactColumnClassName,
}

const commonDensity = {
  rowClassName: '',
  getColumnClassName: commonColumnClassName,
}

export function getUsageLogsTableDensity(logCategory: LogCategory) {
  return logCategory === 'common' ? commonDensity : compactDensity
}
```

在 `UsageLogsTable` 中取得一次分类配置，并同时传给 `DataTablePage.getColumnClassName` 与自定义 `DataTableRow`：

```tsx
const tableDensity = getUsageLogsTableDensity(logCategory)

<DataTablePage
  getColumnClassName={tableDensity.getColumnClassName}
  renderRow={(row) => (
    <DataTableRow
      key={row.id}
      row={row}
      className={cn(
        'transition-colors',
        tableDensity.rowClassName,
        tintClass
      )}
      getColumnClassName={tableDensity.getColumnClassName}
    />
  )}
/>
```

删除当前行内创建的 `getColumnClassName={() => ...}`，普通日志继续使用 `py-2`，移动端仍走 `UsageLogsMobileList`。

- [ ] **步骤 4：运行密度测试和用量日志测试**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/lib/__tests__/table-density.test.ts
bun test --parallel=1 src/features/usage-logs
```

工作目录：`web/`

预期：全部用量日志测试通过；任务/绘图分类返回紧凑配置，普通日志保持现有配置。

- [ ] **步骤 5：提交桌面表格紧凑布局**

```powershell
git add web/src/features/usage-logs/lib/table-density.ts web/src/features/usage-logs/lib/__tests__/table-density.test.ts web/src/features/usage-logs/components/usage-logs-table.tsx
git commit -m "style: 收紧任务与绘图日志列表"
```

### 任务 3：静态检查、构建与浏览器验收

**文件：** 无额外生产文件。

- [ ] **步骤 1：运行类型检查、涉及文件 lint 和格式检查**

```powershell
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/usage-logs/components/columns/task-logs-columns.tsx src/features/usage-logs/components/dialogs/task-audit-data-dialog.tsx src/features/usage-logs/components/usage-logs-table.tsx src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx src/features/usage-logs/lib/table-density.ts src/features/usage-logs/lib/__tests__/table-density.test.ts
bun run format:check
```

工作目录：`web/`

预期：类型检查、lint 和格式检查均无错误。

- [ ] **步骤 2：运行生产构建**

```powershell
bun run build
```

工作目录：`web/`

预期：Rsbuild 生产构建成功。

- [ ] **步骤 3：启动本地开发服务**

```powershell
bun run dev -- --host 127.0.0.1
```

工作目录：`web/`

预期：输出本地访问 URL；若默认端口被占用，使用 Rsbuild 自动选择的新端口。

- [ ] **步骤 4：完成桌面浏览器验收**

打开任务日志和绘图日志页面，确认：

- 三列“查看”入口悬停后出现完整 JSON，可在浮层内滚动并复制。
- 点击入口后悬停层消失，详情 Dialog 打开；关闭后焦点返回入口。
- 键盘 `Tab` 聚焦入口可看到预览，`Enter` 打开详情，`Esc` 可关闭浮层。
- 任务日志和绘图日志的表头横向留白、数据行高度均明显收紧，双行时间、状态、任务 ID 和错误信息没有重叠或截断异常。
- 普通日志列表密度保持不变。

- [ ] **步骤 5：完成移动端与窄桌面验收**

使用 `390x844` 和约 `900x700` 视口确认：

- 移动端任务日志卡片内容和点击查看行为保持正常，不依赖悬停。
- 窄桌面 Hover Card 不越出视口，长 JSON 可滚动，复制按钮不遮挡标题或内容。
- 页面横向滚动、表头和单元格不存在不合理重叠。

- [ ] **步骤 6：检查最终差异**

```powershell
git diff --check
git status --short
```

预期：无空白错误；只保留本任务文件及工作区中原有、未纳入本任务的改动。

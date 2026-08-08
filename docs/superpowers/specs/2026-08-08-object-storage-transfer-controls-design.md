# 对象存储视频转存控制设计

## 背景

对象存储页面当前只有视频结果转存总开关，以及固定语义的白名单和黑名单文本框。页面无法明确表达“全部转存”和“域名规则”两种互斥模式，也无法表达规则开关已启用但列表暂时为空的状态。本次改造只调整视频转存策略配置和管理页面交互，连接参数、密钥、签名链接和视频大小配置保持原有行为。

## 目标

- 提供视频结果转存总开关。
- 提供“全部转存”和“域名规则”两个互斥开关，两个开关均可关闭，均关闭时按默认不转存处理。
- 在域名规则模式中分别提供白名单和黑名单开关及输入框。
- 明确四种规则组合：白名单单独开启时仅转存白名单；黑名单单独开启时仅排除黑名单；两者同时开启时白名单命中转存、黑名单命中不转存；两者均关闭时默认不转存。
- 关闭总开关时保留模式、开关和输入内容，重新开启后恢复原配置。
- 兼容已有配置，升级后不改变既有转存结果。

## 配置模型

在 `setting/object_storage.ObjectStorageConfig` 中新增：

- `TransferMode string`，JSON 键 `transfer_mode`，允许值为 `default`、`all`、`rules`。
- `WhitelistEnabled bool`，JSON 键 `whitelist_enabled`。
- `BlacklistEnabled bool`，JSON 键 `blacklist_enabled`。

现有 `TransferDomainWhitelist` 和 `NoTransferDomainBlacklist` 列表继续保留。配置通过现有选项键值表持久化，不新增数据库表或数据库方言相关迁移。

### 默认和兼容归一化

`NormalizeConfig` 在新字段为空或无效时执行兼容归一化：

- 任一旧域名列表非空时，将模式初始化为 `rules`，并根据对应列表是否非空初始化规则开关。
- 两个列表均为空时，将模式初始化为 `default`，规则开关保持关闭。
- 未知模式归一为 `default`。
- 切换模式时不清空域名列表和规则开关；`all` 模式仅在判断时忽略规则，切回 `rules` 后恢复。

## 转存判定

`pkg/objectstorage.ShouldTransfer` 扩展为接收转存模式和两个规则开关，仍先校验源视频链接为绝对 HTTP(S) URL。

1. `default` 返回 `false`。
2. `all` 返回 `true`。
3. `rules` 模式：
   - 黑名单开关开启且域名命中黑名单时返回 `false`；
   - 白名单开关开启且域名命中白名单时返回 `true`；
   - 两个开关都开启且同时命中时黑名单优先，返回 `false`；
   - 白名单未命中或白名单关闭时返回 `false`。

总开关关闭由 `ProcessVideoResultURL` 在判定前处理，直接保留上游 URL，不触发下载、上传或对象存储访问。

## 管理页面交互

对象存储页面的“视频结果转存”分组包含：

- 总开关“启用视频结果转存”；
- “全部转存”开关；开启时将 `transfer_mode` 设为 `all`，并关闭“域名规则”；
- “域名规则”开关；开启时将 `transfer_mode` 设为 `rules`，并关闭“全部转存”；
- 两个开关均关闭时将 `transfer_mode` 设为 `default`。

选择 `rules` 后显示两个规则行：

- 白名单开关与白名单多行输入框；
- 黑名单开关与黑名单多行输入框。

规则开关关闭时对应输入框禁用但不清空；再次开启后可以继续编辑。两个规则开关同时开启时显示黑名单优先提示。总开关关闭时模式和规则控件保留，页面显示暂停生效状态，不自动清空或重置值。

表单保存请求新增上述字段，响应回填同样字段。域名文本继续按现有规则逐行解析、规范化、去重和校验。

## API 变更

更新对象存储管理接口的请求和响应 DTO：

- `transfer_mode`
- `whitelist_enabled`
- `blacklist_enabled`

更新控制器的配置转换和响应映射。后端归一化后的值作为最终响应，确保刷新页面后开关状态与实际运行时配置一致。

## 测试策略

### 后端

- 配置默认值、旧配置迁移和未知模式归一化。
- `ShouldTransfer` 的 `default`、`all`、`rules` 三种模式。
- 域名规则四种开关组合、黑名单优先、通配域名和无匹配默认不转存。
- 视频结果处理在总开关关闭、全部转存、规则模式下的上传或直返行为。
- 控制器请求和响应包含新增字段，并保持密钥脱敏与旧字段兼容。

### 前端

- 页面渲染三个层级的转存开关。
- 开启一个模式开关会关闭另一个，两个均可关闭。
- 规则开关关闭时输入框禁用且内容保留。
- 四种规则组合的保存请求字段和冲突提示。
- 关闭总开关后再次开启仍保留模式、规则开关和输入内容。

所有新增 UI 文案通过 `web/scripts/add-missing-keys.mjs` 写入 `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi`，并运行 `bun run i18n:sync`。

## 验证

- `go test ./pkg/objectstorage ./setting/object_storage ./service ./controller`
- `cd web && bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`
- `cd web && bun run typecheck`
- 对涉及 TSX、类型和本地化脚本执行项目 lint。
- `cd web && bun run build`

## 不在范围内

- 不修改 S3 兼容协议、对象键生成、签名链接时长和视频大小限制。
- 不改变上游渠道选择、任务日志和计费逻辑。
- 不新增对象存储供应商专用配置。

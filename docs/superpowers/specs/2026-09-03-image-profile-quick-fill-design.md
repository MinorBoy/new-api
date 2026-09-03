# 图像协议绑定默认配置与快捷恢复设计

## 背景

渠道编辑抽屉中的“图像协议绑定”目前要求管理员手动编辑 JSON。普通的 OpenAI-compatible 图像渠道使用固定的协议档案和标准路径，手写配置会增加录入时间和 JSON 填写错误。目标是在不改变后端协议、兼容性测试和成本核算流程的前提下，降低首次配置成本。

## 目标与范围

### 目标

- 支持 OpenAI Images 的渠道在图像配置为空时自动获得标准配置。
- 保留现有 JSON 编辑能力，允许高级渠道覆盖协议档案内容。
- 提供明确的“恢复默认配置”操作，便于修复误填内容。
- 通过前端行为测试保护自动填充、覆盖保护和权限禁用等用户可见契约。

### 非目标

- 不新增后端字段或修改 `settings.image_profile` 的保存格式。
- 不改变图像兼容性测试的端点、超时、请求协议或结果判定。
- 不改变图像供应商成本、公共模型售价或路由权重策略。
- 不对已有无效 JSON 做静默修复。

## 方案

### 默认档案

在渠道表单图像配置模块定义单一的默认档案常量，并使用格式化 JSON 展示：

```json
{
  "profile": "openai_images",
  "profile_version": 1,
  "paths": {
    "generations": "/v1/images/generations",
    "edits": "/v1/images/edits"
  }
}
```

该常量只作为前端表单初始值和“恢复默认配置”的来源，提交仍由现有 `buildSettingsJSON` 解析为对象后写入 `settings.image_profile`。

### 自动填充规则

- 仅当当前渠道类型属于 `OPENAI_IMAGES_CHANNEL_TYPES` 时执行。
- 新建渠道初始化时，如果 `image_profile` 去除空白后为空或等于 `{}`，写入默认档案并标记字段为 dirty，使用户保存时落库。
- 新建表单切换到支持图像的渠道类型时执行同样判断。
- 编辑渠道加载后，后端返回的有效 JSON 原样保留，不覆盖用户配置。
- 编辑渠道加载后，如果返回值为空，可填入默认档案；如果返回值是无效非空 JSON，保留原值并交给现有 Zod 校验提示。
- 切换到非图像渠道类型时不自动填充，也不额外清理已有值；提交时沿用既有 settings 构建逻辑。

### 恢复默认配置

在 JSON 编辑器工具栏提供次要按钮“恢复默认配置”，与复制、格式化操作并列。按钮点击后直接将字段替换为格式化默认 JSON，并触发表单 dirty 和校验状态。该操作是唯一允许在非空配置上覆盖用户内容的入口，自动填充永远不覆盖非空值。

按钮在敏感配置锁定或表单提交中禁用；非图像渠道不渲染。操作不弹确认框，避免普通修复流程被打断，用户仍可通过撤销表单修改或关闭抽屉放弃变更。

### 界面提示

- 空值自动填充后显示简短状态提示“已自动填入默认配置”。
- 用户修改 JSON 后状态提示显示“已自定义”。
- placeholder 只保留简短用途说明，不重复完整 JSON 示例。
- 所有新增文案通过 `useTranslation()` 的 i18n 键渲染，并同步七种前端 locale 与 `static-keys.ts`（如项目提取流程要求）。

## 数据流与错误处理

1. 渠道类型或编辑数据变化时，表单 effect 读取当前 `image_profile`。
2. 对支持图像的类型执行空值判断；满足条件则调用 `form.setValue`，设置 `shouldDirty` 和 `shouldValidate`。
3. JSON 编辑器继续通过 `FormField` 绑定字段，手动输入实时更新状态。
4. 提交时沿用 `channelFormSchema` 的对象 JSON 校验；空值删除 `settings.image_profile`，有效值序列化为对象。
5. 无效非空 JSON 不被自动格式化或覆盖，字段错误仍由现有表单错误展示。
6. 自动填充不会改变兼容性测试的“保存后才能测试”门槛，因此自动填入后需先保存渠道再测试。

## 测试验收标准

在 `web/src/features/channels/components/drawers/__tests__/` 增加用户行为测试，至少覆盖：

- 支持图像类型的新建表单在空值时显示默认 JSON，并将字段标记为待保存。
- 支持图像类型已有有效 JSON 时，打开编辑抽屉不会覆盖原内容。
- 点击“恢复默认配置”后，非空自定义 JSON 被替换为标准档案。
- 非图像渠道不会显示自动填充提示或恢复按钮。
- 敏感字段锁定或提交中，恢复按钮不可用。
- 恢复后输入无效 JSON 时，保存触发字段级校验错误，且不会静默写入。

验证命令：

```text
bun run vitest run src/features/channels/components/drawers/__tests__
bun run typecheck
bun run lint -- src/features/channels/components/drawers/channel-mutate-drawer.tsx src/features/channels/components/drawers/__tests__
```

具体脚本参数以 `web/package.json` 当前定义为准；若 lint 脚本不接受路径参数，则运行项目规定的等价文件级检查。

## 发布与回滚

本设计只涉及前端渠道编辑表单和翻译资源，不需要数据库迁移或后端部署。回滚时撤销前端提交即可，已有 `settings.image_profile` 数据格式保持兼容。


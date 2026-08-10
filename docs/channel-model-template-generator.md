<!--
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
-->

# 渠道模型成本与利润模板维护

生成的 `渠道模型成本与利润模板-v1.xlsx` 是输出物，不是长期维护的主数据。

维护时优先更新原始表 `sd收录.xlsx`：

1. `channel`：渠道名称、定价页面和 Base URL。
2. `sd`：渠道模型、系列、计费方式、统一单价、结构化素材限制、状态和备注。
3. `sd官价`：官方 SKU、系列、分辨率、帧率、尺寸和官方售价。

不要直接在生成后的 V1 模板中长期手工修改数据。任何需要保留的修正都应写回原始表或规则文件，再重新生成。

## 最新源表契约

- `系列` 是渠道模型与官方价格的匹配维度。官方价格必须按“系列 + 模型 + 分辨率”匹配，不能让 Seedance 2.0 与 2.5 交叉取价。
- `计费方式 + 单价 元` 是供应商成本的唯一源表字段组合。`second`、`call`、`token` 分别决定按时长、按次和按 Token 计费，三种模式都读取 `单价 元`。
- 是否支持视频输入只由 `参考视频数` 判定：大于 0 表示支持，等于 0 表示不支持。空值、负数或非整数属于素材合同错误，不能猜测或默认。
- `sd` 不再支持旧价格列 `元/秒`、`元/次`、`元/1M`，也不再支持 `视频输入`。检测到旧字段或新旧字段混用时必须停止生成。
- `sd官价` 可以先收录 Seedance 2.5 官方价格；只有 `sd` 中存在同系列的有效渠道模型行时，才生成对应 SKU、售价、成本映射和活动配置。仅有官方价格不得发布或激活 2.5。

## 规则文件

`web/scripts/channel-model-template/conversion-rules.json` 保存不适合直接写入原始表的、可审计的转换规则：

- 新的源渠道编号到稳定 `CH-*` 渠道代码的映射；
- 源模型 ID 到客户端模型、上游模型或 SKU 能力的明确映射；
- 已确认的逐行成本或能力例外；
- 货币、汇率、折扣和 token 换算等全局假设。

每项例外都必须有确定的源行号或渠道/行标识。规则文件不允许保存 API Key、Token、Cookie 或其他凭据。

## V1 基座

`web/src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx` 只负责 V1 工作簿的布局、表头、公式和格式契约。只有模板结构、公式或 V1 表头变化时才更新它；日常价格和模型维护不要修改基座。

## 生成

原始表固定在仓库根目录下的 `docs\new-channels\sd收录.xlsx`。在 `web` 目录执行时，使用其相对路径 `..\docs\new-channels\sd收录.xlsx`。每次使用一个新的输出文件名：

```powershell
bun run channel-model-template:generate -- `
  --source "..\docs\new-channels\sd收录.xlsx" `
  --rules "scripts\channel-model-template\conversion-rules.json" `
  --base "src\channel-config-converter\__fixtures__\channel-config-v1-corrected.xlsx" `
  --output "C:\Users\880pro\Documents\new-api\outputs\<日期>\渠道模型成本与利润模板-v1.xlsx" `
  --allow-warnings
```

命令会在同目录写出 `渠道模型成本与利润模板-v1.report.json`。默认禁止覆盖已有输出；只有确认需要替换时才添加 `--force`。

## 审核和发布

1. 先查看 JSON 报告。任何 `FAIL` 都不会生成正式工作簿，必须修复源表或规则后重试。
2. `WARN` 对应的成本和映射会保留为 `draft`，不会作为活动配置发布。先补齐同系列官方价格、结构化素材限制或明确规则；仅在需要审阅草稿时使用 `--allow-warnings`。
3. 打开生成的 Excel 后让它完成重算，检查 `校验` 工作表。
4. 将 V1 工作簿通过现有转换器校验后再用于后续配置导入。

日常流程因此是：更新原始表，必要时更新显式规则，运行生成命令，审核报告和草稿项。无需由 agent 根据表格内容临时判断或手工拼接模板。

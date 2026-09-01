# OpenAI Images 兼容渠道配置

本文说明如何在 new-api 中接入提供 OpenAI Images 兼容接口的 `gpt-image`、`gemini-image` 等生图模型。Google 表格或 `refreshing-sd-channel-config` 只继续维护 Seedance 配置；图像模型的协议能力、售价和路由在 new-api 管理端维护。

## 配置来源

图像请求使用四层配置：

1. **内置协议档案**：`openai_images` 定义 generations/edits 的请求、响应和安全上限。
2. **全局图像模型目录**：系统设置中的 `ImageModelCatalog` 定义公共模型、端点能力、默认值、SKU 和单张用户售价。
3. **渠道 Image Profile**：渠道编辑页的 `image_profile` 定义档案版本、路径覆盖、上游模型映射和能力收窄。
4. **供应商成本规则**：成本规则按渠道、映射后的上游模型和 SKU 保存 `per_image` 成本，并通过草稿、校验、激活流程发布。

## 全局图像目录

在「系统设置 → 计费 → Image Models & Routing」维护目录。目录必须是版本为 `1` 的 JSON 对象；每个启用端点至少配置一个默认 SKU。价格使用非负十进制定点字符串，单位始终是单张图：

```json
{
  "version": 1,
  "models": {
    "gpt-image-1": {
      "profile": "openai_images",
      "profile_version": 1,
      "endpoints": {
        "generations": {
          "capability": {
            "enabled": true,
            "sizes": ["1024x1024"],
            "qualities": ["medium"],
            "response_formats": ["b64_json"],
            "max_n": 4
          },
          "default_size": "1024x1024",
          "default_quality": "medium",
          "default_response_format": "b64_json"
        }
      },
      "skus": {
        "gen-1024x1024-medium": {
          "endpoint": "generations",
          "size": "1024x1024",
          "quality": "medium",
          "unit": "image",
          "sale_price_usd": "0.040000"
        }
      }
    }
  }
}
```

`n` 会在请求校验时限制在 `1..128`，并按实际生成数量结算。修改目录后，使用旧目录合同哈希的渠道兼容性状态会被视为过期，需要重新测试。

## 渠道配置

渠道类型必须是支持 OpenAI Images 适配器的类型：OpenAI、Azure、OpenAI Max、Custom、OpenRouter 或 Xinference。渠道的 `models` 仍填写下游公共模型名，`model_mapping` 填写供应商上游模型名：

```json
{
  "image_profile": {
    "profile": "openai_images",
    "profile_version": 1,
    "paths": {
      "generations": "/v1/images/generations",
      "edits": "/v1/images/edits"
    },
    "capability_overrides": {
      "gpt-image-1": {
        "edits": false,
        "max_n": 1
      }
    }
  }
}
```

能力覆盖只能收窄全局目录和协议档案，不能放宽尺寸、响应格式、输入图数量或 `max_n`。原生 Gemini `generateContent` 渠道不属于该档案；只有供应商明确提供 OpenAI Images 兼容路径时，才创建单独的兼容渠道。

普通渠道保存不能伪造 `compatibility.status=passed`。在渠道页保存档案后，使用「图像兼容性测试」调用 `POST /api/channel/:id/image-profile/test`，测试结果只保存脱敏摘要、档案版本、合同哈希和时间。档案、路径、映射、能力或目录发生变化后，旧结果自动失效。

## 供应商成本

为每个渠道的映射后模型和 SKU 创建成本规则：

- 成本模式选择 `per_image`；`UnitPrice` 表示单张图供应商成本。
- `cost_variant_key` 必须使用目录生成的 SKU，例如 `gen-1024x1024-medium` 或 `edit-1024x1024-high`。
- 图像规则使用 `response_succeeded` 结算事件和 `validated_request` 或 `upstream_actual` 计量源。
- 成本规则必须校验并激活后，严格成本模式才会将其用于路由。

同一公共模型可以把多个渠道放进同一分组。每个渠道仍使用自己的映射、兼容性状态和成本规则，不需要在 SD 收录表重复登记协议能力。

## 路由策略

`ImageRoutingPolicy` 默认策略为 `manual`：继续按现有 Priority 分层和 Weight 加权。将某个分组/模型设置为 `lowest_cost` 后，候选排序为：

1. 已知本次请求成本优先于未知成本。
2. 成本相同时按 Priority 从高到低。
3. 成本和 Priority 都相同才按 Weight 加权。

需要在成本接近时保留多个供应商流量时，可使用 `cost_weighted`：

```json
{
  "version": 1,
  "default": {
    "strategy": "cost_weighted",
    "cost_tolerance_bps": 1000
  }
}
```

`cost_tolerance_bps` 是相对最低已知成本的容差，范围为 `0..10000`，缺省为 `1000`（10%）。只有成本已知、通过兼容性和最低毛利检查、且成本不超过最低成本 `1 + 容差` 的渠道进入混流池；未知成本不会进入该池。池内有效权重为现有 `Weight + 10` 乘以最低成本与当前成本的倒数比例，因此低价渠道获得更多请求，成本接近的高价渠道仍会保留流量。重试排除渠道后会重新计算最低成本和混流池。

`cost_weighted` 不承诺固定的 80/20 比例。需要精确比例时，请继续使用 `manual`，把渠道设为同一 Priority，再通过 Weight 配置比例。

严格成本模式下，缺少成本规则、成本无法计算、兼容性失败或未达到最低毛利的渠道会被排除。`require_compatibility_test=true` 时，未通过当前合同测试的渠道也会被排除；关闭时，未测试渠道可以作为回退但会留下管理员诊断。

管理端可在图像路由预览中查看每个候选的上游模型、SKU、预计成本、规则版本和排除原因。预览接口为 `POST /api/routing-policies/image/preview`，不会发送上游请求。

## 重试和结算

明确未发送到上游的失败可以排除当前渠道并重新计算成本排序。上游已经接受、响应状态未知或等待实际计量的图像请求不会自动切换供应商，以避免重复生成和重复扣费。响应中的 `data` 数量或供应商用量优先作为实际生成数量；无法可靠取得时回退到已校验的请求 `n`。

售价和供应商成本均在请求开始时冻结。配额换算使用有界 decimal 算法，发生饱和时写入管理员可见的 `quota_saturation` 审计标记，不会产生负扣费。

## 灰度与回滚

建议先保持全局 `manual`，为一个测试分组的单个模型启用 `lowest_cost`，检查路由预览、兼容性哈希、成本规则和结算数量。出现异常时把策略切回 `manual`，或禁用渠道的 `image_profile`；不需要修改 SD 收录表。

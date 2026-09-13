# OpenAI Image 2.5 模型矩阵适配设计

## 背景

上游新增 `gpt-image-2.5-flare` 与 `gpt-image-2.5-sunburst`。两个模型与现有 `gpt-image-2` 使用相同的 `openai_images` 协议，并支持相同的九种能力组合：`1k/2k/4k × low/medium/high`。

当前图像目录、渠道能力矩阵、上游成本矩阵和用户售价矩阵已经按模型键工作，但默认数据、同步入口、管理端筛选和模型广场展示仍主要以 `gpt-image-2` 为示例。适配目标是在不改变已有模型行为的前提下，让两个新模型可以独立配置、路由、计费和展示。

## 目标与非目标

### 目标

- 在公共图像目录中为两个新模型创建独立模型条目。
- 每个新模型拥有独立的 generations 能力矩阵和九个按档位/质量划分的 SKU。
- 渠道页可按模型分别保存能力矩阵；渠道可以只支持部分组合。
- 成本同步按“渠道 + 客户端模型映射后的上游模型 + SKU”独立生成和激活成本规则。
- 售价矩阵和模型广场按模型、分辨率档位、质量展示实际售价，不再把 `gpt-image-2` 的价格误用于新模型。
- 下游按 `model + size + quality` 请求时，路由只选择声明支持该组合且有有效成本规则的渠道。
- 保持既有 `gpt-image-2` 配置、旧具体尺寸 SKU 迁移和现有 API 行为兼容。

### 非目标

- 不新增新的协议 profile 或 profile version。
- 不改变 1K/2K/4K 总像素上限、`size` 省略或 `auto` 归入 1K 的规则。
- 不把 SKU 维度抽象为任意动态属性集合。
- 不自动修改生产渠道的真实上游价格或用户售价；只扩展配置能力和默认目录结构。

## 方案

采用模型级复用现有矩阵的方案。目录中的每个模型均保存完整的 `profile`、端点能力和 SKU 集合，模型之间不共享可变配置。

新增模型的目录形态如下（字段与现有目录一致）：

```json
{
  "gpt-image-2.5-flare": {
    "profile": "openai_images",
    "profile_version": 1,
    "endpoints": {
      "generations": {
        "capability": {
          "enabled": true,
          "resolution_tiers": ["1k", "2k", "4k"],
          "qualities": ["low", "medium", "high"],
          "response_formats": ["b64_json"],
          "max_n": 4
        },
        "default_size": "auto",
        "default_quality": "medium",
        "default_response_format": "b64_json"
      }
    },
    "skus": {
      "gen-1k-low": {"endpoint": "generations", "tier": "1k", "quality": "low", "unit": "image"},
      "gen-1k-medium": {"endpoint": "generations", "tier": "1k", "quality": "medium", "unit": "image"},
      "gen-1k-high": {"endpoint": "generations", "tier": "1k", "quality": "high", "unit": "image"},
      "gen-2k-low": {"endpoint": "generations", "tier": "2k", "quality": "low", "unit": "image"},
      "gen-2k-medium": {"endpoint": "generations", "tier": "2k", "quality": "medium", "unit": "image"},
      "gen-2k-high": {"endpoint": "generations", "tier": "2k", "quality": "high", "unit": "image"},
      "gen-4k-low": {"endpoint": "generations", "tier": "4k", "quality": "low", "unit": "image"},
      "gen-4k-medium": {"endpoint": "generations", "tier": "4k", "quality": "medium", "unit": "image"},
      "gen-4k-high": {"endpoint": "generations", "tier": "4k", "quality": "high", "unit": "image"}
    }
  }
}
```

`gpt-image-2.5-sunburst` 使用同样结构和 SKU 键。售价字段由全局售价矩阵维护，供应商成本由每个渠道的成本规则维护；目录模板不写入未经确认的真实价格。

## 数据流

1. 管理端读取图像目录，按模型列出能力矩阵和九个 SKU。
2. 渠道编辑器根据渠道模型列表和 `image_profile.capability_overrides` 展示各模型的独立勾选状态；空配置仍使用目录默认能力，显式清空后保存空集合。
3. 保存渠道后，图像成本同步根据模型映射和已声明能力创建/更新对应 SKU 的成本草稿；发布后才成为 active 规则。
4. 图像请求解析 `model`、`size`、`quality`，由目录计算分辨率档位并解析模型对应 SKU。
5. 路由筛选同模型、同端点、支持该能力组合且存在 active 成本规则的渠道，再按现有 routing policy 选择。
6. 计费使用该模型 SKU 的全局售价，渠道成本用于成本核算和 `lowest_cost`/毛利约束策略。
7. 模型广场读取按模型聚合的售价矩阵，分辨率档位和质量分别显示，避免显示旧模型单一价格。

## 兼容性与校验

- `gpt-image-2` 原有目录键、SKU 键、API 请求和数据库成本规则不变。
- 旧渠道只配置 `gpt-image-2` 时，不自动获得两个新模型的能力；渠道必须在模型列表/映射中显式加入新模型并保存能力矩阵。
- 渠道声明了新模型但没有能力覆盖时，使用目录默认九组合；显式保存空矩阵时视为不支持任何组合。
- 新模型缺少对应 active 成本规则时，不进入严格成本路由；管理端显示缺失成本状态，避免把未知成本当成零成本。
- 所有模型名和 SKU 均经过现有目录、profile 和成本规则校验；不放宽 `1k/2k/4k`、质量枚举、`max_n` 或图像计费数量边界。

## UI 适配

- 渠道编辑器增加模型级分组标题，分别显示 `gpt-image-2`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst` 的能力矩阵。
- 能力勾选继续使用九个组合，不要求手动编辑 JSON；现有 JSON 绑定保留为高级兼容入口。
- 图像价格工作台和模型广场按模型筛选，并在每个模型卡片内按 `1K/2K/4K` 展示 `low/medium/high` 售价。
- 成本缺失、草稿、active 和覆盖状态继续沿用现有状态语义，不新增另一套成本生命周期。

## 测试策略

### 后端

- 目录加载测试：两个新模型包含 `openai_images` profile、generations 端点和九个 SKU。
- 解析测试：两个新模型分别覆盖 `auto`/1K、2K、4K 与三种质量的 SKU 解析。
- 能力覆盖测试：新模型的部分矩阵、空矩阵和默认矩阵语义正确，且不影响旧模型。
- 成本同步测试：按新模型和映射后的上游模型生成独立成本规则，缺失规则不会被错误当作 active。
- 售价/模型广场测试：三个模型的价格聚合互不串用，并按档位/质量返回。
- 路由测试：请求只命中支持对应模型和能力组合、且成本规则有效的渠道。

### 前端

- 渠道矩阵可分别编辑三个模型，刷新后状态保持，显式清空不会回填九项。
- 成本工作台可按新模型显示九个 SKU 和成本状态。
- 模型广场不再显示单一旧模型价格，长价格摘要在卡片内正确布局。
- 运行现有 typecheck、lint、单元测试和生产构建。

## 风险与回滚

- 风险主要是默认目录新增模型后，管理端筛选或同步代码仍只遍历旧模型。通过模型遍历测试和 UI E2E 覆盖避免。
- 目录新增是向后兼容的；如需回滚，可删除两个新模型目录项和对应未发布成本/售价规则，不影响 `gpt-image-2` 数据。
- 不修改已有渠道的模型列表、能力覆盖或 active 成本规则，避免配置面意外扩大。

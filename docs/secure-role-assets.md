# Secure 企业 Video 角色素材

本项目为下游用户提供统一的图片角色素材接口，并将素材请求转发到创建时绑定的 Secure 企业 Video 渠道。

## 素材接口

所有接口均支持登录会话或项目 API Key。新建素材的项目 ID 格式为 `asset-YYYYMMDDHHmmss-xxxxx`（例如 `asset-20260401123823-6d4x2`）。素材只属于创建用户，其他用户访问同一 `asset-*` 返回 `asset_not_found`。

- `POST /api/v3/assets`：创建图片素材。请求体为 `{"type":"image","url":"https://example.com/character.png"}`，`url` 必须是无需登录即可访问的公网 HTTP(S) 地址；建议携带唯一 `Idempotency-Key`。
- `GET /api/v3/assets`：列出当前用户素材。
- `GET /api/v3/assets/{asset_id}`：查询并刷新素材状态。
- `POST /api/v3/assets/{asset_id}/refresh`：主动刷新状态。

响应只返回项目素材 ID、原始公网图片 URL、状态和 `asset://asset-*` 引用，不返回上游 `asset-local-*`、渠道 ID 或 API Key。

状态包括 `processing`、`active`、`failed` 和 `unknown`。只有 `active` 素材可以用于视频生成；Secure 创建和查询失败会保留已知状态并返回 `asset_upstream_error`。

## 视频引用

Ark 请求使用标准内容格式：

```json
{
  "model": "video-2.0-pro",
  "content": [
    {"type": "text", "text": "角色走过街道"},
    {"type": "image_url", "role": "reference_image", "image_url": {"url": "asset://asset-..."}}
  ],
  "duration": 8,
  "ratio": "16:9",
  "resolution": "720p"
}
```

同一请求最多 9 个角色素材。角色素材不能与普通公网图片混用，且只能路由到绑定素材创建时使用的 Secure enterprise 渠道。系统在上游请求中转换为 `use_person_character=true` 与 `extra_images: ["asset://asset-local-..."]`。

第一版不提供素材删除接口；Secure 上游素材长期保留。已绑定素材的 Secure 渠道不允许删除、修改 API Key、切换企业分组或启用多 Key，但允许修改普通描述字段或禁用渠道。

# 统一角色素材中转系统设计

## 1. 背景与目标

项目当前已经接入 Secure 企业 Video 的视频生成链路，但尚未接入 Secure 的角色素材 API。Secure 角色素材具有明确的上游凭证归属：素材只能由创建它的 Secure 企业 Video API Key 查询和引用。

本设计在项目内增加一层面向下游用户的统一角色素材系统。下游用户使用在本项目创建的 API Key 调用素材接口；项目使用管理员配置的 Secure 企业 Video 渠道 API Key 调用 Secure `/v1/asset/create` 和 `/v1/asset/get`，并将上游 `asset-local-*` 映射为项目内的 `asset-*`。

第一版范围严格限定为：

- 只支持视频生成角色素材；
- 素材类型只支持图片；
- 只接入 Secure 企业 Video；
- 图片输入只接受无需登录即可访问的公网 HTTP(S) URL；
- 不实现本地文件上传、Base64、音频素材、视频素材或其他上游；
- 视频生成继续使用 Ark SDK 标准请求格式；
- 一个请求可引用多个素材，数量和其他参数由绑定渠道的模型能力约束；Secure 企业 `video-2.0-pro` 最多支持 9 张图片。

后续接入更多上游时，可以在不改变下游 API 的前提下增加适配器和上游绑定副本，但第一版不实现跨渠道自动复制或动态迁移。

## 2. 核心身份与绑定规则

系统中存在两类 API Key，职责必须分离：

- 下游用户 API Key：由本项目创建，用于认证调用方、确定用户身份和执行素材权限隔离；
- 上游 Secure 渠道 API Key：保存在渠道安全配置中，用于调用 Secure 素材创建和查询接口，并决定 `asset-local-*` 的有效归属。

第一版采用管理员配置的默认素材渠道：

- 系统配置一个默认 Secure enterprise 渠道；
- 素材创建接口不允许下游用户选择 `channel_id`；
- 创建时记录实际 Secure 渠道 ID 和凭证指纹/版本；
- 更换默认渠道只影响新建素材，历史素材继续固定使用创建时的渠道；
- 创建、查询和角色视频生成均不得自动切换其他 Secure 渠道；
- 绑定素材的渠道可以被暂时禁用，但禁用期间相关素材不可查询刷新或生成；
- 绑定素材存在时，禁止修改渠道 API Key 或删除渠道；如因泄露必须退役，应显式强制退役并将旧素材标记为不可用。后续再设计凭证版本归档和迁移。

同一个本地逻辑素材第一版只有一个 Secure 上游绑定。数据结构预留 `provider`、`asset_type` 和多绑定关系，以便未来支持多个上游副本。

## 3. 架构

```text
下游 API / 登录用户素材库
             |
             v
       AssetService
  归属、幂等、状态、ID转换
             |
             v
    AssetProviderAdapter
             |
             v
       SecureAssetAdapter
  /v1/asset/create、/v1/asset/get
```

`AssetService` 是唯一的业务入口，登录页面和 API Key 调用复用相同的归属、状态和错误规则。`AssetProviderAdapter` 隔离供应商协议，第一版只实现 `SecureAssetAdapter`。

创建链路：

1. 下游以用户 API Key 提交公网图片 URL。
2. 项目校验素材类型、URL 协议和公网可访问性边界。
3. 读取当前默认且启用的 Secure enterprise 渠道。
4. 生成可恢复的上游幂等键，调用 Secure `POST /v1/asset/create`。
5. 保存 Secure 返回的 `asset-local-*`、本地 `asset-*`、渠道绑定和初始状态。
6. 返回本地 `asset-*`，不返回上游素材 ID或 API Key。

查询链路：

1. 下游使用本地 `asset-*` 查询。
2. 项目按当前用户和本地 ID查找绑定。
3. 使用绑定渠道的 Secure API Key 调用 `POST /v1/asset/get`。
4. 映射状态并返回统一响应。

生成链路：

1. Ark content 中使用 `image_url.url = asset://asset-*`。
2. 项目识别并批量解析本地素材 ID。
3. 校验用户归属、`Active` 状态、数量限制和绑定渠道一致性。
4. 锁定素材创建时的 Secure 渠道，绕过普通动态渠道选择。
5. 适配器把本地引用转换为 `asset://asset-local-*`，向上游设置 `use_person_character: true` 并填充 `extra_images`。
6. 保留 prompt、duration、ratio、resolution 等企业 Video 参数。

没有 `asset://` 时，保持现有普通视频生成路由和公网参考图片流程；角色素材模式不得混入普通公网图片、参考视频或不支持的字段。

## 4. 统一 API 合同

### 4.1 创建素材

```http
POST /api/v3/assets
Authorization: Bearer <用户 API Key>
Content-Type: application/json
Idempotency-Key: <可选>

{
  "type": "image",
  "url": "https://example.com/character.png"
}
```

响应示例：

```json
{
  "id": "asset-xxxxxxxxxxxxxxxx",
  "type": "image",
  "url": "https://example.com/character.png",
  "status": "processing",
  "provider": "secure",
  "created_at": "2026-08-13T12:00:00Z"
}
```

规则：

- `type` 第一版只能为 `image`；
- `url` 必须为公网 `http` 或 `https` URL；
- 拒绝 multipart、本地文件、Base64、`data:`、`file:` 和 `asset://`；
- 不要求下游必须提供 `Idempotency-Key`；
- 提供 key 时，以“用户 ID + key”幂等，相同请求返回原 `asset-*`；同 key 提交不同 URL 或类型返回冲突；
- 未提供 key 时每次视为新建，项目为 Secure 请求生成随机上游幂等键；
- 前端素材库始终自动生成并发送幂等键。

### 4.2 查询素材

```http
GET /api/v3/assets/{asset_id}
Authorization: Bearer <用户 API Key>
```

查询按需刷新绑定的 Secure 状态。返回数据只包含项目字段和统一状态，不返回 `asset-local-*`、Secure 渠道 API Key 或内部凭证信息。

```json
{
  "id": "asset-xxxxxxxxxxxxxxxx",
  "type": "image",
  "url": "https://example.com/character.png",
  "status": "active",
  "provider": "secure",
  "provider_status": "Active",
  "reference": "asset://asset-xxxxxxxxxxxxxxxx",
  "created_at": "2026-08-13T12:00:00Z",
  "updated_at": "2026-08-13T12:00:08Z"
}
```

### 4.3 素材列表

```http
GET /api/v3/assets?type=image&page=1&page_size=20
Authorization: Bearer <用户 API Key>
```

只返回当前用户拥有的素材。列表页可以使用保存的公网 URL 预览图片，并展示本地 `asset-*`、状态和时间。

### 4.4 刷新素材

```http
POST /api/v3/assets/{asset_id}/refresh
Authorization: Bearer <用户 API Key>
```

页面可以显式刷新；查询接口也会按需刷新。终态允许再次刷新，以处理上游状态延迟或恢复场景。

### 4.5 Ark 视频生成引用

下游继续使用 Ark 标准格式：

```json
{
  "model": "video-2.0-pro",
  "content": [
    {"type": "text", "text": "人物在城市街道行走"},
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "asset://asset-xxxxxxxxxxxxxxxx"}
    }
  ],
  "duration": 8,
  "resolution": "720p",
  "ratio": "16:9",
  "watermark": false
}
```

多个素材可以在同一请求中引用，Secure 企业 `video-2.0-pro` 最多 9 个。所有素材必须属于当前用户、为 `active` 且绑定同一个 Secure 渠道。

## 5. 状态与错误

Secure 状态映射：

| Secure 状态 | 项目状态 | 行为 |
| --- | --- | --- |
| `Pending` | `processing` | 页面继续轮询 |
| `Processing` | `processing` | 页面继续轮询 |
| `Active` | `active` | 允许视频生成 |
| `Failed` | `failed` | 停止轮询 |
| `Unknown` | `unknown` | 停止轮询并提示检查 |

状态同步采用请求驱动策略：创建时保存初始状态；前端每 2 秒调用项目查询接口；项目查询时调用绑定渠道的上游查询接口；进入终态后页面停止轮询。上游临时查询失败时保留已知状态并返回刷新失败，不能误标为 `failed`。视频生成前强制刷新一次，只有 `active` 放行。

统一业务错误码：

- `asset_invalid_url`
- `asset_type_unsupported`
- `asset_not_found`
- `asset_not_active`
- `asset_channel_unavailable`
- `asset_channel_mismatch`
- `asset_reference_mixed`
- `asset_limit_exceeded`
- `asset_idempotency_conflict`
- `asset_upstream_error`

越权访问和不存在的素材统一返回 `asset_not_found`，避免泄露其他用户是否拥有某个 ID。

创建调用超时或响应丢失时，本地记录保持可恢复状态，并保留同一次请求的上游幂等键，后续恢复不能盲目创建第二个上游素材。

## 6. 数据模型

### `assets`

- `id`：项目公开 ID，格式 `asset-*`，唯一且不可枚举；
- `user_id`：素材归属用户；
- `created_by_token_id`：创建请求使用的下游 API Key ID，仅作审计；
- `type`：第一版固定 `image`；
- `source_url`：公网图片 URL，用于预览和审计；
- `status`：`pending`、`processing`、`active`、`failed`、`unknown`；
- `last_error`、`created_at`、`updated_at`；
- `idempotency_key_hash`：只保存幂等键哈希。

### `asset_provider_bindings`

- `asset_id`；
- `provider`：第一版固定 `secure`；
- `channel_id`：创建时实际使用的默认 Secure enterprise 渠道；
- `upstream_asset_id`：Secure `asset-local-*`，只供服务端使用；
- `upstream_status`；
- `upstream_idempotency_key` 的安全可恢复表示或加密存储；
- `upstream_created_at`、`last_checked_at`；
- `credential_fingerprint`：创建时使用的渠道密钥指纹或凭证版本；
- `last_error`。

约束：

- `assets.id` 唯一；
- `(user_id, idempotency_key_hash)` 唯一；
- `(asset_id, provider, channel_id)` 唯一；
- 第一版一个逻辑素材只能有一个 Secure 绑定；
- 所有用户查询必须带 `user_id` 条件；
- Secure API Key 明文继续只保存在渠道安全配置中；
- 本地删除只能是隐藏/停用，不向上游宣称已删除。

## 7. 前端素材库

侧边栏新增登录用户可见的“素材库”入口，进入后提供：

- 素材列表、预览图、项目素材 ID、状态和创建时间；
- 公网图片 URL 创建表单；
- 创建后的 2 秒状态刷新；
- 显式刷新按钮；
- 复制 `asset://asset-*` 引用；
- 用户可理解的失败信息；
- 不展示 `asset-local-*`、渠道 ID、API Key 或凭证指纹；
- 不提供会误导用户的“删除上游素材”操作。

## 8. 安全与治理

- API Key 和登录会话都映射到同一项目用户，并复用同一 `AssetService`；
- 素材查询、刷新和视频引用解析都强制校验用户归属；
- URL 校验拒绝 localhost、回环地址、内网 IP、`data:`、`file:` 和 `asset://`；
- 创建、查询和刷新增加用户级限流，避免页面轮询放大上游请求；
- 日志只记录本地素材 ID、渠道 ID 和错误码，不记录完整上游凭证或敏感 URL 查询参数；
- URL 仍按公网地址交给 Secure 获取，项目不把图片内容上传为本地文件；
- Secure 当前无删除接口，本地停用只影响项目侧引用权限。

## 9. 验收标准

1. 公网 URL 创建成功，Secure `/v1/asset/create` 返回 `asset-local-*` 后本地生成并入库 `asset-*`。
2. 同用户同幂等键重试返回同一素材；参数不同返回冲突。
3. `Pending`、`Processing`、`Active`、`Failed`、`Unknown` 状态映射正确。
4. 不同用户不能查询、刷新或引用其他用户素材。
5. 多个 `asset://asset-*` 能转换成对应的 `asset://asset-local-*`，并固定原绑定渠道。
6. 角色素材与普通公网图片混用时拒绝。
7. 超过 9 个图片素材时拒绝。
8. 有绑定素材时 Secure 渠道密钥修改和删除被拒绝；渠道禁用时返回 `asset_channel_unavailable`。
9. 上游创建超时后本地记录可恢复，不因重试产生重复创建。
10. 前端侧边栏入口、创建、轮询、预览和复制引用均可用。

## 10. 非目标与后续扩展

第一版不实现：

- 本地文件上传和对象存储中转；
- 音频、视频或其他非图片素材；
- Secure 以外的素材供应商；
- 跨 Secure 渠道自动复制素材；
- 凭证轮换后的素材迁移；
- 上游素材删除；
- 用户自选素材渠道。

后续可以新增其他 `AssetProviderAdapter`，在 `asset_provider_bindings` 下维护按渠道的上游副本，并在确认能力矩阵后实现缺少副本时的自动复制和路由。

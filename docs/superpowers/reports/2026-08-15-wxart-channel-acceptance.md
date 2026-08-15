# WxArt Seedance 渠道验收记录

**日期：** 2026-08-15

## 范围

本次接入仅覆盖 WxArt 的 `seedance2.0` 和 `seedance2.5`，代码渠道类型为 `215`，默认地址为
`https://api.wxart.space`。调用方继续使用现有 Ark 任务接口，不新增客户端协议。渠道保持默认禁用。

## 已验证

- WxArt profile 使用 `POST /v1/videos` 和 `GET /v1/videos/{task_id}`，Bearer 请求头、URL 转义和任务注册通过测试。
- WxArt 构造上游请求前要求 provider validation 已完成；新增回归测试确认直接调用构造阶段不会绕过参考媒体时长等校验。
- 请求映射覆盖文本、首尾帧、参考图片、参考视频和参考音频；首尾帧与参考素材不能混用，显式首尾帧比例只能为 `Auto`。
- 模型白名单拒绝 Seedance 2.0 Fast/Mini 等未收录变体，只接受两种 WxArt Seedance 模型及已登记 canonical 映射。
- 2.0 时长为 4-15 秒、分辨率为 480p/720p/1080p/4k，参考素材限制为 9/3/3/12；2.5 时长为 4-30 秒、分辨率为 480p/720p，限制为 30/10/10/50。
- 参考媒体使用现有视频/音频元数据服务；无效媒体返回 400，元数据服务不可用返回服务错误，不伪造时长。
- 成功终态返回结果 URL；失败终态把上游 `video_url` 当作脱敏错误原因，不向 Ark 暴露失败 URL 或私有字段。
- 提交任务时将 WxArt 选中 Key 仅保存于任务私有数据，供失败错误脱敏使用；公开任务序列化不包含该 Key。
- 模板和导入映射为 `17 -> CH-WXART -> 215`，管理端只显示两种模型并默认禁用，七种 locale 无缺失键。
- 收录模板的六个清晰度能力行已按模型边界登记：Seedance 2.0 的 480p、720p、1080p、4k，以及 Seedance 2.5 的 480p、720p；价格仍由导入数据提供。

## 测试证据

本轮复核通过：

```text
go test ./relay/channel/task/newapivideo ./relay ./service ./controller ./constant ./pkg/modelrouting ./pkg/seedancepricing -count=1
bun test --timeout=30000 tests/channel-type-config.test.ts --test-name-pattern='WxArt'
bun test --timeout=30000 src/features/channels/lib/__tests__/new-api-channel.test.ts --test-name-pattern='pre-acceptance|New API channel'
bun test --timeout=30000 scripts/channel-model-template/__tests__/build.test.ts --test-name-pattern='wxart'
bun test --timeout=30000 src/channel-config-converter/__tests__/v1.test.ts --test-name-pattern='reserved YSR'
bun run typecheck
bun run build
bun run i18n:sync
git diff --check
```

以上命令均返回 0。受影响的 Go 包和 WxArt 定向前端测试均通过。

本轮收尾复核重新执行了上述 Go 聚焦套件、WxArt/模板/导入前端定向测试、`bun run typecheck`、`bun run build` 和 `git diff --check`，均返回 0。

对本次触及的前端文件执行定向 `oxlint` 也返回 0；同时将 `build.ts` 中原有的嵌套三元表达式改为等价的 `if/else`，没有改变导入逻辑。

补充限制：完整的 `bun test --parallel=1 scripts/channel-model-template src/channel-config-converter tests/channel-type-config.test.ts src/features/channels/lib/__tests__`
在当前机器上有 5 个既有导入夹具用例超过 Bun 默认 5 秒；提高到 30 秒后仍有一个慢用例超过 30 秒，超时发生在测试回调耗时而非断言失败。该套件未作为全量通过证据。
`go test ./... -count=1` 仍会被仓库已有的 `outputs/2026-08-12-sd-refresh-0819` 目录中的多个同包 `main` 函数阻断；未修改该无关目录。
全量 `bun run lint` 仍被仓库既有错误阻断，错误分布在多个未涉及本次渠道的前端模块；本次触及文件已通过定向 lint。

## 未执行与启用条件

当前没有真实 WxArt API Key，因此没有执行真实上游提交、轮询、多模态 Canary 或账务对账，不能宣称真实上游验收通过。
在绑定 Key、审阅收录表成本/利润规则并完成文本、首尾帧、三类参考媒体、完整轮询、失败退款和账务核对前，WxArt
渠道必须保持 disabled。

# OpenAI Images 统一路由真实 Canary 验收

## 范围

- 服务：`http://127.0.0.1:3000`
- 公共模型：`gpt-image-2`
- 请求接口：`POST /v1/images/generations`
- SKU：`gen-1024x1024-medium`
- 请求参数：`1024x1024`、`medium`、`n=1`、`b64_json`
- 渠道：41、42，均为 OpenAI Images 兼容配置，分组 `default`
- 路由策略：`lowest_cost`，严格模式，最低毛利率 `2000 bps`
- 测试凭据：仅用于本地 Canary，请求记录和本文不保存 API Key

## 真实请求结果

| 渠道 | HTTP | 图片数量 | 解码结果 | 图片字节数 | 请求耗时 | 成本规则 |
| --- | ---: | ---: | --- | ---: | ---: | --- |
| 41 | 200 | 1 | PNG，Base64 解码成功 | 789,361 | 28.7 秒 | 7140，`0.008 USD/image` |
| 42 | 200 | 1 | PNG，Base64 解码成功 | 897,312 | 44.0 秒 | 7141，`0.012 USD/image` |

渠道 42 的请求在验收期间临时隔离渠道 41 以固定路由目标；测试结束后渠道 41 的状态和模型能力已恢复为启用，服务重启后健康检查通过。当前 41、42 均为启用状态。

## 账务核对

- 渠道 41：消费日志 `6253`，成本核算请求 `4809`，`settled/complete`；用户收入 `0.030 USD`，供应商成本 `0.008 USD`，毛利 `0.022 USD`，毛利率约 `73.33%`。
- 渠道 42：消费日志 `6254`，成本核算请求 `4810`，`settled/complete`；用户收入 `0.030 USD`，供应商成本 `0.012 USD`，毛利 `0.018 USD`，毛利率 `60.00%`。
- 两条请求均为 `per_image`、`response_succeeded`、`validated_request`，预扣费与结算完成，无失败退款或未结算记录。

## 代码与服务验证

- 统一图像路由已保留选中渠道凭据，严格利润复核使用冻结的图像售价快照。
- `go test ./service -run 'TestUnifiedImageRouting|TestValidateKnownImageChannelRequiresProfileAndPublishesIdentity|TestStrictImageProfitRecheckUsesFrozenImageRevenue' -count=1`：通过。
- `go test ./service -run 'Test.*Image' -count=1`：通过。
- `git diff --check`：通过。
- 本地 new-api、MySQL、Redis、video-metadata 容器均健康；`GET /api/status` 返回 `200`。

## 结论与边界

本次真实 Canary 已验证 `gpt-image-2` 通过统一 OpenAI Images generations 路由、渠道选择、上游出图、Base64 返回、预扣费、结算、成本和利润审计的完整闭环。当前 `lowest_cost` 会优先使用渠道 41；渠道 42 仅在渠道 41 不可用或被排除时回退。不同成本渠道不会自动按比例混流，若要实现例如 80/20 的动态成本加权，需要另行启用 `cost_weighted` 策略实现。

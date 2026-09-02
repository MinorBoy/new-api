# 图像定价工作台浏览器验收记录

## 环境

- 服务地址：`http://127.0.0.1:3000`
- 运行方式：本机 Docker Compose 容器
- 验收页面：`/system-settings/billing/image-pricing`
- 验收账号：本地管理员账号（凭据不写入本文）

## 浏览器验收

管理员登录后，图像定价工作台正常显示 `gpt-image-2` 的两个 SKU：

- `gen-1024x1024-medium`：上游成本 `$0.02`，用户售价 `$0.03`，预计毛利率 `33.33%`
- `gen-4096x4096-medium`：上游成本 `$0.08`，用户售价 `$0.12`，预计毛利率 `33.33%`

已完成以下操作并观察到预期结果：

1. 将 1K SKU 售价临时改为 `0.031`，保存成功且未保存计数清零。
2. 刷新页面后临时价格保持，再恢复为原售价 `0.03` 并保存；刷新后恢复值保持。
3. 展开高级设置，执行图像路由预览，返回策略、SKU、候选渠道和选中渠道信息。
4. 设置移动视口 `390x844`，价格表仍可见，表格容器 `scrollWidth=900`、`clientWidth=397`，可横向滚动。

## 真实 Canary

本轮真实 `/v1/images/generations` 请求确实进入渠道 `41`，但上游 `api.mikoto.vip` 在约 60 秒后返回 `unexpected EOF`，本地最终返回 HTTP `500`。未生成有效图片，未继续请求渠道 `42`，避免额外消耗测试额度。

日志显示本次预扣费已返还；对应成本记录仍为 `pending / incomplete_revenue`，因此真实上游账务闭环本轮不判定为通过。

## 自动化验证

- `bun test src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`：10 项通过。
- `go test ./service ./controller ./router -run 'Image|CostAccounting|Routing' -count=1 -p=1`：通过。
- `bunx playwright test e2e/image-pricing-workbench.pw.ts --config=playwright.config-import.config.ts`：未设置认证环境变量时 2 项跳过；本次使用已登录本地浏览器会话完成等价桌面和移动验收。
- `GET /api/status`：HTTP `200`；new-api、MySQL、Redis、video-metadata 容器均为 healthy。

## 结论

图像定价工作台的管理员浏览器流程通过；真实上游 Canary 受供应商连接异常影响未通过。后续应先处理上游连接和 `pending / incomplete_revenue` 收敛，再进行下一轮受控真实验收。

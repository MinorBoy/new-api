# Seedance 系列感知渠道合同 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以 `sd` 工作表 G 列系列生成的 canonical model 为权威，在静态路由合同和 Ark 请求运行时统一执行 Seedance 2.0 的 `9/3/3` 与 Seedance 2.5 的 `30/10/10`，解除系列误判造成的渠道草稿隔离并完成 Mock E2E。

**Architecture:** 在 `pkg/modelrouting` 建立唯一的系列能力合同，路由策略保存和批次激活显式把 canonical model 传给 relay 合同，Ark 请求层直接用原始请求模型选择系列。供应商模型名只控制上游转发和供应商专属规则；Paipu、ZZone、Dimensio 复用共享上限，4SToken 依赖公共 Ark 校验，OmegaAI 白名单保持不变。

**Tech Stack:** Go 1.22、Gin、Testify、Bun、TypeScript、ExcelJS、`@oai/artifact-tool`、Google 表格、new-api 配置导入状态机、Mock Ark SDK E2E。

---

## 文件结构

- Create: `pkg/modelrouting/seedance_series.go`：定义按 canonical model 查询的系列、素材和时长合同。
- Create: `pkg/modelrouting/seedance_series_test.go`：覆盖 2.0、Fast、Mini、2.5、稳定配置名称和未知旧别名。
- Modify: `pkg/modelrouting/validate.go`：策略校验复用共享系列合同。
- Modify: `service/route_contract.go`：渠道合同回调增加 canonical model 参数。
- Modify: `service/routing_policy.go`：保存策略时传递 `request.Model`。
- Modify: `service/config_import_activation.go`：激活预检时传递 `policy.Model`。
- Modify: `service/routing_policy_test.go`：确认手工保存传递 canonical model。
- Modify: `service/config_import_activation_test.go`：确认激活预检传递 canonical model。
- Modify: `relay/video_route_contract.go`：Paipu、ZZone、Dimensio 按系列校验路由目标。
- Modify: `relay/video_route_contract_test.go`：覆盖两个系列的静态合同边界。
- Modify: `relay/channel/task/newapivideo/native.go`：公共 Ark 素材校验按 `request.Model` 选择系列。
- Modify: `relay/channel/task/newapivideo/native_test.go`：提供可复用素材请求夹具并覆盖公共边界。
- Modify: `relay/channel/task/newapivideo/paipu_request.go`：删除写死的 `9/3/3` 常量。
- Modify: `relay/channel/task/newapivideo/paipu_request_test.go`：覆盖 2.5 边界和 2.0 回归。
- Modify: `relay/channel/task/newapivideo/zzone_request.go`：把旧 `4/3/1` 改为系列合同。
- Modify: `relay/channel/task/newapivideo/zzone_request_test.go`：覆盖 2.0 与 2.5 边界并保留专属字段限制。
- Modify: `relay/channel/task/newapivideo/fourstoken_request_test.go`：证明 4SToken 复用公共系列校验。
- Modify: `relay/channel/task/dimensio/translate.go`：分别接收 canonical model 与供应商模型，按系列校验后发送供应商模型。
- Modify: `relay/channel/task/dimensio/translate_test.go`：覆盖 2.5 素材边界和 JMG 2.0 总数特例。
- Modify: `relay/channel/task/dimensio/adaptor.go`：按系列校验时长并保留模型映射边界。
- Modify: `relay/channel/task/dimensio/adaptor_test.go`：覆盖 2.5 的 30 秒请求和映射后转发。
- Modify: `relay/channel/task/dimensio/e2e_test.go`：确认 canonical model 与上游模型职责分离。
- Modify: `cmd/ark-video-material-seed/main.go`：本地矩阵工具识别 `seedance-2.5` 并向路由合同传递运行时模型。
- Modify: `cmd/ark-video-material-seed/main_test.go`：覆盖 2.5 配置加载和合同检查。
- Modify: `e2e/seedance_material_matrix_e2e_test.go`：E2E 配置加载器识别 2.5。
- Modify: `web/scripts/channel-model-template/conversion-rules.json`：删除 Dimensio、Paipu、ZZone 共 15 条临时 `draft`，保留 OmegaAI 3 条。
- Modify: `docs/new-channels/sd收录.xlsx`：更新为已确认 R125=2.0 的最新版 Google 表格。
- Modify: `e2e/testdata/channel-config-v1.json`：替换为本轮选中配置 JSON。
- Create: `outputs/2026-08-23-sd-series-contract/*`：保存下载、模板、报告、选中配置和验收证据，不提交 Git。

### Task 1: 建立共享 Seedance 系列能力合同

**Files:**
- Create: `pkg/modelrouting/seedance_series.go`
- Create: `pkg/modelrouting/seedance_series_test.go`
- Modify: `pkg/modelrouting/validate.go`
- Test: `pkg/modelrouting/validate_test.go`

- [ ] **Step 1: 写系列合同失败测试**

新增表驱动测试，先引用尚不存在的 `SeedanceSeriesContractForModel`：

```go
func TestSeedanceSeriesContractForModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  modelrouting.SeedanceSeriesContract
	}{
		{name: "2.0", model: modelrouting.Seedance20, want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
		{name: "2.0 fast", model: modelrouting.Seedance20Fast, want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
		{name: "2.0 mini", model: modelrouting.Seedance20Mini, want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
		{name: "2.5 public", model: modelrouting.Seedance25, want: modelrouting.SeedanceSeriesContract{Series: "2.5", ReferenceLimits: modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, ReferenceTotalMax: 50, MaxDurationSeconds: 30}},
		{name: "2.5 config name", model: " seedance-2.5 ", want: modelrouting.SeedanceSeriesContract{Series: "2.5", ReferenceLimits: modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, ReferenceTotalMax: 50, MaxDurationSeconds: 30}},
		{name: "unknown stays conservative", model: "legacy-provider-alias", want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, modelrouting.SeedanceSeriesContractForModel(test.model))
		})
	}
}
```

- [ ] **Step 2: 写策略系列边界失败测试**

在 `validate_test.go` 增加：2.0 的 16 秒和图片 10 被拒绝；2.5 的 30 秒、`30/10/10`、总数 50 被接受；31 秒或任一素材维度加一被拒绝。

- [ ] **Step 3: 运行测试确认红灯**

```powershell
go test ./pkg/modelrouting -run 'TestSeedanceSeriesContractForModel|TestValidatePolicy.*Series' -count=1
```

Expected: 编译失败，提示 `SeedanceSeriesContractForModel` 或 `SeedanceSeriesContract` 未定义。

- [ ] **Step 4: 实现最小共享合同**

创建 `seedance_series.go`：

```go
package modelrouting

import "strings"

type SeedanceSeriesContract struct {
	Series             string
	ReferenceLimits    ReferenceLimits
	ReferenceTotalMax  int
	MaxDurationSeconds int
}

func SeedanceSeriesContractForModel(modelName string) SeedanceSeriesContract {
	if strings.TrimSpace(modelName) == Seedance25 || strings.TrimSpace(modelName) == "seedance-2.5" {
		return SeedanceSeriesContract{
			Series: "2.5", ReferenceLimits: ReferenceLimits{Images: 30, Videos: 10, Audios: 10},
			ReferenceTotalMax: 50, MaxDurationSeconds: 30,
		}
	}
	return SeedanceSeriesContract{
		Series: "2.0", ReferenceLimits: ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
		ReferenceTotalMax: 15, MaxDurationSeconds: 15,
	}
}
```

- [ ] **Step 5: 让策略校验复用合同**

在 `validateConstraints` 中替换本地系列常量：

```go
contract := SeedanceSeriesContractForModel(canonicalModel)
allowedModelResolutions := allowedResolutions
if contract.Series == "2.5" {
	allowedModelResolutions = []string{"480p", "720p"}
}
modelMaxDuration := maxDuration
if modelMaxDuration > contract.MaxDurationSeconds {
	modelMaxDuration = contract.MaxDurationSeconds
}
maxImages := contract.ReferenceLimits.Images
maxVideos := contract.ReferenceLimits.Videos
maxAudios := contract.ReferenceLimits.Audios
```

后续现有分辨率、时长、最小素材和聚合约束校验继续使用这些局部变量。

- [ ] **Step 6: 运行包测试确认绿灯**

```powershell
go test ./pkg/modelrouting -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交共享合同**

```powershell
git add -- pkg/modelrouting/seedance_series.go pkg/modelrouting/seedance_series_test.go pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go
git commit --only -- pkg/modelrouting/seedance_series.go pkg/modelrouting/seedance_series_test.go pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go -m "feat: centralize Seedance series limits"
```

### Task 2: 把 canonical model 传入渠道路由合同

**Files:**
- Modify: `service/route_contract.go`
- Modify: `service/routing_policy.go`
- Modify: `service/config_import_activation.go`
- Modify: `service/routing_policy_test.go`
- Modify: `service/config_import_activation_test.go`
- Modify: `relay/video_route_contract.go`
- Modify: `relay/video_route_contract_test.go`
- Modify: `cmd/ark-video-material-seed/main_test.go`

- [ ] **Step 1: 先修改服务测试要求 canonical model**

把现有 mock 改成三参数，并断言实际模型：

```go
service.RouteTargetContractValidator = func(channel *model.Channel, canonicalModel string, target modelrouting.Target) error {
	assert.Equal(t, modelrouting.Seedance20, canonicalModel)
	assert.Equal(t, 11, channel.Id)
	assert.Equal(t, "provider-standard", target.UpstreamModel)
	return errors.New("provider route contract rejected the target")
}
```

激活预检测试同样捕获第二个参数并断言为 fixture 策略的 `modelrouting.Seedance20`。

- [ ] **Step 2: 运行测试确认红灯**

```powershell
go test ./service -run 'TestSaveRoutingPolicyRejectsIncompatibleChannelContract|TestPreviewConfigImportBatchActivationRejectsContractMismatch' -count=1 -p=1
```

Expected: 编译失败，三参数函数不能赋给旧的两参数回调。

- [ ] **Step 3: 修改回调和 relay 入口签名**

```go
var RouteTargetContractValidator func(channel *model.Channel, canonicalModel string, target modelrouting.Target) error

func ValidateVideoRouteTargetContract(channel *model.Channel, canonicalModel string, target modelrouting.Target) error {
	// 原 switch 保持；只有需要系列的渠道在后续任务使用 canonicalModel。
}
```

在 `SaveRoutingPolicy` 传 `request.Model`，在激活预检传 `policy.Model`。

- [ ] **Step 4: 更新全部直接调用点**

`relay/video_route_contract_test.go` 的主表默认使用 `modelrouting.Seedance20`；2.5 用例显式使用 `modelrouting.Seedance25`。`cmd/ark-video-material-seed/main_test.go` 使用 `target.RuntimeModel`：

```go
err := relay.ValidateVideoRouteTargetContract(channel, target.RuntimeModel, modelrouting.Target{
	UpstreamModel: target.UpstreamModel,
	Constraints:   constraints,
})
```

用以下搜索确保没有旧调用：

```powershell
rg -n "ValidateVideoRouteTargetContract\(" -g "*.go"
```

- [ ] **Step 5: 运行定向测试确认绿灯**

```powershell
go test ./service ./relay ./cmd/ark-video-material-seed -run 'TestSaveRoutingPolicyRejectsIncompatibleChannelContract|TestPreviewConfigImportBatchActivationRejectsContractMismatch|TestValidateVideoRouteTargetContract|TestLoadTargetsMatchesRouteContractBlocks' -count=1 -p=1
```

Expected: PASS，行为尚未放宽。

- [ ] **Step 6: 提交 canonical model 数据流**

```powershell
git add -- service/route_contract.go service/routing_policy.go service/config_import_activation.go service/routing_policy_test.go service/config_import_activation_test.go relay/video_route_contract.go relay/video_route_contract_test.go cmd/ark-video-material-seed/main_test.go
git commit --only -- service/route_contract.go service/routing_policy.go service/config_import_activation.go service/routing_policy_test.go service/config_import_activation_test.go relay/video_route_contract.go relay/video_route_contract_test.go cmd/ark-video-material-seed/main_test.go -m "refactor: pass canonical model to video route contracts"
```

### Task 3: 让 Paipu、ZZone、Dimensio 静态路由合同感知系列

**Files:**
- Modify: `relay/video_route_contract.go`
- Modify: `relay/video_route_contract_test.go`

- [ ] **Step 1: 添加两个系列的失败测试**

在测试表加入 `canonicalModel` 字段，空值默认 2.0。增加以下代表性用例：

```go
{
	name: "paipu accepts Seedance 2.5 expanded route", channelType: constant.ChannelTypePaipu,
	canonicalModel: modelrouting.Seedance25,
	target: videoContractTargetWithTotal("jmg-video-seedance-2.5", []string{"720p"}, 4, 30, nil,
		modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, 50),
},
{
	name: "zzone accepts Seedance 2.0 933 route", channelType: constant.ChannelTypeZZone,
	canonicalModel: modelrouting.Seedance20,
	target: videoContractTargetWithTotal("video-ds-2.0", []string{"720p"}, 4, 15, nil,
		modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, 15),
},
{
	name: "zzone accepts Seedance 2.5 expanded route", channelType: constant.ChannelTypeZZone,
	canonicalModel: modelrouting.Seedance25,
	target: videoContractTargetWithTotal("video-ds-2.5", []string{"720p"}, 4, 30, nil,
		modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, 50),
},
{
	name: "dimensio accepts Seedance 2.5 expanded route", channelType: constant.ChannelTypeDimensio,
	canonicalModel: modelrouting.Seedance25,
	target: videoContractTargetWithTotal("jmg-video-seedance-2.5", []string{"720p"}, 4, 30, nil,
		modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, 50),
},
```

同时把 ZZone 原来的越界用例更新为图片 10、视频 4、音频 4，并为三个渠道增加总数 51、2.0 时长 16、2.5 时长 31 的拒绝用例。保留 Dimensio JMG 2.0 总数 12 特例。

- [ ] **Step 2: 运行测试确认红灯**

```powershell
go test ./relay -run 'TestValidateVideoRouteTargetContract' -count=1
```

Expected: Paipu、ZZone、Dimensio 的 2.5 扩展合同被旧常量拒绝；ZZone 2.0 的 `9/3/3` 被 `4/3/1` 拒绝。

- [ ] **Step 3: 用共享合同实现路由边界**

三个函数接收 `canonicalModel`，读取：

```go
contract := modelrouting.SeedanceSeriesContractForModel(canonicalModel)
limits := target.Constraints.ReferenceLimits
minimums := target.Constraints.ReferenceMinimums
if limits.Images > contract.ReferenceLimits.Images ||
	limits.Videos > contract.ReferenceLimits.Videos ||
	limits.Audios > contract.ReferenceLimits.Audios ||
	minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios ||
	routeReferenceTotalMax(target.Constraints) > contract.ReferenceTotalMax {
	return newVideoRouteContractError("route_contract_references", "route reference limits exceed Seedance "+contract.Series+" limits")
}
```

Paipu、ZZone 使用 1 至系列最大时长；Dimensio 使用 4 至系列最大时长。Dimensio 只有在系列为 2.0 且上游模型以 `jmg-` 开头时把总数进一步限制为 12。

- [ ] **Step 4: 保持供应商专属约束**

不要删除 ZZone 比例白名单、Dimensio 分辨率规则、空上游模型检查或 WxArt 已有分支。

- [ ] **Step 5: 运行 relay 测试确认绿灯**

```powershell
go test ./relay -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交静态合同修复**

```powershell
git add -- relay/video_route_contract.go relay/video_route_contract_test.go
git commit --only -- relay/video_route_contract.go relay/video_route_contract_test.go -m "fix: validate video routes by Seedance series"
```

### Task 4: 让公共 Ark、Paipu、ZZone、4SToken 请求校验感知系列

**Files:**
- Modify: `relay/channel/task/newapivideo/native.go`
- Modify: `relay/channel/task/newapivideo/native_test.go`
- Modify: `relay/channel/task/newapivideo/paipu_request.go`
- Modify: `relay/channel/task/newapivideo/paipu_request_test.go`
- Modify: `relay/channel/task/newapivideo/zzone_request.go`
- Modify: `relay/channel/task/newapivideo/zzone_request_test.go`
- Modify: `relay/channel/task/newapivideo/fourstoken_request_test.go`

- [ ] **Step 1: 添加共享测试请求夹具**

在 `native_test.go` 添加多调用方夹具，使用项目 JSON 包装器生成合法公共 URL：

```go
func arkReferenceMediaBody(t *testing.T, modelName string, images, videos, audios int) []byte {
	t.Helper()
	content := []map[string]any{{"type": "text", "text": "series boundary"}}
	for index := 0; index < images; index++ {
		content = append(content, map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": fmt.Sprintf("https://8.8.8.%d/image.png", index%250+1)}})
	}
	for index := 0; index < videos; index++ {
		content = append(content, map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": fmt.Sprintf("https://8.8.4.%d/video.mp4", index%250+1)}})
	}
	for index := 0; index < audios; index++ {
		content = append(content, map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": fmt.Sprintf("https://1.1.1.%d/audio.mp3", index%250+1)}})
	}
	body, err := common.Marshal(map[string]any{"model": modelName, "content": content})
	require.NoError(t, err)
	return body
}
```

- [ ] **Step 2: 写公共校验失败测试**

表驱动覆盖 2.0 的 `9/3/3` 和 15 秒成功，任一素材维度加一或 16 秒失败；2.5 的 `30/10/10` 和 30 秒成功，任一素材维度加一或 31 秒失败；未知旧别名的图片 10 和 16 秒失败。

- [ ] **Step 3: 写三个适配器失败测试**

Paipu 和 ZZone 分别把 2.5 exact boundary 直接送入 `buildPaipuRequest`、`buildZZoneRequest`，预期成功；2.0 的 10/4/4 与 2.5 的 31/11/11 预期 `InvalidParameter.content`。4SToken 通过 `parseARKRequest(..., fourSTokenProtocolProfile())` 与 `buildFourSTokenRequest` 验证 2.5 exact boundary 可通过。

- [ ] **Step 4: 运行测试确认红灯**

```powershell
go test ./relay/channel/task/newapivideo -run 'Test.*SeriesMediaLimits|Test.*Seedance25.*Boundary' -count=1
```

Expected: 2.5 exact boundary 被公共 `9/3/3`、Paipu 常量或 ZZone `4/3/1` 拒绝。

- [ ] **Step 5: 修改公共 Ark 校验**

在 `validateARKSemantics` 开始处读取 `request.Model`，让时长和素材共用同一合同：

```go
contract := modelrouting.SeedanceSeriesContractForModel(request.Model)
maximumDuration := contract.MaxDurationSeconds
if maximumDuration > relaycommon.MaxTaskDurationSeconds {
	maximumDuration = relaycommon.MaxTaskDurationSeconds
}
if request.Duration != nil && (*request.Duration <= 0 || *request.Duration > maximumDuration) {
	return &arkRequestError{
		Code: "InvalidParameter.duration",
		Message: fmt.Sprintf("duration must be between 1 and %d for Seedance %s", maximumDuration, contract.Series),
	}
}

if imageCount > contract.ReferenceLimits.Images || videoCount > contract.ReferenceLimits.Videos || audioCount > contract.ReferenceLimits.Audios {
	return &arkRequestError{
		Code: "InvalidParameter.content",
		Message: fmt.Sprintf("reference media count exceeds Seedance %s limits (%d images, %d videos, %d audios)",
			contract.Series, contract.ReferenceLimits.Images, contract.ReferenceLimits.Videos, contract.ReferenceLimits.Audios),
	}
}
```

- [ ] **Step 6: 修改 Paipu 与 ZZone 专属校验**

删除 Paipu 三个固定常量；两个适配器都用 `request.Model` 取得共享合同。保留角色、URL、字段、分辨率和比例校验。4SToken 不增加重复生产逻辑。

- [ ] **Step 7: 运行适配器测试确认绿灯**

```powershell
go test ./relay/channel/task/newapivideo -count=1
```

Expected: PASS。

- [ ] **Step 8: 提交运行时 Ark 修复**

```powershell
git add -- relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go relay/channel/task/newapivideo/paipu_request.go relay/channel/task/newapivideo/paipu_request_test.go relay/channel/task/newapivideo/zzone_request.go relay/channel/task/newapivideo/zzone_request_test.go relay/channel/task/newapivideo/fourstoken_request_test.go
git commit --only -- relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go relay/channel/task/newapivideo/paipu_request.go relay/channel/task/newapivideo/paipu_request_test.go relay/channel/task/newapivideo/zzone_request.go relay/channel/task/newapivideo/zzone_request_test.go relay/channel/task/newapivideo/fourstoken_request_test.go -m "fix: validate Ark references by Seedance series"
```

### Task 5: 修复 Dimensio canonical model 与供应商模型覆盖

**Files:**
- Modify: `relay/channel/task/dimensio/translate.go`
- Modify: `relay/channel/task/dimensio/translate_test.go`
- Modify: `relay/channel/task/dimensio/adaptor.go`
- Modify: `relay/channel/task/dimensio/adaptor_test.go`
- Modify: `relay/channel/task/dimensio/e2e_test.go`

- [ ] **Step 1: 写翻译器系列失败测试**

构造 2.5 canonical request，分别验证 `30/10/10`、总数 50 成功，图片 31、视频 11、音频 11、总数 51 失败。继续验证 2.0 `jmg-*` 的总数 13 失败，而 2.5 `jmg-*` 不继承 12 条旧特例。

- [ ] **Step 2: 写时长与上游模型失败测试**

在 adaptor 测试中提交 `modelrouting.Seedance25 + duration=30`，`info.UpstreamModelName="jmg-video-seedance-2.5"`，断言请求校验和 `EstimateDurationSeconds` 成功。构建请求体后断言：

```go
assert.JSONEq(t, `{
	"model":"jmg-video-seedance-2.5",
	"prompt":"series boundary",
	"functionMode":"first_last_frames",
	"duration":30,
	"resolution":"720p",
	"ratio":"16:9"
}`, string(body))
```

再断言 2.0 的 16 秒和 2.5 的 31 秒返回 `invalid_duration`。

- [ ] **Step 3: 运行测试确认红灯**

```powershell
go test ./relay/channel/task/dimensio -run 'Test.*Seedance25|Test.*SeriesMedia|Test.*Duration' -count=1
```

Expected: 2.5 素材被 `9/3/3` 拒绝，30 秒被 15 秒上限拒绝，或校验因供应商模型覆盖 canonical model 而回落到 2.0。

- [ ] **Step 4: 分离能力模型与上游模型**

把翻译函数改为显式双模型输入，完整主体如下：

```go
func ArkToDimensio(ark ArkRequest, upstreamModel string) (DimensioRequest, error) {
	if err := validateUnsupportedFields(ark); err != nil {
		return DimensioRequest{}, err
	}
	modelName := strings.TrimSpace(upstreamModel)
	if modelName == "" {
		modelName = ark.Model
	}
	if err := validateArkContentRoles(ark.Content, ark.Model, modelName); err != nil {
		return DimensioRequest{}, err
	}

	dim := DimensioRequest{
		Model: modelName, Ratio: ark.Ratio, Resolution: ark.Resolution, Duration: ark.Duration,
		IntelligentRatio: ark.IntelligentRatio, FaceGrid: ark.FaceGrid,
		FilePaths: []string{}, ImageFiles: map[string]string{}, VideoFiles: map[string]string{}, AudioFiles: map[string]string{},
	}
	imageIndex, videoIndex, audioIndex := 0, 0, 0
	for _, item := range ark.Content {
		switch item.Type {
		case "text":
			if dim.Prompt == "" && strings.TrimSpace(item.Text) != "" {
				dim.Prompt = item.Text
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("image_url.url is required")
			}
			imageIndex++
			dim.ImageFiles[fmt.Sprintf("image_file_%d", imageIndex)] = item.ImageURL.URL
			dim.FilePaths = append(dim.FilePaths, item.ImageURL.URL)
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("video_url.url is required")
			}
			videoIndex++
			dim.VideoFiles[fmt.Sprintf("video_file_%d", videoIndex)] = item.VideoURL.URL
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("audio_url.url is required")
			}
			audioIndex++
			dim.AudioFiles[fmt.Sprintf("audio_file_%d", audioIndex)] = item.AudioURL.URL
		default:
			return DimensioRequest{}, fmt.Errorf("unsupported content type: %s", item.Type)
		}
	}
	if strings.TrimSpace(dim.Prompt) == "" {
		return DimensioRequest{}, fmt.Errorf("text prompt is required")
	}
	dim.FunctionMode = deriveFunctionMode(ark.Content)
	return dim, nil
}
```

`validateArkContentRoles` 用 `ark.Model` 读取共享系列合同；只有 `contract.Series == "2.0"` 且 `upstreamModel` 以 `jmg-` 开头时总数限制为 12。

- [ ] **Step 5: 修改 adaptor 的时长和调用点**

`ValidateRequestAndSetAction`、`EstimateDurationSeconds` 根据存储请求的 canonical model 使用系列最大时长；不要再创建覆盖 `Model` 的 `validationRequest`。`BuildRequestBody` 把原始请求和 `info.UpstreamModelName` 分开传给翻译器。

- [ ] **Step 6: 更新所有翻译器测试调用**

现有测试显式传空上游模型以保持输出模型等于请求模型；涉及真实转发的测试传映射后的模型。用搜索保证没有旧签名：

```powershell
rg -n "ArkToDimensio\(" relay/channel/task/dimensio -g "*.go"
```

- [ ] **Step 7: 运行 Dimensio 完整测试确认绿灯**

```powershell
go test ./relay/channel/task/dimensio -count=1
```

Expected: PASS。

- [ ] **Step 8: 提交 Dimensio 修复**

```powershell
git add -- relay/channel/task/dimensio/translate.go relay/channel/task/dimensio/translate_test.go relay/channel/task/dimensio/adaptor.go relay/channel/task/dimensio/adaptor_test.go relay/channel/task/dimensio/e2e_test.go
git commit --only -- relay/channel/task/dimensio/translate.go relay/channel/task/dimensio/translate_test.go relay/channel/task/dimensio/adaptor.go relay/channel/task/dimensio/adaptor_test.go relay/channel/task/dimensio/e2e_test.go -m "fix: preserve Dimensio series during model mapping"
```

### Task 6: 让素材矩阵工具识别 Seedance 2.5

**Files:**
- Modify: `cmd/ark-video-material-seed/main.go`
- Modify: `cmd/ark-video-material-seed/main_test.go`
- Modify: `e2e/seedance_material_matrix_e2e_test.go`

- [ ] **Step 1: 写 2.5 映射失败测试**

在两个包分别断言稳定配置名映射为运行时模型：

```go
func TestRuntimeModelSupportsSeedance25(t *testing.T) {
	assert.Equal(t, modelrouting.Seedance25, runtimeModel("seedance-2.5"))
}
```

E2E 包对 `importedMaterialMatrixRuntimeModel("seedance-2.5")` 做同样断言。更新 `modelroutingCanonicalModels()` 的期望集合，包含 `seedance-2.5`。

- [ ] **Step 2: 运行测试确认红灯**

```powershell
go test ./cmd/ark-video-material-seed ./e2e -run 'TestRuntimeModelSupportsSeedance25|TestImportedMaterialMatrixRuntimeModelSupportsSeedance25' -count=1 -p=1
```

Expected: 两个函数返回空字符串。

- [ ] **Step 3: 增加 2.5 分支**

两个映射函数都增加：

```go
case "seedance-2.5":
	return modelrouting.Seedance25
```

本地矩阵合同检查使用 `target.RuntimeModel` 作为新增的 canonical model 参数。

- [ ] **Step 4: 运行工具和 E2E 加载测试**

```powershell
go test ./cmd/ark-video-material-seed ./e2e -run 'TestRuntimeModel|TestLoadTargets|TestImportedMaterialMatrix' -count=1 -p=1
```

Expected: PASS。

- [ ] **Step 5: 提交矩阵支持**

```powershell
git add -- cmd/ark-video-material-seed/main.go cmd/ark-video-material-seed/main_test.go e2e/seedance_material_matrix_e2e_test.go
git commit --only -- cmd/ark-video-material-seed/main.go cmd/ark-video-material-seed/main_test.go e2e/seedance_material_matrix_e2e_test.go -m "test: cover Seedance 2.5 material matrix"
```

### Task 7: 刷新规则、Google 源表、模板和导入配置

**Files:**
- Modify: `web/scripts/channel-model-template/conversion-rules.json`
- Modify: `docs/new-channels/sd收录.xlsx`
- Modify: `e2e/testdata/channel-config-v1.json`
- Create: `outputs/2026-08-23-sd-series-contract/sd收录.xlsx`
- Create: `outputs/2026-08-23-sd-series-contract/渠道模型成本与利润模板-v1.xlsx`
- Create: `outputs/2026-08-23-sd-series-contract/渠道模型成本与利润模板-v1.report.json`
- Create: `outputs/2026-08-23-sd-series-contract/channel-config-v1.json`
- Create: `outputs/2026-08-23-sd-series-contract/channel-config-issues.json`

- [ ] **Step 1: 删除临时系列 draft 覆盖**

仅删除以下键，不改其他用户规则：

```text
7/R123  7/R124  7/R132  7/R133  7/R142  7/R143  7/R149  7/R150
2/R59   2/R60   2/R61
14/R248 14/R249 14/R250 14/R251
```

确认 `10/R216`、`10/R217`、`10/R218` 仍为 `draft`。这是配置文件修改，按用户批准的 TDD 例外使用真实模板报告验证，不增加只锁行号的单元测试。

- [ ] **Step 2: 从已登录表格重新下载源表**

使用 `browser:control-in-app-browser` 在用户指定 Google 表格执行“下载 -> Microsoft Excel (.xlsx)”，确认表格 ID 完全一致。创建唯一目录并记录哈希：

```powershell
New-Item -ItemType Directory -Path outputs/2026-08-23-sd-series-contract
Get-FileHash -Algorithm SHA256 outputs/2026-08-23-sd-series-contract/sd收录.xlsx
```

Expected: 文件来自本轮浏览器下载，不复用旧输出。

- [ ] **Step 3: 使用电子表格门禁校验**

调用 `codex_app__load_workspace_dependencies`，按 `spreadsheets:Spreadsheets` 使用 `artifact_tool` 检查并渲染 `channel`、`sd`、`sd官价`。必须确认：

```text
sd!G125 = 2.0
有效模型行 = 259
系列 2.0 = 242
系列 2.5 = 17
旧价格列与旧“视频输入”不存在
公式错误 = 0
```

若实际行数因用户后续编辑变化，记录差异并重新盘点，不强行改回预期计数。

- [ ] **Step 4: 从已验证副本生成模板**

```powershell
Set-Location web
bun run channel-model-template:generate -- `
  --source "..\outputs\2026-08-23-sd-series-contract\sd收录.xlsx" `
  --rules "scripts\channel-model-template\conversion-rules.json" `
  --base "src\channel-config-converter\__fixtures__\channel-config-v1-corrected.xlsx" `
  --output "C:\Users\880pro\Documents\new-api\outputs\2026-08-23-sd-series-contract\渠道模型成本与利润模板-v1.xlsx" `
  --report "C:\Users\880pro\Documents\new-api\outputs\2026-08-23-sd-series-contract\渠道模型成本与利润模板-v1.report.json" `
  --allow-warnings
Set-Location ..
```

Expected: `FAIL=0`；Dimensio、Paipu、ZZone 不再因系列上限进入 draft；OmegaAI 3 条仍有明确隔离说明。

- [ ] **Step 5: 复核模板并更新仓库源表**

用电子表格流程检查模板所有受管理工作表、公式显示值、利润样本和渲染。通过后执行：

```powershell
Copy-Item -LiteralPath outputs/2026-08-23-sd-series-contract/sd收录.xlsx -Destination docs/new-channels/sd收录.xlsx
```

- [ ] **Step 6: 通过页面导出选中配置 JSON**

使用现有登录态打开 `/config-import`，上传本轮模板，选择类型、Base URL、分组和凭据均匹配的已有线路。下载“选中配置 JSON”和问题报告到本轮目录，确认 `manifest.source_sha256` 与模板一致、转换 error 为 0。

- [ ] **Step 7: 更新 E2E fixture**

确认选中 JSON 不含凭据并核对实体计数后：

```powershell
Copy-Item -LiteralPath outputs/2026-08-23-sd-series-contract/channel-config-v1.json -Destination e2e/testdata/channel-config-v1.json
```

- [ ] **Step 8: 提交受版本控制的刷新结果**

```powershell
git add -- web/scripts/channel-model-template/conversion-rules.json docs/new-channels/sd收录.xlsx e2e/testdata/channel-config-v1.json
git commit --only -- web/scripts/channel-model-template/conversion-rules.json docs/new-channels/sd收录.xlsx e2e/testdata/channel-config-v1.json -m "chore: refresh Seedance series channel config"
```

不要提交 `outputs/` 下载物、运行日志或浏览器导出证据。

### Task 8: 导入激活、完整验证和验收报告

**Files:**
- Create: `outputs/2026-08-23-sd-series-contract/e2e.log`
- Create: `outputs/2026-08-23-sd-series-contract/验收报告.md`

- [ ] **Step 1: 运行格式化和 focused tests**

```powershell
gofmt -w pkg/modelrouting/seedance_series.go pkg/modelrouting/seedance_series_test.go pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go service/route_contract.go service/routing_policy.go service/config_import_activation.go service/routing_policy_test.go service/config_import_activation_test.go relay/video_route_contract.go relay/video_route_contract_test.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go relay/channel/task/newapivideo/paipu_request.go relay/channel/task/newapivideo/paipu_request_test.go relay/channel/task/newapivideo/zzone_request.go relay/channel/task/newapivideo/zzone_request_test.go relay/channel/task/newapivideo/fourstoken_request_test.go relay/channel/task/dimensio/translate.go relay/channel/task/dimensio/translate_test.go relay/channel/task/dimensio/adaptor.go relay/channel/task/dimensio/adaptor_test.go relay/channel/task/dimensio/e2e_test.go cmd/ark-video-material-seed/main.go cmd/ark-video-material-seed/main_test.go e2e/seedance_material_matrix_e2e_test.go
go test ./pkg/modelrouting ./service ./relay ./relay/channel/task/newapivideo ./relay/channel/task/dimensio ./cmd/ark-video-material-seed -count=1 -p=1
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
Set-Location ..
```

Expected: 所有命令退出码 0。

- [ ] **Step 2: 按状态机导入并激活本地批次**

在 `/config-import` 依次完成：

```text
导入选中配置
-> 绑定真实渠道并暂存
-> 解决冲突并重新暂存
-> 定价审阅并重新暂存
-> 路由差异审阅
-> 校验
-> 发布
-> 激活
-> 必要时刷新缓存
```

记录批次 ID、绑定、发布、激活、退休和启用计数。任何 `ACTIVATION_CHANNEL_CONTRACT`、成本、价格或映射 blocker 都停止后续步骤，不绕过。

- [ ] **Step 3: 运行 Mock 素材矩阵 E2E**

```powershell
go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1 2>&1 | Tee-Object outputs/2026-08-23-sd-series-contract/e2e.log
```

Expected: PASS；配置导入、路由、Ark 提交/查询、任务日志、使用日志、供应商成本和利润均有断言。

- [ ] **Step 4: 运行完整 focused 回归**

```powershell
go test ./cmd/ark-video-material-seed ./e2e -count=1 -p=1
```

Expected: PASS。

- [ ] **Step 5: 审计 draft 与系列边界**

从生成报告、选中 JSON和激活结果核对：

```text
Dimensio 系列临时 draft = 0
Paipu 系列临时 draft = 0
ZZone 系列临时 draft = 0
OmegaAI 白名单 draft = 3
Seedance 2.0 最大素材 = 9/3/3，最大时长 = 15
Seedance 2.5 最大素材 = 30/10/10，总数 = 50，最大时长 = 30
真实供应商请求 = 0
```

- [ ] **Step 6: 写简体中文验收报告**

`验收报告.md` 必须记录源表、模板、生成报告、选中 JSON 的绝对路径与 SHA-256；模板计数与剩余 draft；批次 ID 和激活统计；测试命令、退出码和 Mock 结果；未覆盖项与真实上游未执行说明。

- [ ] **Step 7: 最终工作树审计**

```powershell
git status --short
git diff --check
git log --oneline -8
```

Expected: 无空白错误；用户原有 Dimensio skill 和相关暂存文件仍保持原状态；`outputs/` 未进入提交。

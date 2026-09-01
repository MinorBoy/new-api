import fs from "node:fs/promises";
import { FileBlob, SpreadsheetFile } from "@oai/artifact-tool";

const outputDir = process.env.OUTPUT_DIR;
const sourceXlsx = process.env.SOURCE_XLSX;
const apiJsonPath = process.env.API_JSON;
const apiHeadersPath = process.env.API_HEADERS;

const api = JSON.parse(await fs.readFile(apiJsonPath, "utf8"));
const sourceXlsxBlob = await FileBlob.load(sourceXlsx);
const workbook = await SpreadsheetFile.importXlsx(sourceXlsxBlob);
const sd = workbook.worksheets.getItem("sd");
const values = sd.getRange("A1:AJ1096").values;
const headers = values[1];
const index = Object.fromEntries(headers.map((value, column) => [String(value), column]));

const targetRows = values
  .map((row, position) => ({ row: position + 1, values: row }))
  .filter(({ row, values: rowValues }) => row > 2 && Number(rowValues[index["渠道"]]) === 14);

const targetSnapshot = targetRows.map(({ row, values: rowValues }) => ({
  row,
  values: Object.fromEntries(headers.map((header, column) => [String(header ?? `column_${column + 1}`), rowValues[column] ?? null])),
}));

const h3 = workbook.worksheets.getItem("h3");
const h3Values = h3.getRange("A1:AF76").values;
const h3Headers = h3Values[1];
const h3Index = Object.fromEntries(h3Headers.map((value, column) => [String(value), column]));
const h3TargetSnapshot = h3Values
  .map((row, position) => ({ row: position + 1, values: row }))
  .filter(({ row, values: rowValues }) => row > 2 && Number(rowValues[h3Index["渠道"]]) === 14)
  .map(({ row, values: rowValues }) => ({
    row,
    values: Object.fromEntries(h3Headers.map((header, column) => [String(header ?? `column_${column + 1}`), rowValues[column] ?? null])),
  }));

const durationRange = /(\d+(?:\.\d+)?)\s*(?:-|~|至)\s*(\d+(?:\.\d+)?)\s*(?:秒|s|sec(?:onds)?)/gi;
const durationList = /(?:支持|时长(?:为)?)\s*((?:\d+(?:\.\d+)?\s*[、,/]\s*)+\d+(?:\.\d+)?)\s*(?:秒|s|sec(?:onds)?)/gi;
const ratioLabeled = /(?:比例|aspect\s*ratio|画幅)\s*[:：]?\s*(\d{1,2})\s*[:：/]\s*(\d{1,2})/gi;
const ratioKnown = /(?<!\d)(?:1:1|16:9|9:16|4:3|3:4|21:9)(?!\d)/gi;
const resolution = /(?<![A-Za-z0-9])(?:480p|720p|1080p|1440p|2160p|2k|4k)(?![A-Za-z0-9])/gi;
const versionMarked = /(?:\b(?:v|version)\s*[-_:]?\s*(\d+(?:\.\d+)*)\b|版本\s*[:：]?\s*([A-Za-z0-9][A-Za-z0-9._-]*))/gi;
const billingSecond = /(?:按秒|逐秒|per[- ]seconds?)/gi;
const billingCall = /(?:按次|每次|per[- ]request|per successful call)/gi;
const priceCny = /(?:¥|￥|CNY|RMB|人民币)\s*\d+(?:\.\d+)?|\d+(?:\.\d+)?\s*(?:元|CNY|RMB)/gi;
const refImage = /(?:(?:最多|至多|up to)\s*)?(\d+)\s*(?:张\s*)?(?:参考\s*)?(?:图片|图像|reference images?)/gi;
const refVideo = /(?:(?:最多|至多|up to)\s*)?(\d+)\s*(?:个\s*)?(?:参考\s*)?(?:视频|reference videos?)/gi;
const refAudio = /(?:(?:最多|至多|up to)\s*)?(\d+)\s*(?:个\s*)?(?:参考\s*)?(?:音频|reference audios?)/gi;
const materialTotal = /(?:(?:最多|至多)\s*(\d+)\s*(?:个|份)?\s*素材|up to\s*(\d+)\s*total\s*materials?)/gi;

function matches(text, pattern, regexId, normalize) {
  pattern.lastIndex = 0;
  return [...text.matchAll(pattern)].map((match) => ({
    regex_id: regexId,
    raw_match: match[0],
    normalized_value: normalize(match),
  }));
}

function textEvidence(model, dataIndex) {
  const sources = [
    { source_path: `data[${dataIndex}].model_name`, text: String(model.model_name ?? "") },
    { source_path: `data[${dataIndex}].description`, text: String(model.description ?? "") },
  ];
  const evidence = [];
  for (const source of sources) {
    evidence.push(...matches(source.text, durationRange, "ZZONE-DURATION-RANGE", (m) => ({ kind: "range", min: Number(m[1]), max: Number(m[2]), unit: "second" })).map((item) => ({ field: "duration", ...source, ...item })));
    evidence.push(...matches(source.text, durationList, "ZZONE-DURATION-LIST", (m) => ({ kind: "list", values: m[1].split(/[、,\/]/).map(Number), unit: "second" })).map((item) => ({ field: "duration", ...source, ...item })));
    evidence.push(...matches(source.text, ratioLabeled, "ZZONE-RATIO-LABELED", (m) => `${m[1]}:${m[2]}`).map((item) => ({ field: "ratio", ...source, ...item })));
    evidence.push(...matches(source.text, ratioKnown, "ZZONE-RATIO-KNOWN", (m) => m[0]).map((item) => ({ field: "ratio", ...source, ...item })));
    evidence.push(...matches(source.text, resolution, "ZZONE-RESOLUTION", (m) => m[0].toLowerCase().endsWith("p") ? m[0].toLowerCase() : m[0].toUpperCase()).map((item) => ({ field: "resolution", ...source, ...item })));
    evidence.push(...matches(source.text, versionMarked, "ZZONE-VERSION-MARKED", (m) => m[1] ?? m[2]).map((item) => ({ field: "version", ...source, ...item })));
    evidence.push(...matches(source.text, billingSecond, "ZZONE-BILLING-SECOND", () => "second").map((item) => ({ field: "billing_mode", ...source, ...item })));
    evidence.push(...matches(source.text, billingCall, "ZZONE-BILLING-CALL", () => "call").map((item) => ({ field: "billing_mode", ...source, ...item })));
    evidence.push(...matches(source.text, priceCny, "ZZONE-PRICE-CNY", (m) => m[0]).map((item) => ({ field: "price", ...source, ...item })));
    evidence.push(...matches(source.text, refImage, "ZZONE-REF-IMAGE", (m) => Number(m[1])).map((item) => ({ field: "reference_images", ...source, ...item })));
    evidence.push(...matches(source.text, refVideo, "ZZONE-REF-VIDEO", (m) => Number(m[1])).map((item) => ({ field: "reference_videos", ...source, ...item })));
    evidence.push(...matches(source.text, refAudio, "ZZONE-REF-AUDIO", (m) => Number(m[1])).map((item) => ({ field: "reference_audios", ...source, ...item })));
    evidence.push(...matches(source.text, materialTotal, "ZZONE-MATERIAL-TOTAL", (m) => Number(m[1] ?? m[2])).map((item) => ({ field: "material_total", ...source, ...item })));
  }
  return evidence;
}

const data = Array.isArray(api.data) ? api.data : [];
const modelIds = data.map((model) => String(model.model_name ?? ""));
const videoEndpointRecords = data.filter((model) => Array.isArray(model.supported_endpoint_types) && model.supported_endpoint_types.includes("videos"));
const ignoredNonSd = data.filter((model) => Array.isArray(model.supported_endpoint_types) && !model.supported_endpoint_types.includes("videos"));
const targetByModel = new Map();
for (const row of targetSnapshot) {
  const modelId = String(row.values["模型ID"] ?? "");
  if (!targetByModel.has(modelId)) targetByModel.set(modelId, []);
  targetByModel.get(modelId).push(row);
}

const normalized = data.map((model, dataIndex) => {
  const endpointTypes = Array.isArray(model.supported_endpoint_types) ? model.supported_endpoint_types : [];
  const isVideo = endpointTypes.includes("videos");
  const evidence = textEvidence(model, dataIndex);
  const billingMode = model.video_billing_mode === "per_request" ? "call" : model.video_billing_mode === "per_second" ? "second" : null;
  const unitPrice = model.video_unit_price ?? model.model_price ?? null;
  const bothPricesConflict = model.video_unit_price != null && model.model_price != null && Number(model.video_unit_price) !== Number(model.model_price);
  const priceUnit = Object.entries(model).find(([key]) => /^(currency|price_unit|unit)$/.test(key) && /cny|rmb|人民币|元/i.test(String(model[key]))) ? "CNY" : evidence.some((item) => item.field === "price") ? "CNY" : "unknown";
  let decision = isVideo ? "candidate" : "ignored_non_sd";
  if (isVideo && bothPricesConflict) decision = "draft_conflict";
  if (isVideo && (!billingMode || !Number.isFinite(Number(unitPrice)) || priceUnit !== "CNY")) decision = decision === "draft_conflict" ? decision : "draft";
  return {
    model_id: model.model_name,
    series: null,
    version: evidence.filter((item) => item.field === "version").map((item) => item.normalized_value),
    resolution: evidence.filter((item) => item.field === "resolution").map((item) => item.normalized_value),
    billing_mode: billingMode,
    unit_price: unitPrice,
    price_unit: priceUnit,
    price_source: { video_unit_price: model.video_unit_price ?? null, model_price: model.model_price ?? null, description: model.description ?? null },
    supported_parameters: evidence.filter((item) => ["duration", "ratio", "reference_images", "reference_videos", "reference_audios", "material_total"].includes(item.field)),
    text_evidence: evidence,
    protocol: isVideo ? "videos" : endpointTypes,
    raw_source: model,
    decision,
  };
});

const sourceVideoIds = new Set(videoEndpointRecords.map((model) => String(model.model_name)));
const patch = [];
for (const row of targetSnapshot) {
  const modelId = String(row.values["模型ID"] ?? "");
  if (!sourceVideoIds.has(modelId)) {
    patch.push({ row: row.row, model_id: modelId, decision: "review_missing", reason: "API 视频候选中不存在；不删除、不改状态、不退休路由" });
  }
}
for (const record of normalized.filter((item) => item.decision === "draft" || item.decision === "draft_conflict")) {
  patch.push({ model_id: record.model_id, decision: record.decision, reason: record.price_unit !== "CNY" ? "价格为代币/平台计价或无人民币单位证据，不能写入单价 元" : "模型字段存在冲突或无法唯一匹配", text_evidence: record.text_evidence, raw_source: record.raw_source });
}

const headerText = await fs.readFile(apiHeadersPath, "utf8");
const sourceMetadata = JSON.parse(await fs.readFile(`${outputDir}/source-metadata.json`, "utf8"));
sourceMetadata.source_url = "https://zzone.cc.cd/api/pricing";
sourceMetadata.channel_id = 14;
sourceMetadata.channel_name = "zzone";
sourceMetadata.model_count = data.length;
sourceMetadata.video_candidate_count = videoEndpointRecords.length;
sourceMetadata.ignored_non_sd_count = ignoredNonSd.length;
sourceMetadata.pricing_version = api.pricing_version ?? null;
sourceMetadata.success = api.success === true;
sourceMetadata.headers_sha256 = await (async () => {
  const bytes = await fs.readFile(apiHeadersPath);
  const hash = await import("node:crypto").then(({ createHash }) => createHash("sha256").update(bytes).digest("hex"));
  return hash;
})();

await fs.writeFile(`${outputDir}/source-metadata.json`, JSON.stringify(sourceMetadata, null, 2));
await fs.writeFile(`${outputDir}/normalized-models.json`, JSON.stringify({ source_url: sourceMetadata.source_url, pricing_version: sourceMetadata.pricing_version, model_count: data.length, video_candidate_count: videoEndpointRecords.length, ignored_non_sd_count: ignoredNonSd.length, records: normalized }, null, 2));
await fs.writeFile(`${outputDir}/sd-update-patch.json`, JSON.stringify({ source_url: sourceMetadata.source_url, channel_id: 14, sd_target_rows: targetSnapshot, h3_target_rows: h3TargetSnapshot, patch }, null, 2));
await fs.writeFile(`${outputDir}/preflight.json`, JSON.stringify({ source_url: sourceMetadata.source_url, channel_id: 14, source_sha256: sourceMetadata.source_sha256, api_sha256: sourceMetadata.api_sha256, pricing_version: sourceMetadata.pricing_version, model_count: data.length, video_candidate_count: videoEndpointRecords.length, ignored_non_sd_count: ignoredNonSd.length, sd_target_rows: targetSnapshot, h3_target_rows: h3TargetSnapshot, model_ids: modelIds, patch_count: patch.length, patch }, null, 2));

const report = [
  "# ZZ One 模型同步预检报告",
  "",
  "- 结论：未写入 Google 表格（当前 API 没有可证明为人民币元的价格，新增候选均为 draft；仅生成审阅补丁）。",
  `- 源表：sd收录.xlsx，SHA-256：${sourceMetadata.source_sha256}`,
  `- API：${sourceMetadata.source_url}`,
  `- API 响应：HTTP ${sourceMetadata.api_status ?? 200}，success=${sourceMetadata.success}，SHA-256：${sourceMetadata.api_sha256}`,
  `- pricing_version：${sourceMetadata.pricing_version}`,
  `- 渠道匹配：zzone 唯一，数字渠道 ID 14，URL 逐字一致。`,
  `- API 模型数：${data.length}；视频候选：${videoEndpointRecords.length}；ignored_non_sd：${ignoredNonSd.length}`,
  `- 当前 sd 渠道 14 行数：${targetRows.length}；审阅补丁：${patch.length}`,
  `- h3 工作表渠道 14：${h3TargetSnapshot.length} 行；\`minimax-h3\` 当前行与 API 数值均为 call / 3，因 API 文案单位为“代币”而不更新“单价 元”。`,
  "",
  "## 视频模型 ID",
  "",
  ...videoEndpointRecords.map((model) => `- \`${model.model_name}\`：${normalized.find((item) => item.model_id === model.model_name)?.decision ?? "candidate"}`),
  "",
  "## 处理决定",
  "",
  "- `supported_endpoint_types` 只有明确包含 `videos` 的记录进入视频候选；`openai`、`images`、`chat` 等记录不参与 sd 新增、缺失或价格统计。",
  "- 标题/描述中的时长、清晰度、版本、计费方式和素材数量已按固定规则提取并保存 `text_evidence`；结构化字段与文字冲突时不写入。",
  "- 当前视频模型的 `video_unit_price`/`model_price` 没有 CNY/RMB/人民币元单位证据，描述中出现“代币”或仅有平台数值，因此不得写入“单价 元”。",
  "- API 中未出现的既有行只标记 `review_missing`，不删除、不改状态、不退休路由。",
  "",
  "## 完整 API 模型 ID",
  "",
  ...modelIds.map((modelId) => `- \`${modelId}\``),
  "",
  "## 审计文件",
  "",
  "- api-response.json、api-response-headers.txt、source-metadata.json",
  "- normalized-models.json、preflight.json、sd-update-patch.json",
  "- sd收录.xlsx（本轮下载副本）",
].join("\n");
await fs.writeFile(`${outputDir}/sd-update-report.md`, report);

console.log(JSON.stringify({
  model_count: data.length,
  video_candidate_count: videoEndpointRecords.length,
  ignored_non_sd_count: ignoredNonSd.length,
  target_row_count: targetRows.length,
  patch_count: patch.length,
  patch,
}, null, 2));

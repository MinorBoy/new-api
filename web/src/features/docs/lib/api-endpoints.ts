/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { ApiEndpoint } from '../types'

/**
 * Structured API endpoint catalog. Paths and methods mirror the real relay
 * routes in `common/endpoint_defaults.go` and `router/relay-router.go`.
 *
 * Parameter tables, error codes, and code samples are hand-authored to give
 * each endpoint a 4stoken-style reference page. To add an endpoint: append an
 * `ApiEndpoint` entry here; the helpers, sidebar, and routing pick it up
 * automatically.
 */
export const apiEndpoints: ApiEndpoint[] = [
  {
    slug: 'chat-completions',
    method: 'POST',
    path: '/v1/chat/completions',
    protocol: 'openai',
    category: 'chat',
    title: { en: 'Chat Completions', zh: '对话 Chat API' },
    summary: {
      en: 'OpenAI-compatible chat completion endpoint, the core conversational API.',
      zh: 'OpenAI 兼容对话补全接口，核心对话 API。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Model name to call.', zh: '要调用的模型名称。' } },
      { name: 'messages', type: 'object[]', required: 'yes', description: { en: 'Message list; supports text and multimodal content per model.', zh: '消息列表；不同模型支持文本、图片、音频等多种模态。' } },
      { name: 'messages[].role', type: 'string', required: 'yes', description: { en: 'system / user / assistant / tool.', zh: 'system / user / assistant / tool。' } },
      { name: 'messages[].content', type: 'string | object[]', required: 'yes', description: { en: 'Message content. Plain text or an array of content blocks for multimodal input.', zh: '消息内容。纯文本或多模态内容块数组。' } },
      { name: 'stream', type: 'boolean | null', required: 'no', description: { en: 'Enable SSE streaming. Ends with data: [DONE].', zh: '是否启用 SSE 流式返回，以 data: [DONE] 结束。' } },
      { name: 'stream_options.include_usage', type: 'boolean | null', required: 'no', description: { en: 'Return token usage stats in the streaming response.', zh: '流式响应中是否返回 token 用量统计。' } },
      { name: 'temperature', type: 'number | null', required: 'no', description: { en: 'Sampling temperature.', zh: '采样温度。' } },
      { name: 'max_tokens', type: 'integer | null', required: 'no', description: { en: 'Maximum output tokens.', zh: '最大输出 token 数。' } },
      { name: 'tools', type: 'object[] | null', required: 'no', description: { en: 'Function-calling tool definitions.', zh: '函数调用工具定义。' } },
      { name: 'response_format', type: 'object | null', required: 'no', description: { en: 'Output format control, e.g. { "type": "json_object" }.', zh: '输出格式控制，例如 { "type": "json_object" }。' } },
    ],
    responseParams: [
      { name: 'id', type: 'string', required: 'yes', description: { en: 'Response ID.', zh: '响应 ID。' } },
      { name: 'object', type: 'string', required: 'yes', description: { en: 'Usually chat.completion.', zh: '通常为 chat.completion。' } },
      { name: 'model', type: 'string', required: 'no', description: { en: 'Model name used for the response.', zh: '实际响应的模型名称。' } },
      { name: 'choices', type: 'object[]', required: 'yes', description: { en: 'Model output candidates.', zh: '模型输出候选。' } },
      { name: 'choices[].message.content', type: 'string | null', required: 'yes', description: { en: 'Assistant reply text (non-streaming).', zh: '助手回复文本（非流式）。' } },
      { name: 'choices[].finish_reason', type: 'string | null', required: 'no', description: { en: 'Stop reason: stop, length, tool_calls, etc.', zh: '停止原因：stop、length、tool_calls 等。' } },
      { name: 'usage.prompt_tokens', type: 'integer', required: 'no', description: { en: 'Input token count.', zh: '输入 token 数。' } },
      { name: 'usage.completion_tokens', type: 'integer', required: 'no', description: { en: 'Output token count.', zh: '输出 token 数。' } },
      { name: 'usage.total_tokens', type: 'integer', required: 'no', description: { en: 'Total token count.', zh: '总 token 数。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Bad request — malformed parameters or unsupported field.', zh: '参数格式错误或模型不支持该字段。' } },
      { status: 401, description: { en: 'Unauthorized — API key invalid or missing.', zh: 'API Key 无效或未提供。' } },
      { status: 429, description: { en: 'Rate limited or insufficient quota.', zh: '触发限流或额度不足。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      { "role": "user", "content": "Hello!" }
    ]
  }'`,
      },
      {
        lang: 'python',
        label: 'Python',
        highlight: 'python',
        code: `from openai import OpenAI

client = OpenAI(
    base_url="https://<your-domain>/v1",
    api_key="<YOUR_API_KEY>",
)
resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)`,
      },
      {
        lang: 'node',
        label: 'Node.js',
        highlight: 'javascript',
        code: `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://<your-domain>/v1",
  apiKey: "<YOUR_API_KEY>",
});
const resp = await client.chat.completions.create({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(resp.choices[0].message.content);`,
      },
    ],
  },
  {
    slug: 'messages',
    method: 'POST',
    path: '/v1/messages',
    protocol: 'anthropic',
    category: 'chat',
    title: { en: 'Messages', zh: 'Claude Messages API' },
    summary: {
      en: 'Anthropic-style Messages endpoint for the Claude model family.',
      zh: 'Anthropic 风格 Messages 接口，适用于 Claude 系列模型。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Claude model name.', zh: 'Claude 模型名称。' } },
      { name: 'messages', type: 'object[]', required: 'yes', description: { en: 'Message list with role and content.', zh: '消息列表，含 role 与 content。' } },
      { name: 'max_tokens', type: 'integer', required: 'yes', description: { en: 'Maximum output tokens.', zh: '最大输出 token 数。' } },
      { name: 'system', type: 'string | object[]', required: 'no', description: { en: 'System prompt.', zh: '系统提示词。' } },
      { name: 'stream', type: 'boolean', required: 'no', description: { en: 'Enable SSE streaming.', zh: '是否启用流式返回。' } },
      { name: 'temperature', type: 'number', required: 'no', description: { en: 'Sampling temperature (0–1).', zh: '采样温度（0–1）。' } },
      { name: 'tools', type: 'object[]', required: 'no', description: { en: 'Tool definitions for function calling.', zh: '函数调用工具定义。' } },
    ],
    responseParams: [
      { name: 'id', type: 'string', required: 'yes', description: { en: 'Message ID.', zh: '消息 ID。' } },
      { name: 'type', type: 'string', required: 'yes', description: { en: 'Usually message.', zh: '通常为 message。' } },
      { name: 'role', type: 'string', required: 'yes', description: { en: 'Always assistant.', zh: '始终为 assistant。' } },
      { name: 'content', type: 'object[]', required: 'yes', description: { en: 'Content blocks (text/tool_use).', zh: '内容块（text/tool_use）。' } },
      { name: 'stop_reason', type: 'string | null', required: 'no', description: { en: 'Why generation stopped.', zh: '停止生成的原因。' } },
      { name: 'usage.input_tokens', type: 'integer', required: 'no', description: { en: 'Input token count.', zh: '输入 token 数。' } },
      { name: 'usage.output_tokens', type: 'integer', required: 'no', description: { en: 'Output token count.', zh: '输出 token 数。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid request body.', zh: '请求体无效。' } },
      { status: 401, description: { en: 'Invalid API key.', zh: 'API Key 无效。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/messages \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [
      { "role": "user", "content": "Hello!" }
    ]
  }'`,
      },
      {
        lang: 'python',
        label: 'Python',
        highlight: 'python',
        code: `import anthropic

client = anthropic.Anthropic(
    base_url="https://<your-domain>",
    api_key="<YOUR_API_KEY>",
)
message = client.messages.create(
    model="claude-3-5-sonnet-20241022",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello!"}],
)
print(message.content[0].text)`,
      },
    ],
  },
  {
    slug: 'responses',
    method: 'POST',
    path: '/v1/responses',
    protocol: 'openai',
    category: 'chat',
    title: { en: 'Responses', zh: 'Responses API' },
    summary: {
      en: 'OpenAI next-generation Responses API for stateful conversations.',
      zh: 'OpenAI 新一代 Responses API，支持有状态对话。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Model name.', zh: '模型名称。' } },
      { name: 'input', type: 'string | object[]', required: 'yes', description: { en: 'Input text or content blocks.', zh: '输入文本或内容块。' } },
      { name: 'instructions', type: 'string', required: 'no', description: { en: 'System instructions.', zh: '系统指令。' } },
      { name: 'stream', type: 'boolean', required: 'no', description: { en: 'Enable streaming.', zh: '是否流式返回。' } },
      { name: 'previous_response_id', type: 'string', required: 'no', description: { en: 'Chain to a previous response.', zh: '关联上一次响应以实现多轮。' } },
    ],
    responseParams: [
      { name: 'id', type: 'string', required: 'yes', description: { en: 'Response ID.', zh: '响应 ID。' } },
      { name: 'output', type: 'object[]', required: 'yes', description: { en: 'Output content items.', zh: '输出内容项。' } },
      { name: 'usage', type: 'object', required: 'no', description: { en: 'Token usage stats.', zh: 'token 用量统计。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid request.', zh: '请求无效。' } },
      { status: 401, description: { en: 'Unauthorized.', zh: '未授权。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/responses \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "gpt-4o",
    "input": "Hello!"
  }'`,
      },
    ],
  },
  {
    slug: 'embeddings',
    method: 'POST',
    path: '/v1/embeddings',
    protocol: 'openai',
    category: 'embeddings',
    title: { en: 'Embeddings', zh: '文本向量' },
    summary: {
      en: 'Convert text into vector embeddings for search and clustering.',
      zh: '将文本转换为向量，用于检索与聚类。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Embedding model name.', zh: '向量模型名称。' } },
      { name: 'input', type: 'string | string[]', required: 'yes', description: { en: 'Text to embed.', zh: '要向量化的文本。' } },
      { name: 'encoding_format', type: 'string', required: 'no', description: { en: 'float (default) or base64.', zh: 'float（默认）或 base64。' } },
      { name: 'dimensions', type: 'integer', required: 'no', description: { en: 'Output dimensions (supported models).', zh: '输出维度（部分模型支持）。' } },
    ],
    responseParams: [
      { name: 'object', type: 'string', required: 'yes', description: { en: 'Usually list.', zh: '通常为 list。' } },
      { name: 'data', type: 'object[]', required: 'yes', description: { en: 'Embedding vectors.', zh: '向量数据。' } },
      { name: 'data[].embedding', type: 'number[]', required: 'yes', description: { en: 'The embedding vector.', zh: '向量数组。' } },
      { name: 'usage.prompt_tokens', type: 'integer', required: 'no', description: { en: 'Input token count.', zh: '输入 token 数。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid input.', zh: '输入无效。' } },
      { status: 401, description: { en: 'Unauthorized.', zh: '未授权。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/embeddings \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "text-embedding-3-small",
    "input": "Text to embed."
  }'`,
      },
      {
        lang: 'python',
        label: 'Python',
        highlight: 'python',
        code: `from openai import OpenAI

client = OpenAI(base_url="https://<your-domain>/v1", api_key="<YOUR_API_KEY>")
resp = client.embeddings.create(
    model="text-embedding-3-small",
    input="Text to embed.",
)
print(resp.data[0].embedding[:8])`,
      },
    ],
  },
  {
    slug: 'images-generations',
    method: 'POST',
    path: '/v1/images/generations',
    protocol: 'openai',
    category: 'images',
    title: { en: 'Image Generation', zh: '文生图' },
    summary: {
      en: 'Generate images from a text prompt.',
      zh: '根据文本提示生成图像。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Image model name.', zh: '图像模型名称。' } },
      { name: 'prompt', type: 'string', required: 'yes', description: { en: 'Text description of the image.', zh: '图像的文本描述。' } },
      { name: 'n', type: 'integer', required: 'no', description: { en: 'Number of images (1–10).', zh: '生成数量（1–10）。' } },
      { name: 'size', type: 'string', required: 'no', description: { en: 'e.g. 1024x1024.', zh: '如 1024x1024。' } },
      { name: 'response_format', type: 'string', required: 'no', description: { en: 'url or b64_json.', zh: 'url 或 b64_json。' } },
    ],
    responseParams: [
      { name: 'created', type: 'integer', required: 'no', description: { en: 'Unix timestamp.', zh: 'Unix 时间戳。' } },
      { name: 'data', type: 'object[]', required: 'yes', description: { en: 'Generated images.', zh: '生成的图像。' } },
      { name: 'data[].url', type: 'string', required: 'conditional', description: { en: 'Image URL (when response_format=url).', zh: '图像 URL（response_format=url 时）。' } },
      { name: 'data[].b64_json', type: 'string', required: 'conditional', description: { en: 'Base64 image (when response_format=b64_json).', zh: 'Base64 图像（response_format=b64_json 时）。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid parameters.', zh: '参数无效。' } },
      { status: 401, description: { en: 'Unauthorized.', zh: '未授权。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/images/generations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "dall-e-3",
    "prompt": "a cute cat",
    "n": 1,
    "size": "1024x1024"
  }'`,
      },
    ],
  },
  {
    slug: 'images-edits',
    method: 'POST',
    path: '/v1/images/edits',
    protocol: 'openai',
    category: 'images',
    title: { en: 'Image Edit', zh: '图片编辑' },
    summary: {
      en: 'Edit or extend an existing image.',
      zh: '编辑或扩展已有图像。',
    },
    auth: 'Bearer Token',
    contentType: 'multipart/form-data',
    requestParams: [
      { name: 'image', type: 'file', required: 'yes', description: { en: 'Original image to edit.', zh: '要编辑的原始图像。' } },
      { name: 'prompt', type: 'string', required: 'yes', description: { en: 'Edit instruction.', zh: '编辑指令。' } },
      { name: 'mask', type: 'file', required: 'no', description: { en: 'Mask marking editable regions.', zh: '标记可编辑区域的蒙版。' } },
      { name: 'model', type: 'string', required: 'no', description: { en: 'Image model name.', zh: '图像模型名称。' } },
      { name: 'n', type: 'integer', required: 'no', description: { en: 'Number of results.', zh: '结果数量。' } },
    ],
    responseParams: [
      { name: 'data', type: 'object[]', required: 'yes', description: { en: 'Edited images.', zh: '编辑后的图像。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid image or prompt.', zh: '图像或指令无效。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/images/edits \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -F image="@original.png" \\
  -F prompt="add a hat"`,
      },
    ],
  },
  {
    slug: 'audio-transcriptions',
    method: 'POST',
    path: '/v1/audio/transcriptions',
    protocol: 'openai',
    category: 'audio',
    title: { en: 'Speech to Text', zh: '语音转写' },
    summary: {
      en: 'Transcribe audio into text.',
      zh: '将音频转录为文本。',
    },
    auth: 'Bearer Token',
    contentType: 'multipart/form-data',
    requestParams: [
      { name: 'file', type: 'file', required: 'yes', description: { en: 'Audio file to transcribe.', zh: '要转写的音频文件。' } },
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Transcription model.', zh: '转写模型。' } },
      { name: 'language', type: 'string', required: 'no', description: { en: 'ISO-639-1 language code.', zh: 'ISO-639-1 语言代码。' } },
      { name: 'response_format', type: 'string', required: 'no', description: { en: 'json, text, srt, vtt.', zh: 'json、text、srt、vtt。' } },
    ],
    responseParams: [
      { name: 'text', type: 'string', required: 'yes', description: { en: 'Transcribed text.', zh: '转写文本。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid audio file.', zh: '音频文件无效。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/audio/transcriptions \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -F file="@audio.mp3" \\
  -F model="whisper-1"`,
      },
    ],
  },
  {
    slug: 'audio-speech',
    method: 'POST',
    path: '/v1/audio/speech',
    protocol: 'openai',
    category: 'audio',
    title: { en: 'Text to Speech', zh: '语音合成' },
    summary: {
      en: 'Convert text into spoken audio.',
      zh: '将文本合成为语音。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'TTS model name.', zh: 'TTS 模型名称。' } },
      { name: 'input', type: 'string', required: 'yes', description: { en: 'Text to synthesize.', zh: '要合成的文本。' } },
      { name: 'voice', type: 'string', required: 'yes', description: { en: 'Voice name.', zh: '音色名称。' } },
      { name: 'response_format', type: 'string', required: 'no', description: { en: 'mp3, opus, aac, flac.', zh: 'mp3、opus、aac、flac。' } },
    ],
    responseParams: [
      { name: '(binary)', type: 'audio stream', required: 'yes', description: { en: 'Audio binary data.', zh: '音频二进制数据。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid input.', zh: '输入无效。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/audio/speech \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "tts-1",
    "input": "Hello world",
    "voice": "alloy"
  }' --output speech.mp3`,
      },
    ],
  },
  {
    slug: 'videos',
    method: 'POST',
    path: '/v1/videos',
    protocol: 'gateway',
    category: 'video',
    title: { en: 'Video Generation', zh: '视频生成' },
    summary: {
      en: 'Gateway video generation API (async task).',
      zh: 'Gateway 视频生成接口（异步任务）。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Video model name.', zh: '视频模型名称。' } },
      { name: 'prompt', type: 'string', required: 'yes', description: { en: 'Generation prompt.', zh: '生成提示词。' } },
      { name: 'image', type: 'string', required: 'no', description: { en: 'Reference image URL (image-to-video).', zh: '参考图 URL（图生视频）。' } },
      { name: 'duration', type: 'integer', required: 'no', description: { en: 'Video duration in seconds.', zh: '视频时长（秒）。' } },
    ],
    responseParams: [
      { name: 'id', type: 'string', required: 'yes', description: { en: 'Task ID for polling.', zh: '任务 ID，用于轮询。' } },
      { name: 'status', type: 'string', required: 'yes', description: { en: 'Task status.', zh: '任务状态。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid parameters.', zh: '参数无效。' } },
      { status: 429, description: { en: 'Rate limited.', zh: '触发限流。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/videos \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "seedance-v1",
    "prompt": "a cat playing"
  }'`,
      },
    ],
  },
  {
    slug: 'rerank',
    method: 'POST',
    path: '/v1/rerank',
    protocol: 'openai',
    category: 'rerank',
    title: { en: 'Rerank', zh: '重排序' },
    summary: {
      en: 'Rerank documents by relevance to a query.',
      zh: '根据查询对文档重排序。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Rerank model name.', zh: '重排模型名称。' } },
      { name: 'query', type: 'string', required: 'yes', description: { en: 'Search query.', zh: '搜索查询。' } },
      { name: 'documents', type: 'string[]', required: 'yes', description: { en: 'Documents to rerank.', zh: '待重排的文档。' } },
      { name: 'top_n', type: 'integer', required: 'no', description: { en: 'Number of top results.', zh: '返回前 N 条。' } },
    ],
    responseParams: [
      { name: 'results', type: 'object[]', required: 'yes', description: { en: 'Reranked results with scores.', zh: '重排序结果及分数。' } },
      { name: 'results[].relevance_score', type: 'number', required: 'yes', description: { en: 'Relevance score.', zh: '相关性分数。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid input.', zh: '输入无效。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/rerank \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "rerank-v1",
    "query": "what is ai",
    "documents": ["AI is ...", "Machine learning ..."]
  }'`,
      },
    ],
  },
  {
    slug: 'moderations',
    method: 'POST',
    path: '/v1/moderations',
    protocol: 'openai',
    category: 'moderation',
    title: { en: 'Moderations', zh: '内容审核' },
    summary: {
      en: 'Check whether text violates content policies.',
      zh: '检查文本是否违反内容政策。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'input', type: 'string | string[]', required: 'yes', description: { en: 'Text to moderate.', zh: '要审核的文本。' } },
      { name: 'model', type: 'string', required: 'no', description: { en: 'Moderation model.', zh: '审核模型。' } },
    ],
    responseParams: [
      { name: 'results', type: 'object[]', required: 'yes', description: { en: 'Moderation results.', zh: '审核结果。' } },
      { name: 'results[].flagged', type: 'boolean', required: 'yes', description: { en: 'Whether flagged.', zh: '是否被标记。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'Invalid input.', zh: '输入无效。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/moderations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{ "input": "some text to check" }'`,
      },
    ],
  },
  {
    slug: 'models',
    method: 'GET',
    path: '/v1/models',
    protocol: 'openai',
    category: 'models',
    title: { en: 'List Models', zh: '模型列表' },
    summary: {
      en: 'List models available to your API key.',
      zh: '列出当前 API Key 可用的模型。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [],
    responseParams: [
      { name: 'object', type: 'string', required: 'yes', description: { en: 'Usually list.', zh: '通常为 list。' } },
      { name: 'data', type: 'object[]', required: 'yes', description: { en: 'Model list.', zh: '模型列表。' } },
      { name: 'data[].id', type: 'string', required: 'yes', description: { en: 'Model name.', zh: '模型名称。' } },
      { name: 'data[].owned_by', type: 'string', required: 'no', description: { en: 'Owner.', zh: '所有者。' } },
    ],
    errorCodes: [
      { status: 401, description: { en: 'Unauthorized.', zh: '未授权。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/v1/models \\
  -H "Authorization: Bearer <YOUR_API_KEY>"`,
      },
    ],
  },
  {
    slug: 'ark-video-tasks-create',
    method: 'POST',
    path: '/api/v3/contents/generations/tasks',
    protocol: 'ark',
    category: 'video',
    title: { en: 'Ark Video Task Create', zh: 'Seedance 视频任务创建' },
    summary: {
      en: 'Volcengine Ark native video generation task submission (Seedance).',
      zh: '火山引擎方舟原生视频生成任务提交（Seedance）。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'model', type: 'string', required: 'yes', description: { en: 'Seedance model name.', zh: 'Seedance 模型名称。' } },
      { name: 'content', type: 'object[]', required: 'yes', description: { en: 'Content blocks: text prompts, image/video/audio references.', zh: '内容块：文本提示词、图片/视频/音频引用。' } },
      { name: 'content[].type', type: 'string', required: 'yes', description: { en: 'Block type: text, image_url, video_url, audio_url.', zh: '内容块类型：text、image_url、video_url、audio_url。' } },
      { name: 'content[].text', type: 'string', required: 'conditional', description: { en: 'Text prompt (when type=text).', zh: '文本提示词（type=text 时）。' } },
      { name: 'content[].image_url', type: 'object', required: 'conditional', description: { en: 'Reference image URL object (when type=image_url).', zh: '参考图 URL 对象（type=image_url 时）。' } },
      { name: 'resolution', type: 'string', required: 'no', description: { en: 'Output resolution, e.g. 720p, 1080p.', zh: '输出分辨率，如 720p、1080p。' } },
      { name: 'ratio', type: 'string', required: 'no', description: { en: 'Aspect ratio, e.g. 16:9, 9:16.', zh: '画面比例，如 16:9、9:16。' } },
      { name: 'duration', type: 'integer', required: 'no', description: { en: 'Video duration in seconds.', zh: '视频时长（秒）。' } },
      { name: 'frames', type: 'integer', required: 'no', description: { en: 'Total frame count.', zh: '总帧数。' } },
      { name: 'seed', type: 'integer', required: 'no', description: { en: 'Random seed for reproducibility.', zh: '随机种子，用于结果复现。' } },
      { name: 'service_tier', type: 'string', required: 'no', description: { en: 'Service tier.', zh: '服务等级。' } },
      { name: 'generate_audio', type: 'boolean', required: 'no', description: { en: 'Whether to generate audio.', zh: '是否生成音频。' } },
      { name: 'watermark', type: 'boolean', required: 'no', description: { en: 'Whether to add watermark.', zh: '是否添加水印。' } },
    ],
    responseParams: [
      { name: 'id', type: 'string', required: 'yes', description: { en: 'Task ID for polling.', zh: '任务 ID，用于轮询查询。' } },
      { name: 'model', type: 'string', required: 'no', description: { en: 'Model name.', zh: '模型名称。' } },
      { name: 'status', type: 'string', required: 'no', description: { en: 'Task status: queued, running, succeeded, failed.', zh: '任务状态：queued、running、succeeded、failed。' } },
    ],
    errorCodes: [
      { status: 400, description: { en: 'InvalidParameter — malformed request body or missing model/content.', zh: '参数无效——请求体格式错误或缺少 model/content。' } },
      { status: 401, description: { en: 'Unauthorized — API key invalid or missing.', zh: '未授权——API Key 无效或未提供。' } },
      { status: 429, description: { en: 'Rate limited or insufficient quota.', zh: '触发限流或额度不足。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl https://<your-domain>/api/v3/contents/generations/tasks \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer <YOUR_API_KEY>" \\
  -d '{
    "model": "seedance-1-0-pro-250528",
    "content": [
      { "type": "text", "text": "一只猫在草地上奔跑" }
    ],
    "resolution": "1080p",
    "ratio": "16:9",
    "duration": 5
  }'`,
      },
      {
        lang: 'python',
        label: 'Python',
        highlight: 'python',
        code: `import requests

resp = requests.post(
    "https://<your-domain>/api/v3/contents/generations/tasks",
    headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer <YOUR_API_KEY>",
    },
    json={
        "model": "seedance-1-0-pro-250528",
        "content": [{"type": "text", "text": "一只猫在草地上奔跑"}],
        "resolution": "1080p",
        "ratio": "16:9",
        "duration": 5,
    },
)
print(resp.json()["id"])`,
      },
    ],
  },
  {
    slug: 'ark-video-tasks-fetch',
    method: 'GET',
    path: '/api/v3/contents/generations/tasks/{task_id}',
    protocol: 'ark',
    category: 'video',
    title: { en: 'Ark Video Task Query', zh: 'Seedance 视频任务查询' },
    summary: {
      en: 'Query the status and result of a Volcengine Ark video generation task.',
      zh: '查询火山引擎方舟视频生成任务的状态与结果。',
    },
    auth: 'Bearer Token',
    contentType: 'application/json',
    requestParams: [
      { name: 'task_id (path)', type: 'string', required: 'yes', description: { en: 'Task ID returned by the create endpoint.', zh: '创建接口返回的任务 ID。' } },
    ],
    responseParams: [
      { name: 'id', type: 'string', required: 'yes', description: { en: 'Task ID.', zh: '任务 ID。' } },
      { name: 'status', type: 'string', required: 'yes', description: { en: 'succeeded, running, failed.', zh: 'succeeded、running、failed。' } },
      { name: 'model', type: 'string', required: 'no', description: { en: 'Model name.', zh: '模型名称。' } },
      { name: 'content', type: 'object[]', required: 'conditional', description: { en: 'Output content (video URL) when succeeded.', zh: '成功时输出的内容（视频 URL）。' } },
      { name: 'usage', type: 'object', required: 'no', description: { en: 'Token / duration usage.', zh: 'token / 时长用量。' } },
      { name: 'error', type: 'object', required: 'conditional', description: { en: 'Error detail when failed.', zh: '失败时的错误详情。' } },
    ],
    errorCodes: [
      { status: 401, description: { en: 'Unauthorized.', zh: '未授权。' } },
      { status: 404, description: { en: 'task_not_exist — unknown task ID.', zh: 'task_not_exist——任务 ID 不存在。' } },
    ],
    codeSamples: [
      {
        lang: 'curl',
        label: 'cURL',
        highlight: 'bash',
        code: `curl "https://<your-domain>/api/v3/contents/generations/tasks/<TASK_ID>" \\
  -H "Authorization: Bearer <YOUR_API_KEY>"`,
      },
      {
        lang: 'python',
        label: 'Python',
        highlight: 'python',
        code: `import requests

resp = requests.get(
    "https://<your-domain>/api/v3/contents/generations/tasks/<TASK_ID>",
    headers={"Authorization": "Bearer <YOUR_API_KEY>"},
)
print(resp.json()["status"])`,
      },
    ],
  },
]

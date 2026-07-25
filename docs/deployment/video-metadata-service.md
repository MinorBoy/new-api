# 视频元数据服务部署

## 作用与边界

视频元数据服务负责下载并解析用户提供的 MP4/MOV 输入视频，向 new-api 返回时长、宽高、帧率和缓存校验信息。视频正文只进入该独立服务的临时文件，不进入 new-api 进程，也不会完整载入内存。

生产环境应将该服务部署到独立服务器或独立计算节点。与 new-api 同机运行只能隔离进程资源，不能隔离出口带宽和故障域。

## 配置

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `VIDEO_METADATA_LISTEN_ADDR` | `:8090` | 内部 HTTP 监听地址 |
| `VIDEO_METADATA_SERVICE_TOKEN` | 无 | 必填服务令牌；生产环境至少使用 32 个随机字节 |
| `VIDEO_METADATA_MAX_BYTES` | `134217728` | 单视频最大下载字节数，不得超过 128 MiB |
| `VIDEO_METADATA_TIMEOUT_SECONDS` | `30` | HEAD、GET 和解析的总预算，范围 1-30 秒 |
| `VIDEO_METADATA_MAX_CONCURRENCY` | `16` | 同时处理的元数据请求数 |
| `VIDEO_METADATA_CACHE_ENTRIES` | `10000` | 进程内 LRU 缓存条目数；0 表示禁用 |
| `VIDEO_METADATA_CACHE_TTL_SECONDS` | `600` | 普通 URL 缓存时间 |
| `VIDEO_METADATA_SIGNED_URL_CACHE_TTL_SECONDS` | `60` | 带查询参数 URL 的缓存时间，不得大于普通 TTL |

生成生产令牌示例：

```bash
openssl rand -hex 32
```

令牌只用于 new-api 与元数据服务之间的内部鉴权，不能复用用户 API Key，也不能写入日志、错误响应或业务报表。

## 网络安全

服务默认只允许访问 HTTP/HTTPS 的 80、443 端口，并在 URL 校验、DNS 解析、实际拨号和每次重定向时阻止私网、回环、链路本地及保留地址。DNS 结果只要包含一个被禁止的地址，就会拒绝整个请求。

生产防火墙应只允许 new-api 节点访问 8090 端口，不应把该端口暴露到公网。基础设施支持时，在反向代理或服务网格层增加 mTLS；Bearer 令牌仍保留为应用层鉴权。

## Docker Compose

本地完整环境已在 `docker-compose.local.yml` 中包含 `video-metadata` 服务：

```bash
VIDEO_METADATA_SERVICE_TOKEN=replace-with-a-random-secret \
docker compose -f docker-compose.local.yml up -d --build video-metadata new-api
```

健康检查：

```bash
docker compose -f docker-compose.local.yml ps video-metadata
docker compose -f docker-compose.local.yml logs --no-color video-metadata
```

服务内部健康接口为 `GET /healthz`，不需要 Bearer 令牌。元数据接口示例：

```bash
curl -X POST http://video-metadata:8090/v1/metadata/video \
  -H "Authorization: Bearer ${VIDEO_METADATA_SERVICE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://assets.example/input.mp4","media_type":"video","max_bytes":134217728,"deadline_ms":30000}'
```

## 容量与可观测性

每个并发请求最多占用 128 MiB 临时文件空间和一个素材源连接。容量规划至少按 `最大并发数 × 单视频上限` 预留临时磁盘与出口带宽，并在基础设施层限制进程 CPU、内存、临时磁盘和连接数。

结构化日志只包含 `request_id`、`result_code`、`elapsed_ms`、`bytes` 和 `cache_hit`。监控系统应从日志或采集代理形成以下指标：

- 请求数和各 `result_code` 错误数；
- 下载字节数与解析延迟；
- 当前活跃请求数和并发拒绝数；
- 缓存命中率；
- 容器临时磁盘、出口流量、CPU 和内存。

日志采集规则不得加入 Authorization、完整素材 URL、URL 查询参数、响应正文或视频内容。请求完成后 `/tmp` 不应残留 `video-metadata-*` 临时文件。

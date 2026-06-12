# baixin-switch 私有化部署手册

## 1. 部署目标

`baixin-switch` 默认作为 OpenAI-compatible 到 Anthropic Messages 的私有化协议网关：

```text
Comate / OpenAI-compatible client
  -> baixin-switch /v1/chat/completions
  -> 内部 DS Pro Anthropic-compatible gateway /v1/messages
```

当前版本支持：

- OpenAI Chat Completions -> Anthropic Messages
- 非流式和流式响应
- OpenAI tools/tool_choice/tool messages 与 Anthropic tool_use/tool_result 转换
- `/healthz`、`/readyz`、`/metrics`
- JSON access log
- 会话级 JSONL 审计日志，默认脱敏/截断请求与响应预览
- token 使用统计脚本，支持按天、周、会话聚合
- 单实例进程内并发限流，默认 1000 active requests / 1000 active streams
- `MODE=openai_passthrough` 兼容旧 DS Pro OpenAI-compatible 流式工具调用修正

当前版本不支持：

- `/v1/responses`
- 多上游自动故障转移
- 内置客户端鉴权
- 数据库审计后台和可视化管理台

## 2. 环境要求

二进制部署：

- Go 1.22 或以上
- 可以访问内部 Anthropic Messages-compatible 上游
- 建议运行机器 `ulimit -n` 不低于 `8192`

Docker 部署：

- Docker 20+ 或兼容运行时
- 容器可以访问内部上游

## 3. 配置项

| 变量 | 默认值 | 必填 | 说明 |
| --- | --- | --- | --- |
| `LISTEN_ADDR` | `127.0.0.1:11435` | 否 | 服务监听地址。内网部署通常设为 `0.0.0.0:11435`。 |
| `MODE` | `anthropic_messages` | 否 | 默认协议转换模式。旧 OpenAI 透传模式填 `openai_passthrough`。 |
| `UPSTREAM_BASE_URL` | `https://api.deepseek.com` | 是 | 上游 base URL，不要带 `/v1/messages`。 |
| `UPSTREAM_API_KEY` | 空 | 视上游而定 | 发给上游的 token。 |
| `ANTHROPIC_VERSION` | `2023-06-01` | 否 | Anthropic-compatible 上游版本头。 |
| `DEFAULT_MODEL` | `deepseek-v4-pro` | 否 | OpenAI 请求未传 `model` 时使用。 |
| `MODEL_MAP` | 空 | 否 | 模型映射，例如 `gpt-4.1=deepseek-v4-pro,claude-sonnet=deepseek-v4-pro`。 |
| `MAX_CONCURRENT_REQUESTS` | `1000` | 否 | 单进程 active request 上限，超出返回 429。 |
| `MAX_CONCURRENT_STREAMS` | `1000` | 否 | 单进程 active stream 上限，超出返回 429。 |
| `REQUEST_TIMEOUT_SECONDS` | `600` | 否 | 上游请求超时。 |
| `AUDIT_ENABLED` | `true` | 否 | 是否写入会话审计 JSONL。 |
| `AUDIT_LOG_DIR` | `./logs` | 否 | 审计和用量 JSONL 存储目录，默认文件名为 `usage.jsonl`。 |
| `AUDIT_LOG_PATH` | 根据 `AUDIT_LOG_DIR` 生成 | 否 | 高级配置：精确指定 JSONL 文件路径，优先级高于 `AUDIT_LOG_DIR`。 |
| `AUDIT_CAPTURE_BODY` | `preview` | 否 | `off`、`preview`、`full`。默认脱敏截断；`full` 也会脱敏但会保存完整正文。 |
| `AUDIT_PREVIEW_CHARS` | `2000` | 否 | 请求/响应预览最大字符数。 |
| `AUDIT_QUEUE_SIZE` | `8192` | 否 | 异步审计队列大小。 |
| `AUDIT_OVERFLOW_POLICY` | `drop` | 否 | 队列满时 `drop` 丢弃审计事件，`sync` 同步写入。 |
| `TOOL_CALL_STREAM_SHIM` | `true` | 否 | 仅 `openai_passthrough` 使用。 |
| `TOOL_CALL_ARGUMENT_CHUNK_SIZE` | `16` | 否 | 仅 `openai_passthrough` 使用。 |

## 4. 源码构建

```bash
cd /path/to/baixin-switch
GOCACHE="$(pwd)/.cache/go-build" go test ./...
GOCACHE="$(pwd)/.cache/go-build" go build -o baixin-switch ./cmd/baixin-switch
```

启动：

```bash
export LISTEN_ADDR="0.0.0.0:11435"
export MODE="anthropic_messages"
export UPSTREAM_BASE_URL="https://your-ds-pro-anthropic-gateway.example.com"
export UPSTREAM_API_KEY="sk-..."
export DEFAULT_MODEL="deepseek-v4-pro"
export MAX_CONCURRENT_REQUESTS="1000"
export MAX_CONCURRENT_STREAMS="1000"
export AUDIT_ENABLED="true"
export AUDIT_LOG_DIR="./logs"
export AUDIT_CAPTURE_BODY="preview"
./baixin-switch
```

探针：

```bash
curl -sS http://127.0.0.1:11435/healthz
curl -sS http://127.0.0.1:11435/readyz
curl -sS http://127.0.0.1:11435/metrics
```

## 5. Docker 部署

构建镜像：

```bash
cd /path/to/baixin-switch
docker build -t baixin-switch:latest .
```

启动容器：

```bash
docker run -d \
  --name baixin-switch \
  --restart unless-stopped \
  -p 11435:11435 \
  -e LISTEN_ADDR=0.0.0.0:11435 \
  -e MODE=anthropic_messages \
  -e UPSTREAM_BASE_URL=https://your-ds-pro-anthropic-gateway.example.com \
  -e UPSTREAM_API_KEY=sk-... \
  -e DEFAULT_MODEL=deepseek-v4-pro \
  -e MAX_CONCURRENT_REQUESTS=1000 \
  -e MAX_CONCURRENT_STREAMS=1000 \
  -e AUDIT_ENABLED=true \
  -e AUDIT_LOG_DIR=/var/log/baixin-switch \
  -e AUDIT_CAPTURE_BODY=preview \
  -v "$(pwd)/logs:/var/log/baixin-switch" \
  baixin-switch:latest
```

检查状态：

```bash
docker logs -f baixin-switch
curl -sS http://127.0.0.1:11435/readyz
```

## 6. docker-compose 示例

```yaml
services:
  baixin-switch:
    image: baixin-switch:latest
    container_name: baixin-switch
    restart: unless-stopped
    ports:
      - "11435:11435"
    environment:
      LISTEN_ADDR: "0.0.0.0:11435"
      MODE: "anthropic_messages"
      UPSTREAM_BASE_URL: "https://your-ds-pro-anthropic-gateway.example.com"
      UPSTREAM_API_KEY: "sk-..."
      DEFAULT_MODEL: "deepseek-v4-pro"
      MAX_CONCURRENT_REQUESTS: "1000"
      MAX_CONCURRENT_STREAMS: "1000"
      REQUEST_TIMEOUT_SECONDS: "600"
      AUDIT_ENABLED: "true"
      AUDIT_LOG_DIR: "/var/log/baixin-switch"
      AUDIT_CAPTURE_BODY: "preview"
    volumes:
      - ./logs:/var/log/baixin-switch
```

启动：

```bash
docker compose up -d
docker compose logs -f
```

## 7. systemd 部署示例

将二进制放到：

```text
/opt/baixin-switch/baixin-switch
```

创建环境文件 `/etc/baixin-switch.env`：

```bash
LISTEN_ADDR=0.0.0.0:11435
MODE=anthropic_messages
UPSTREAM_BASE_URL=https://your-ds-pro-anthropic-gateway.example.com
UPSTREAM_API_KEY=sk-...
DEFAULT_MODEL=deepseek-v4-pro
MAX_CONCURRENT_REQUESTS=1000
MAX_CONCURRENT_STREAMS=1000
REQUEST_TIMEOUT_SECONDS=600
AUDIT_ENABLED=true
AUDIT_LOG_DIR=/var/log/baixin-switch
AUDIT_CAPTURE_BODY=preview
AUDIT_PREVIEW_CHARS=2000
```

创建 `/etc/systemd/system/baixin-switch.service`：

```ini
[Unit]
Description=baixin-switch OpenAI to Anthropic gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/baixin-switch
EnvironmentFile=/etc/baixin-switch.env
ExecStart=/opt/baixin-switch/baixin-switch
Restart=always
RestartSec=3
User=baixin
Group=baixin
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now baixin-switch
sudo systemctl status baixin-switch
```

查看日志：

```bash
journalctl -u baixin-switch -f
```

## 8. 上游地址规则

`UPSTREAM_BASE_URL` 必须是 base URL。

正确：

```text
https://your-ds-pro-anthropic-gateway.example.com
https://your-ds-pro-anthropic-gateway.example.com/anthropic
```

错误：

```text
https://your-ds-pro-anthropic-gateway.example.com/v1/messages
```

`baixin-switch` 默认会自动请求：

```text
${UPSTREAM_BASE_URL}/v1/messages
```

`MODE=openai_passthrough` 时会请求：

```text
${UPSTREAM_BASE_URL}/v1/chat/completions
```

## 9. 上线验证

非流式请求：

```bash
curl -sS http://127.0.0.1:11435/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dummy-client-key' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "stream": false
  }'
```

流式请求：

```bash
curl -N http://127.0.0.1:11435/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dummy-client-key' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "stream": true
  }'
```

观测检查：

```bash
curl -sS http://127.0.0.1:11435/metrics | grep baixin
```

关键指标：

- `baixin_http_requests_total`
- `baixin_conversion_errors_total`
- `baixin_audit_events_total`
- `baixin_audit_queue_depth`
- `baixin_tokens_total`
- `baixin_active_requests`
- `baixin_active_streams`

## 10. 会话观测和 token 统计

默认审计文件：

```text
./logs/usage.jsonl
```

每个模型请求写入一行 JSON，包含：

- `request_id`
- `session_id`
- `model`
- `status`
- `input_tokens`、`output_tokens`、`total_tokens`
- 脱敏截断后的 `request_preview` 和 `response_preview`

客户端建议传入：

```text
X-Session-ID: <业务会话 ID>
```

统计命令：

```bash
./scripts/usage_report.sh --file ./logs/usage.jsonl --by day
./scripts/usage_report.sh --file ./logs/usage.jsonl --by week
./scripts/usage_report.sh --file ./logs/usage.jsonl --by session
./scripts/usage_report.sh --file ./logs/usage.jsonl --session sess_abc
```

输出 JSON：

```bash
./scripts/usage_report.sh --file ./logs/usage.jsonl --by day --json
```

隐私说明：

- `AUDIT_CAPTURE_BODY=preview` 是生产推荐值。
- `AUDIT_CAPTURE_BODY=full` 会保存完整请求/响应正文，虽然仍会脱敏 token、password、authorization 等字段，但仍可能包含用户 prompt 和模型输出，开启前需要明确运维和合规风险。
- 如果只需要 token 统计，不需要正文，设置 `AUDIT_CAPTURE_BODY=off`。

## 11. 1000 并发建议

单实例 1000 并发可行性取决于上游延迟、流式请求占比、机器文件描述符和网络带宽。当前实现已经做了：

- active request limiter 默认 1000
- active stream limiter 默认 1000
- 上游 HTTP connection pool 按并发上限配置
- Anthropic SSE 主路径边读边写，不缓存完整流
- 审计日志异步有界队列写入，默认队列满时丢弃审计事件以保护请求延迟

上线前建议：

- `ulimit -n >= 8192`
- 容器或 systemd 配置 `LimitNOFILE=65535`
- 先用 100、300、600、1000 梯度压测
- 观察 429、上游 5xx、P95/P99 延迟、CPU、内存、连接数

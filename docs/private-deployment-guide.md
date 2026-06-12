# baixin-switch 私有化部署手册

## 1. 部署目标

`baixin-switch` 是一个内部 OpenAI Chat Completions 兼容代理，主要解决 DS Pro 在流式工具调用中一次性返回完整 `function.arguments`，导致 Comate 等内部编程助手无法消费的问题。

部署后，调用链为：

```text
Comate / OpenAI-compatible client
  -> baixin-switch
  -> 内部 DS Pro / vLLM / OpenAI-compatible gateway
```

当前版本支持：

- `GET /health`
- `POST /v1/chat/completions`
- 流式 `text/event-stream` 响应
- DS Pro one-shot tool call arguments -> OpenAI incremental tool call delta
- 非流式响应透传

当前版本不支持：

- `/v1/responses`
- Anthropic Messages API 转换
- 多上游故障转移
- 内置鉴权

## 2. 环境要求

### 二进制部署

- Go 1.22 或以上
- 可以访问内部 DS Pro 网关
- 运行机器开放本地或内网端口，例如 `11435`

### Docker 部署

- Docker 20+ 或兼容运行时
- 可以从运行容器访问内部 DS Pro 网关

## 3. 配置项

| 变量 | 默认值 | 必填 | 说明 |
| --- | --- | --- | --- |
| `LISTEN_ADDR` | `127.0.0.1:11435` | 否 | 服务监听地址。私有化内网部署通常设为 `0.0.0.0:11435`。 |
| `UPSTREAM_BASE_URL` | `https://api.deepseek.com` | 是 | 上游 OpenAI-compatible 网关 base URL，不要带 `/v1/chat/completions`。 |
| `UPSTREAM_API_KEY` | 空 | 视上游而定 | 发给上游的 Bearer Token。 |
| `TOOL_CALL_STREAM_SHIM` | `true` | 否 | 是否启用 DS Pro 工具调用流式兼容。 |
| `TOOL_CALL_ARGUMENT_CHUNK_SIZE` | `16` | 否 | 每个 arguments delta 的 rune 数。Comate 兼容性验证可调小到 `5`。 |
| `REQUEST_TIMEOUT_SECONDS` | `600` | 否 | 上游请求超时时间。 |

## 4. 源码构建

```bash
cd /path/to/baixin-switch
GOCACHE="$(pwd)/.cache/go-build" go test ./...
GOCACHE="$(pwd)/.cache/go-build" go build -o baixin-switch ./cmd/baixin-switch
```

启动：

```bash
export LISTEN_ADDR="0.0.0.0:11435"
export UPSTREAM_BASE_URL="https://your-ds-pro-gateway.example.com"
export UPSTREAM_API_KEY="sk-..."
export TOOL_CALL_STREAM_SHIM="true"
export TOOL_CALL_ARGUMENT_CHUNK_SIZE="16"
./baixin-switch
```

健康检查：

```bash
curl -sS http://127.0.0.1:11435/health
```

期望返回：

```json
{"service":"baixin-switch","status":"ok"}
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
  -e UPSTREAM_BASE_URL=https://your-ds-pro-gateway.example.com \
  -e UPSTREAM_API_KEY=sk-... \
  -e TOOL_CALL_STREAM_SHIM=true \
  -e TOOL_CALL_ARGUMENT_CHUNK_SIZE=16 \
  baixin-switch:latest
```

检查状态：

```bash
docker logs -f baixin-switch
curl -sS http://127.0.0.1:11435/health
```

停止：

```bash
docker stop baixin-switch
docker rm baixin-switch
```

## 6. docker-compose 示例

创建 `docker-compose.yml`：

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
      UPSTREAM_BASE_URL: "https://your-ds-pro-gateway.example.com"
      UPSTREAM_API_KEY: "sk-..."
      TOOL_CALL_STREAM_SHIM: "true"
      TOOL_CALL_ARGUMENT_CHUNK_SIZE: "16"
      REQUEST_TIMEOUT_SECONDS: "600"
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
UPSTREAM_BASE_URL=https://your-ds-pro-gateway.example.com
UPSTREAM_API_KEY=sk-...
TOOL_CALL_STREAM_SHIM=true
TOOL_CALL_ARGUMENT_CHUNK_SIZE=16
REQUEST_TIMEOUT_SECONDS=600
```

创建 `/etc/systemd/system/baixin-switch.service`：

```ini
[Unit]
Description=baixin-switch OpenAI-compatible proxy
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

`UPSTREAM_BASE_URL` 必须是 base URL：

正确：

```text
https://your-ds-pro-gateway.example.com
https://your-ds-pro-gateway.example.com/openai
```

错误：

```text
https://your-ds-pro-gateway.example.com/v1/chat/completions
```

`baixin-switch` 会自动请求：

```text
${UPSTREAM_BASE_URL}/v1/chat/completions
```

## 9. 上线验证

### 健康检查

```bash
curl -sS http://127.0.0.1:11435/health
```

### 非流式请求

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

### 流式工具调用验证

用 Comate 或内部压测脚本发起带 tools 的流式请求。观察返回：

- `tool_calls[0].id/type/function.name` 只在第一帧出现；
- `tool_calls[0].function.arguments` 在多帧中逐步追加；
- 最终帧保留 `finish_reason: "tool_calls"`；
- 最后保留 `data: [DONE]`。

## 10. 回滚

客户端可以直接切回原上游：

```text
https://your-ds-pro-gateway.example.com/v1
```

服务端回滚：

```bash
docker stop baixin-switch
```

或：

```bash
sudo systemctl stop baixin-switch
```

## 11. 排障

### Comate 仍然不能调用工具

检查：

- Comate base URL 是否配置为 `http://<baixin-switch-host>:11435/v1`；
- 请求是否为 `stream: true`；
- `TOOL_CALL_STREAM_SHIM` 是否为 `true`；
- 上游返回是否是 `text/event-stream`；
- 上游是否真的返回了 `tool_calls[].function.arguments`。

可以临时调小：

```bash
TOOL_CALL_ARGUMENT_CHUNK_SIZE=5
```

### 上游返回 401

检查 `UPSTREAM_API_KEY`。客户端传给 `baixin-switch` 的 Authorization 不会原样转发；如果配置了 `UPSTREAM_API_KEY`，代理会使用它覆盖上游 Authorization。

### 请求路径 404

检查 `UPSTREAM_BASE_URL` 是否多写了 `/v1/chat/completions`。该变量只能写 base URL。

### 首个工具调用仍然等待很久

这是预期限制。代理只能把已经收到的一次性完整 `arguments` 拆成增量返回，不能让旧版 vLLM 提前吐出尚未生成的参数。长期修复仍然是升级 vLLM 到 `v0.22.0+` 或 cherry-pick vLLM PR `#42879`。

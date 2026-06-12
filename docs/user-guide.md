# baixin-switch 使用手册

## 1. 适用场景

`baixin-switch` 让 Comate 或其他 OpenAI-compatible 编程助手用 OpenAI Chat Completions 方式接入内部 DS Pro Anthropic-compatible 服务。

客户端看到的是：

```http
POST /v1/chat/completions
```

上游实际收到的是：

```http
POST /v1/messages
```

这就是当前默认的 `MODE=anthropic_messages`。

## 2. 客户端配置

把 OpenAI-compatible 客户端的 base URL 配为：

```text
http://<baixin-switch-host>:11435/v1
```

本机测试：

```text
http://127.0.0.1:11435/v1
```

API Key：

- 当前版本默认不校验客户端 API Key；
- 如果客户端必须填写，可以填 `dummy`；
- 真正发给上游的 key 来自服务端环境变量 `UPSTREAM_API_KEY`。

模型：

- 客户端可以传 `model`；
- 如果不传，服务端使用 `DEFAULT_MODEL`；
- 如需把客户端模型名改成上游模型名，用 `MODEL_MAP`。

会话 ID：

- 推荐客户端传 `X-Session-ID`；
- 也可以传 `X-Conversation-ID`；
- 如果请求 body 中有 `metadata.session_id`，也会用于会话归因；
- 返回响应会带上 `X-Request-ID` 和 `X-Session-ID`。

## 3. 支持的接口

### 健康检查

```http
GET /health
GET /healthz
```

### 就绪检查

```http
GET /readyz
```

### 指标

```http
GET /metrics
```

### Chat Completions

```http
POST /v1/chat/completions
```

支持：

- `stream: true`
- `stream: false`
- `messages`
- `tools`
- `tool_choice`
- `max_tokens`
- `max_completion_tokens`
- `temperature`
- `top_p`
- `stop`

## 4. 普通对话示例

客户端请求：

```bash
curl -sS http://127.0.0.1:11435/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dummy' \
  -H 'X-Session-ID: sess_demo_001' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "system", "content": "你是一个简洁的编程助手"},
      {"role": "user", "content": "用一句话介绍你自己"}
    ],
    "stream": false
  }'
```

`baixin-switch` 会转换为 Anthropic Messages：

```json
{
  "model": "deepseek-v4-pro",
  "max_tokens": 4096,
  "system": "你是一个简洁的编程助手",
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "用一句话介绍你自己"}
      ]
    }
  ],
  "stream": false
}
```

返回给客户端的仍是 OpenAI Chat Completions 响应。

## 5. 流式请求示例

```bash
curl -N http://127.0.0.1:11435/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dummy' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "查询北京天气"}
    ],
    "stream": true
  }'
```

上游 Anthropic SSE 会被转换为 OpenAI Chat SSE：

```text
data: {"object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}

data: {"object":"chat.completion.chunk","choices":[{"delta":{"content":"..."}}]}

data: {"object":"chat.completion.chunk","choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

## 6. 工具调用示例

```bash
curl -N http://127.0.0.1:11435/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dummy' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "查询北京天气"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather by city name",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string"}
            },
            "required": ["city"]
          }
        }
      }
    ],
    "tool_choice": "auto",
    "stream": true
  }'
```

工具调用映射：

| OpenAI Chat | Anthropic Messages |
| --- | --- |
| `tools[].function.name` | `tools[].name` |
| `tools[].function.parameters` | `tools[].input_schema` |
| `assistant.tool_calls[]` | `assistant.content[].tool_use` |
| `role=tool` | `user.content[].tool_result` |
| `finish_reason=tool_calls` | `stop_reason=tool_use` |

## 7. Comate 接入步骤

1. 部署 `baixin-switch`，确认 `/readyz` 正常。
2. 在 Comate 的 OpenAI-compatible 配置中填写：

```text
Base URL: http://<baixin-switch-host>:11435/v1
API Key: dummy
Model: deepseek-v4-pro
```

3. 发起一次普通对话，确认非流式响应正常。
4. 发起一次工具调用任务，确认 Comate 能收到工具调用、执行工具、继续后续对话。
5. 查看 `/metrics` 和服务日志，确认没有 conversion error。
6. 如果客户端支持自定义 header，建议传入稳定的 `X-Session-ID`，便于按会话查看模型请求与响应。

## 8. 会话观测和 token 统计

默认开启 JSONL 审计：

```text
./logs/usage.jsonl
```

可通过 `AUDIT_LOG_DIR` 修改存储目录，例如 `AUDIT_LOG_DIR=/var/log/baixin-switch`；如需精确指定文件路径，可以使用 `AUDIT_LOG_PATH`。

默认保存：

- request/session id
- 模型名
- 状态码和耗时
- input/output/total tokens
- 脱敏截断后的请求/响应预览

按天统计：

```bash
./scripts/usage_report.sh --by day
```

按周统计：

```bash
./scripts/usage_report.sh --by week
```

按会话汇总：

```bash
./scripts/usage_report.sh --by session
```

查看某个会话的模型请求/响应时间线：

```bash
./scripts/usage_report.sh --session sess_demo_001
```

输出 JSON：

```bash
./scripts/usage_report.sh --by day --json
```

正文采集模式：

```bash
AUDIT_CAPTURE_BODY=off      # 只记录元数据和 token
AUDIT_CAPTURE_BODY=preview  # 默认，脱敏截断预览
AUDIT_CAPTURE_BODY=full     # 脱敏完整正文，谨慎开启
```

## 9. 兼容旧 DS Pro OpenAI passthrough

如果你的上游仍是 OpenAI-compatible `/v1/chat/completions`，并且只需要修复 DS Pro 一次性 tool call SSE，可以用：

```bash
MODE=openai_passthrough
TOOL_CALL_STREAM_SHIM=true
```

此时请求链路变成：

```text
Comate / OpenAI-compatible client
  -> baixin-switch /v1/chat/completions
  -> OpenAI-compatible upstream /v1/chat/completions
```

如果上游已经像 GLM 5.1 一样逐段返回 `function.arguments`，代理会原样透传。

## 10. 常用调试方法

检查服务：

```bash
curl -sS http://<baixin-switch-host>:11435/readyz
curl -sS http://<baixin-switch-host>:11435/metrics
```

检查上游：

```bash
curl -sS "$UPSTREAM_BASE_URL/v1/messages" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $UPSTREAM_API_KEY" \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 128,
    "messages": [
      {"role": "user", "content": [{"type": "text", "text": "ping"}]}
    ]
  }'
```

排障顺序：

1. `/readyz` 是否正常。
2. Comate base URL 是否是 `http://<host>:11435/v1`。
3. `MODE` 是否是预期值。
4. `UPSTREAM_BASE_URL` 是否不带 `/v1/messages`。
5. `UPSTREAM_API_KEY` 是否正确。
6. `/metrics` 是否出现 `baixin_conversion_errors_total` 增长。
7. `/metrics` 是否出现 `baixin_audit_events_total` 和 `baixin_tokens_total` 增长。
8. `./logs/usage.jsonl` 是否有对应 `session_id` 的记录。
9. 服务日志里的 `status`、`duration_ms`、`mode` 是否符合预期。

## 11. 当前限制

- 不做客户端 API Key 校验。
- 暂不支持 `/v1/responses`。
- 暂不支持多上游自动故障转移。
- Anthropic SSE 主路径已经边读边写；旧 `openai_passthrough` DS Pro shim 仍会缓存完整上游 SSE 再输出，后续可以继续改造成 writer 流式。
- JSONL 报表适合单机文件扫描；超大规模长期统计后续可迁移到 SQLite 或 PostgreSQL。

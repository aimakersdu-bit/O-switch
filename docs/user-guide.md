# baixin-switch 使用手册

## 1. 适用场景

`baixin-switch` 用于让 Comate 或其他 OpenAI-compatible 编程助手稳定接入内部 DS Pro 服务。

它解决的问题是：

- DS Pro 某些部署在流式工具调用时，一次性返回完整 `function.arguments`；
- Comate 期望 OpenAI 风格的增量 `function.arguments` delta；
- `baixin-switch` 在中间把一次性工具参数拆成多段流式 delta。

## 2. 客户端配置

把 OpenAI-compatible 客户端的 base URL 配为：

```text
http://<baixin-switch-host>:11435/v1
```

本机测试时：

```text
http://127.0.0.1:11435/v1
```

API Key：

- 如果客户端必须填写，可以填任意占位值，例如 `dummy`；
- 当前版本默认不校验客户端 API Key；
- 上游 API Key 由服务端环境变量 `UPSTREAM_API_KEY` 控制。

## 3. 支持的接口

### 健康检查

```http
GET /health
```

返回：

```json
{"service":"baixin-switch","status":"ok"}
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
- 其他字段透传给上游

## 4. 普通对话示例

```bash
curl -sS http://127.0.0.1:11435/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dummy' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "用一句话介绍你自己"}
    ],
    "stream": false
  }'
```

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

期望工具调用返回形态：

```text
data: ... "tool_calls":[{"index":0,"id":"call_xxx","type":"function","function":{"name":"get_weather","arguments":""}}] ...

data: ... "tool_calls":[{"index":0,"function":{"arguments":"{\"city\""}}] ...

data: ... "tool_calls":[{"index":0,"function":{"arguments":":\"北京\"}"}}] ...

data: ... "finish_reason":"tool_calls" ...

data: [DONE]
```

## 6. Comate 接入步骤

1. 运维部署 `baixin-switch`，确认 `/health` 正常。
2. 在 Comate 的 OpenAI-compatible 配置中填写：

```text
Base URL: http://<baixin-switch-host>:11435/v1
API Key: dummy
Model: deepseek-v4-pro
```

3. 发起一次需要工具调用的任务。
4. 确认 Comate 能收到工具调用、执行工具、继续后续对话。

## 7. 常用调试方法

### 检查服务是否可访问

```bash
curl -sS http://<baixin-switch-host>:11435/health
```

### 检查客户端是否走代理

把 Comate base URL 临时改成一个错误端口，确认请求失败；再改回 `baixin-switch` 地址确认恢复。

### 检查上游是否可用

在部署机器上直接请求上游：

```bash
curl -sS "$UPSTREAM_BASE_URL/v1/chat/completions" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $UPSTREAM_API_KEY" \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [{"role": "user", "content": "ping"}],
    "stream": false
  }'
```

### 调整工具参数拆分大小

如果 Comate 对 chunk 更敏感，可以调小：

```bash
TOOL_CALL_ARGUMENT_CHUNK_SIZE=5
```

如果希望减少 chunk 数量，可以调大：

```bash
TOOL_CALL_ARGUMENT_CHUNK_SIZE=32
```

## 8. 行为说明

### 已经流式返回的模型

如果上游已经像 GLM 5.1 一样逐段返回 `function.arguments`，`baixin-switch` 会原样透传，不会二次拆分。

### 一次性返回的 DS Pro

如果上游一次性返回：

```json
{"function":{"name":"get_weather","arguments":"{\"city\":\"北京\"}"}}
```

代理会改成：

- 第一帧：工具 id/type/name + 空 arguments；
- 后续多帧：只追加 arguments 片段；
- 结束帧：`finish_reason: "tool_calls"`。

### 延迟限制

如果旧版 vLLM 本身等完整工具参数生成后才发第一帧，`baixin-switch` 无法消除这段等待。它只负责把收到后的格式改成 Comate 可消费的流式格式。

## 9. 当前限制

- 不做客户端 API Key 校验。
- 不支持 `/v1/responses`。
- 不支持 Anthropic Messages 协议输入。
- 不支持多上游自动故障转移。
- 不记录请求日志到数据库。

## 10. 推荐排障顺序

1. `/health` 是否正常。
2. Comate base URL 是否是 `http://<host>:11435/v1`。
3. `UPSTREAM_BASE_URL` 是否不带 `/v1/chat/completions`。
4. `UPSTREAM_API_KEY` 是否正确。
5. 请求是否开启 `stream: true`。
6. 上游响应 `Content-Type` 是否是 `text/event-stream`。
7. `TOOL_CALL_STREAM_SHIM` 是否为 `true`。

#!/usr/bin/env python3
# ==============================================================================
# 验证 DS Pro One-shot Tool-call 流式切分兼容性能力的集成测试脚本
# ==============================================================================
import http.server
import json
import time
import urllib.request
import threading
import subprocess
import os
import sys

MOCK_UPSTREAM_PORT = 11438
PROXY_PORT = 11439

class MockOpenAIHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def do_POST(self):
        if self.path == "/v1/chat/completions":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream;charset=utf-8")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "close")
            self.end_headers()

            # 模拟 DeepSeek-V4-Pro 返回的一步到位 (one-shot) 的 arguments payload
            chunks = [
                {
                    "id": "chatcmpl-1",
                    "object": "chat.completion.chunk",
                    "created": 123456,
                    "model": "deepseek-v4-pro",
                    "choices": [{
                        "index": 0,
                        "delta": {
                            "content": None,
                            "tool_calls": [{
                                "index": 0,
                                "id": "call_1",
                                "type": "function",
                                "function": {
                                    "name": "get_weather",
                                    "arguments": '{"city":"北京"}'
                                }
                            }]
                        },
                        "finish_reason": None
                    }]
                },
                {
                    "id": "chatcmpl-1",
                    "object": "chat.completion.chunk",
                    "created": 123457,
                    "model": "deepseek-v4-pro",
                    "choices": [{
                        "index": 0,
                        "delta": {
                            "content": None
                        },
                        "finish_reason": "tool_calls"
                    }]
                }
            ]

            for chunk in chunks:
                payload = f"data: {json.dumps(chunk, ensure_ascii=False)}\n\n"
                self.wfile.write(payload.encode('utf-8'))
                self.wfile.flush()
                time.sleep(0.05)
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        else:
            self.send_response(404)
            self.end_headers()

def start_mock_server():
    server = http.server.HTTPServer(('127.0.0.1', MOCK_UPSTREAM_PORT), MockOpenAIHandler)
    t = threading.Thread(target=server.serve_forever, daemon=True)
    t.start()
    return server

def main():
    print("=== [1/5] 启动 Mock 阿里 OpenAI 兼容端 (Port: {}) ===".format(MOCK_UPSTREAM_PORT))
    mock_server = start_mock_server()

    print("=== [2/5] 启动 baixin-switch 代理服务 (Port: {}) ===".format(PROXY_PORT))
    env = os.environ.copy()
    env["LISTEN_ADDR"] = f"127.0.0.1:{PROXY_PORT}"
    env["MODE"] = "openai_passthrough"
    env["UPSTREAM_BASE_URL"] = f"http://127.0.0.1:{MOCK_UPSTREAM_PORT}"
    env["UPSTREAM_API_KEY"] = "dummy-key"
    env["TOOL_CALL_STREAM_SHIM"] = "true"
    env["TOOL_CALL_ARGUMENT_CHUNK_SIZE"] = "3"  # 切分为每3个字符一片，以便明显看出流式增量效果
    env["AUDIT_ENABLED"] = "false"

    proxy_proc = subprocess.Popen(
        ["./baixin-switch"],
        env=env,
        text=True
    )
    time.sleep(1.0) # 等待代理服务器完全就绪

	# 用来存储响应，防止 traceback 找不到变量
    response_body = None

    try:
        print("=== [3/5] 向代理服务发起流式请求 ===")
        req_data = {
            "model": "deepseek-v4-pro",
            "messages": [{"role": "user", "content": "北京天气如何？"}],
            "stream": True
        }
        req = urllib.request.Request(
            f"http://127.0.0.1:{PROXY_PORT}/v1/chat/completions",
            data=json.dumps(req_data).encode('utf-8'),
            headers={"Content-Type": "application/json"}
        )

        chunks_received = []
        with urllib.request.urlopen(req) as response:
            for line in response:
                line_str = line.decode('utf-8').strip()
                if line_str.startswith("data:"):
                    data_val = line_str[5:].strip()
                    if data_val == "[DONE]":
                        print("收到 [DONE]")
                        break
                    chunk_json = json.loads(data_val)
                    chunks_received.append(chunk_json)

        print("\n=== [4/5] 检查代理返回的切分后的事件帧 ===")
        print(f"总计收到 {len(chunks_received)} 个数据帧:")
        for idx, chunk in enumerate(chunks_received):
            print(f"帧 #{idx}: {json.dumps(chunk, ensure_ascii=False)}")

        # 校验：
        # 1. 第一个含有 tool_calls 的帧中 arguments 应该被初始化为 ""
        # 2. 后续的帧中 arguments 应该被流式增量切分（每帧包含3个字符）
        # 3. 拼接所有 arguments 应等于原始的 '{"city":"北京"}'

        first_tool_call_chunk = chunks_received[0]
        tc_meta = first_tool_call_chunk["choices"][0]["delta"]["tool_calls"][0]
        assert tc_meta["id"] == "call_1", "首个事件应当包含 tool call ID"
        assert tc_meta["function"]["name"] == "get_weather", "首个事件应当包含函数名"
        assert tc_meta["function"]["arguments"] == "", "首个事件应当将 arguments 初始化为空字符串"

        concatenated_args = ""
        delta_chunks_count = 0
        for chunk in chunks_received[1:]:
            choices = chunk["choices"]
            if len(choices) == 0:
                continue
            delta = choices[0]["delta"]
            if "tool_calls" in delta:
                arg_part = delta["tool_calls"][0]["function"]["arguments"]
                concatenated_args += arg_part
                delta_chunks_count += 1
                print(f"  -> 参数片 #{delta_chunks_count}: {repr(arg_part)}")

        print(f"\n拼接后的 Arguments: {repr(concatenated_args)}")
        print(f"切分的数据帧个数: {delta_chunks_count}")

        assert concatenated_args == '{"city":"北京"}', "参数拼接内容不符"
        assert delta_chunks_count > 1, f"应该包含多次切分分批传输，实际只有 {delta_chunks_count} 次"

        print("\n=== [5/5] 集成测试结果: PASS ===")
        sys.exit(0)

    except Exception as e:
        print(f"\n=== 集成测试结果: FAIL ===")
        import traceback
        traceback.print_exc()
        proxy_proc.terminate()
        sys.exit(1)
    finally:
        proxy_proc.terminate()
        mock_server.shutdown()

if __name__ == "__main__":
    main()

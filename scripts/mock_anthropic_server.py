#!/usr/bin/env python3
# ==============================================================================
# Mock Anthropic Messages API 服务端 (标准库实现)
# ==============================================================================
import http.server
import json
import time

class MockAnthropicHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # 抑制请求日志输出，使控制台保持干净
        pass

    def do_POST(self):
        if self.path == "/v1/messages":
            # 读取请求体
            content_length = int(self.headers.get('Content-Length', 0))
            body = self.rfile.read(content_length).decode('utf-8')
            try:
                req_data = json.loads(body)
            except Exception:
                req_data = {}
            
            # 返回 HTTP 200 和 SSE 头
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream;charset=utf-8")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.end_headers()
            
            # 模拟 Anthropic 协议下的工具调用流式数据帧
            events = [
                ("message_start", {
                    "type": "message_start",
                    "message": {
                        "id": "msg_123",
                        "type": "message",
                        "role": "assistant",
                        "content": [],
                        "model": req_data.get("model", "claude-3-5-sonnet"),
                        "stop_reason": None,
                        "stop_sequence": None,
                        "usage": {"input_tokens": 10, "output_tokens": 0}
                    }
                }),
                ("content_block_start", {
                    "type": "content_block_start",
                    "index": 0,
                    "content_block": {
                        "type": "tool_use",
                        "id": "toolu_01",
                        "name": "get_weather",
                        "input": {}
                    }
                }),
                ("content_block_delta", {
                    "type": "content_block_delta",
                    "index": 0,
                    "delta": {
                        "type": "input_json_delta",
                        "partial_json": "{\""
                    }
                }),
                ("content_block_delta", {
                    "type": "content_block_delta",
                    "index": 0,
                    "delta": {
                        "type": "input_json_delta",
                        "partial_json": "city\": \"北京\"}"
                    }
                }),
                ("content_block_stop", {
                    "type": "content_block_stop",
                    "index": 0
                }),
                ("message_delta", {
                    "type": "message_delta",
                    "delta": {
                        "stop_reason": "tool_use"
                    },
                    "usage": {
                        "output_tokens": 15
                    }
                }),
                ("message_stop", {
                    "type": "message_stop"
                })
            ]
            
            for event_type, data in events:
                payload = f"event: {event_type}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"
                self.wfile.write(payload.encode('utf-8'))
                self.wfile.flush()
                time.sleep(0.05)
        else:
            self.send_response(404)
            self.end_headers()

def run(port=11436):
    server_address = ('127.0.0.1', port)
    httpd = http.server.HTTPServer(server_address, MockAnthropicHandler)
    print(f"Mock Anthropic Server running on http://127.0.0.1:{port}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        httpd.server_close()

if __name__ == "__main__":
    run()

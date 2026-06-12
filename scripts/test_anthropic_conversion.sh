#!/bin/bash
# ==============================================================================
# baixin-switch Anthropic Messages 转换模式端到端集成测试脚本
# ==============================================================================
set -e

PROXY_PORT=11435
MOCK_PORT=11436

echo "=================================================="
echo " 开始测试 Anthropic 协议转换 (OpenAI -> Anthropic -> OpenAI)..."
echo "=================================================="

# 确保编译好的二进制存在
if [ ! -f "./baixin-switch" ]; then
    echo ">> 编译二进制文件..."
    go build -o baixin-switch cmd/baixin-switch/main.go
fi

# 1. 启动 Mock Anthropic 服务端
echo ">> 启动 Mock Anthropic 节点 (端口: ${MOCK_PORT})..."
python3 scripts/mock_anthropic_server.py &
MOCK_PID=$!

# 2. 启动 baixin-switch 代理服务端 (切换为 anthropic_messages 模式)
echo ">> 启动 baixin-switch 代理服务 (端口: ${PROXY_PORT})..."
export LISTEN_ADDR="127.0.0.1:${PROXY_PORT}"
export MODE="anthropic_messages"
export UPSTREAM_BASE_URL="http://127.0.0.1:${MOCK_PORT}"
export UPSTREAM_API_KEY="mock-api-key"
export AUDIT_ENABLED="false"

./baixin-switch &
PROXY_PID=$!

cleanup() {
    echo ">> 清理测试服务进程..."
    kill $MOCK_PID 2>/dev/null || true
    kill $PROXY_PID 2>/dev/null || true
}
trap cleanup EXIT

# 等待服务启动
sleep 1.5

# 3. 使用百度给的 check_model_response.sh 脚本检测代理对外的 OpenAI 兼容接口格式
echo ">> 运行百度格式规范检测脚本..."
chmod +x scripts/check_model_response.sh
./scripts/check_model_response.sh \
    -u "http://127.0.0.1:${PROXY_PORT}/v1/chat/completions" \
    -m "claude-3-5-sonnet" \
    -k "mock-api-key"

echo -e "\n=================================================="
echo -e "\033[0;32m[PASS] Anthropic 转换模式下对外的 OpenAI 格式完全验证通过！\033[0m"
echo "=================================================="

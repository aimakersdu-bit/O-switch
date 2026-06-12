#!/bin/bash
# ==============================================================================
# baixin-switch 日志格式与字段验证脚本
# ==============================================================================
set -e

LOG_FILE="test_server.log"
PORT=11435

echo "=================================================="
echo " 开始验证 baixin-switch 日志格式..."
echo "=================================================="

# 清理旧的日志文件
rm -f "${LOG_FILE}"

# 1. 在后台启动服务
echo ">> 启动 baixin-switch 服务..."
export LISTEN_ADDR="127.0.0.1:${PORT}"
export UPSTREAM_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode"
export MODE="openai_passthrough"

./baixin-switch > "${LOG_FILE}" 2>&1 &
PID=$!

# 确保脚本退出时一定会杀死后台服务并清理日志
cleanup() {
    echo ">> 清理服务进程..."
    kill $PID 2>/dev/null || true
    rm -f "${LOG_FILE}"
}
trap cleanup EXIT

# 等待服务启动
sleep 1.5

# 2. 发送一个 POST /v1/chat/completions 请求以触发访问日志记录
echo ">> 发送测试请求到 /v1/chat/completions..."
curl -s -X POST "http://127.0.0.1:${PORT}/v1/chat/completions" \
     -H "Content-Type: application/json" \
     -d '{"messages": [{"role": "user", "content": "test"}]}' > /dev/null || true

# 3. 停止服务以确保日志完全刷盘
echo ">> 停止服务..."
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true

# 4. 检查日志内容
if [ ! -f "${LOG_FILE}" ]; then
    echo -e "\033[0;31m[FAIL]\033[0m 未生成日志文件 ${LOG_FILE}"
    exit 1
fi

echo ">> 捕获到的日志内容："
cat "${LOG_FILE}"
echo "--------------------------------------------------"

# 查找包含 request 的 JSON 日志行
JSON_LINE=$(grep '"msg":"request"' "${LOG_FILE}" || true)

if [ -z "${JSON_LINE}" ]; then
    echo -e "\033[0;31m[FAIL]\033[0m 未在日志中找到包含 '\"msg\":\"request\"' 的 JSON 访问日志"
    exit 1
fi

echo ">> 发现访问日志行: ${JSON_LINE}"

# 5. 校验字段
FAILED=0

check_field() {
    local field=$1
    local expected=$2
    
    if echo "${JSON_LINE}" | grep -q "\"${field}\""; then
        if [ -n "${expected}" ]; then
            if echo "${JSON_LINE}" | grep -q "\"${field}\":\"${expected}\"" || echo "${JSON_LINE}" | grep -q "\"${field}\":${expected}"; then
                echo -e "   [PASS] 字段 '${field}' 存在且值匹配 '${expected}'"
            else
                echo -e "   \033[0;31m[FAIL]\033[0m 字段 '${field}' 存在但值不匹配 (期望: ${expected}，实际: $(echo "${JSON_LINE}" | grep -o "\"${field}\":[^\,]*"))"
                FAILED=1
            fi
        else
            echo -e "   [PASS] 字段 '${field}' 存在"
        fi
    else
        echo -e "   \033[0;31m[FAIL]\033[0m 缺少字段 '${field}'"
        FAILED=1
    fi
}

echo ">> 验证日志字段和 JSON 格式..."
check_field "time" ""
check_field "level" "INFO"
check_field "msg" "request"
check_field "method" "POST"
check_field "path" "/v1/chat/completions"
check_field "status" ""
check_field "duration_ms" ""
check_field "mode" "openai_passthrough"

if [ $FAILED -eq 0 ]; then
    echo "=================================================="
    echo -e "\033[0;32m[PASS] 日志格式及字段全部验证通过！符合 JSON Access Log 规范。\033[0m"
    echo "=================================================="
    exit 0
else
    echo "=================================================="
    echo -e "\033[0;31m[FAIL] 日志验证未通过，请检查日志格式实现。\033[0m"
    echo "=================================================="
    exit 1
fi

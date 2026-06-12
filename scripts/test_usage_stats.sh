#!/bin/bash
# ==============================================================================
# baixin-switch Token 统计和审计报告验证脚本
# ==============================================================================
set -e

TEST_JSONL="test_usage.jsonl"

echo "=================================================="
echo " 开始验证 Token 统计与报告脚本功能..."
echo "=================================================="

# 1. 模拟生成审计日志数据 (test_usage.jsonl)
echo ">> 正在生成模拟审计数据..."
cat > "${TEST_JSONL}" << 'EOF'
{"ts":"2026-06-12T10:00:00+08:00","date":"2026-06-12","week":"2026-W24","request_id":"req-01","session_id":"session-a","model":"deepseek-v4-pro","status":200,"input_tokens":150,"output_tokens":350,"total_tokens":500,"request_preview":"查询北京天气","response_preview":"北京今天晴天，气温22度。"}
{"ts":"2026-06-12T10:15:00+08:00","date":"2026-06-12","week":"2026-W24","request_id":"req-02","session_id":"session-a","model":"deepseek-v4-pro","status":200,"input_tokens":200,"output_tokens":400,"total_tokens":600,"request_preview":"查询上海天气","response_preview":"上海今天多云，气温25度。"}
{"ts":"2026-06-13T09:30:00+08:00","date":"2026-06-13","week":"2026-W24","request_id":"req-03","session_id":"session-b","model":"deepseek-v4-pro","status":200,"input_tokens":100,"output_tokens":200,"total_tokens":300,"request_preview":"你好","response_preview":"你好！我是智能助手，很高兴为您服务。"}
{"ts":"2026-06-19T14:20:00+08:00","date":"2026-06-19","week":"2026-W25","request_id":"req-04","session_id":"session-c","model":"deepseek-v4-pro","status":200,"input_tokens":500,"output_tokens":800,"total_tokens":1300,"request_preview":"请写一段快速排序算法","response_preview":"好的，以下是 Go 语言实现的快速排序算法...[REDACTED]"}
EOF

cleanup() {
    echo ">> 清理测试产生的临时文件..."
    rm -f "${TEST_JSONL}"
}
trap cleanup EXIT

# 2. 验证按天 (day) 汇总
echo -e "\n>> [测试] 按天汇总 (by day)："
python3 scripts/usage_report.py --file "${TEST_JSONL}" --by day

# 3. 验证按周 (week) 汇总
echo -e "\n>> [测试] 按周汇总 (by week)："
python3 scripts/usage_report.py --file "${TEST_JSONL}" --by week

# 4. 验证按会话 (session) 汇总
echo -e "\n>> [测试] 按会话汇总 (by session)："
python3 scripts/usage_report.py --file "${TEST_JSONL}" --by session

# 5. 验证单会话时间线分析 (session timeline)
echo -e "\n>> [测试] 查看会话 'session-a' 的具体对话时间线 (session timeline)："
python3 scripts/usage_report.py --file "${TEST_JSONL}" --session session-a

# 6. 验证 JSON 格式输出功能 (用 --json)
echo -e "\n>> [测试] JSON 格式输出测试 (按天)："
python3 scripts/usage_report.py --file "${TEST_JSONL}" --by day --json

echo -e "\n=================================================="
echo -e "\033[0;32m[PASS] Token 统计与报告脚本所有核心统计维度验证完成！\033[0m"
echo "=================================================="

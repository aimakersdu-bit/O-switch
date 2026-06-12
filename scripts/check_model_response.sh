#!/bin/bash

# ============================================
# Comate 4.0 模型返回格式检查脚本
# 支持 OpenAI 格式 (/v1/chat/completions) 和 Anthropic 格式 (/v1/messages)
# ============================================

# set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 全局变量
CHECK_PASSED=0
CHECK_FAILED=0

# 打印带颜色的信息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

print_error() {
    echo -e "${RED}[FAIL]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_title() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}\n"
}

# 显示用法
usage() {
    cat << EOF
用法: $0 [选项]

选项:
    -u, --url <URL>          模型服务地址 (必需)
    -m, --model <MODEL>      模型名称 (必需)
    -k, --apikey <APIKEY>    API Key (可选, 默认: EMPTY)
    -h, --help               显示此帮助信息

使用方式:

  方式1 - 交互式模式 (推荐):
    $0
    然后按提示输入 IP:端口 (如: 10.174.145.94:8366)
    脚本会自动检测 OpenAI 和 Anthropic 两种格式

  方式2 - 命令行模式 (检查指定格式):
    $0 -u http://localhost:8806/v1/chat/completions -m model-name -k my-api-key
    $0 -u http://localhost:8806/v1/messages -m model-name -k my-api-key

说明:
    本脚本用于检查模型返回格式是否符合 Comate 4.0 的 FunctionCalling 要求。
    格式类型会根据请求地址自动判断:
      - 地址包含 /chat/completions -> OpenAI 格式
      - 地址包含 /messages -> Anthropic 格式
EOF
}

# 根据 URL 自动检测格式类型
detect_format_type() {
    local url="$1"
    
    if [[ "$url" == *"/chat/completions"* ]]; then
        echo "openai"
    elif [[ "$url" == *"/messages"* ]]; then
        echo "anthropic"
    else
        echo ""
    fi
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -u|--url)
                MODEL_URL="$2"
                shift 2
                ;;
            -k|--apikey)
                API_KEY="$2"
                shift 2
                ;;
            -m|--model)
                MODEL_NAME="$2"
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                print_error "未知参数: $1"
                usage
                exit 1
                ;;
        esac
    done

    # 设置默认值
    API_KEY="${API_KEY:-EMPTY}"

    # 验证必需参数
    if [[ -z "$MODEL_URL" ]]; then
        print_error "缺少必需参数: 模型服务地址 (-u, --url)"
        usage
        exit 1
    fi

    if [[ -z "$MODEL_NAME" ]]; then
        print_error "缺少必需参数: 模型名称 (-m, --model)"
        usage
        exit 1
    fi

    # 自动检测格式类型
    FORMAT_TYPE=$(detect_format_type "$MODEL_URL")
    
    if [[ -z "$FORMAT_TYPE" ]]; then
        print_error "无法从 URL 自动识别格式类型"
        print_info "URL 应包含 /chat/completions (OpenAI 格式) 或 /messages (Anthropic 格式)"
        print_info "当前 URL: $MODEL_URL"
        exit 1
    fi
}

# 检查 curl 是否安装
check_dependencies() {
    if ! command -v curl &> /dev/null; then
        print_error "curl 未安装,请先安装 curl"
        exit 1
    fi
}

# 打印请求信息
print_request_info() {
    print_title "请求信息"
    print_info "请求地址: $MODEL_URL"
    if [[ "$FORMAT_TYPE" == "openai" ]]; then
        print_info "格式类型: OpenAI (自动检测)"
    else
        print_info "格式类型: Anthropic (自动检测)"
    fi
    print_info "模型名称: $MODEL_NAME"
    print_info "API Key: ${API_KEY:0:4}****"
}

# 构建 OpenAI 格式请求
build_openai_request() {
    cat << 'EOF'
{
    "model": "MODEL_PLACEHOLDER",
    "messages": [
        {"role": "user", "content": "查询北京的天气"}
    ],
    "stream": true,
    "tools": [
        {
            "type": "function",
            "function": {
                "name": "get_weather",
                "description": "获取指定城市的天气信息",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "city": {"type": "string", "description": "城市名称"},
                        "date": {"type": "string", "description": "日期"}
                    },
                    "required": ["city"]
                }
            }
        }
    ],
    "tool_choice": "auto"
}
EOF
}

# 构建 Anthropic 格式请求
build_anthropic_request() {
    cat << 'EOF'
{
    "model": "MODEL_PLACEHOLDER",
    "messages": [
        {"role": "user", "content": "列出 /User/work 目录下的文件"}
    ],
    "stream": true,
    "tools": [
        {
            "name": "list_dir",
            "description": "列出指定目录的文件",
            "input_schema": {
                "type": "object",
                "properties": {
                    "target_directory": {"type": "string", "description": "目标目录路径"}
                },
                "required": ["target_directory"]
            }
        }
    ],
    "max_tokens": 1024
}
EOF
}

# 发送请求并保存响应
send_request() {
    local output_file="$1"
    
    print_info "正在发送请求..."
    
    if [[ "$FORMAT_TYPE" == "openai" ]]; then
        local request_body=$(build_openai_request | sed "s/MODEL_PLACEHOLDER/$MODEL_NAME/")
        print_title "请求体 (OpenAI)"
        echo "$request_body"
        
        curl -s -N "$MODEL_URL" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $API_KEY" \
            -d "$request_body" \
            2>/dev/null > "$output_file"
    else
        local request_body=$(build_anthropic_request | sed "s/MODEL_PLACEHOLDER/$MODEL_NAME/")
        print_title "请求体 (Anthropic)"
        echo "$request_body"
        
        curl -s -N "$MODEL_URL" \
            -H "Content-Type: application/json" \
            -H "x-api-key: $API_KEY" \
            -H "anthropic-version: 2023-06-01" \
            -d "$request_body" \
            2>/dev/null > "$output_file"
    fi
    
    if [[ $? -ne 0 ]]; then
        print_error "请求失败,请检查地址和网络连接"
        exit 1
    fi
    
    # 检查响应中是否包含模型不存在错误
    if grep -q '"The model.*does not exist"' "$output_file" 2>/dev/null || \
       grep -q '"model.*does not exist"' "$output_file" 2>/dev/null || \
       grep -q 'NotFoundError' "$output_file" 2>/dev/null; then
        print_error "模型名称错误: $MODEL_NAME"
        print_info "请检查模型启动命令中的 --served-model-name 参数"
        print_info "当前响应:"
        cat "$output_file"
        exit 1
    fi
    
    print_success "请求完成,响应已保存"
}

# 纯 Bash JSON 处理函数

# 从 JSON 字符串中提取字符串字段值
# 支持简单路径如 "name" 或嵌套路径如 "choices.delta.tool_calls.name"
json_get_string() {
    local json="$1"
    local field_path="$2"
    local default="${3:-}"
    
    local current="$json"
    local IFS='.'
    local fields
    read -ra fields <<< "$field_path"
    
    for field in "${fields[@]}"; do
        # 检查是否是数组索引，如 "tool_calls[0]"
        if [[ "$field" =~ ^(.+)\[([0-9]+)\]$ ]]; then
            local arr_name="${BASH_REMATCH[1]}"
            local idx="${BASH_REMATCH[2]}"
            # 提取数组
            local arr=$(echo "$current" | grep -o "\"$arr_name\"[[:space:]]*:[[:space:]]*\[[^\]]*\]" | head -1)
            # 提取指定索引的对象
            local i=0
            local found=""
            local items="${arr#*:[}"
            items="${items%]}"
            # 简单的对象分割（可能不完美，但对于简单JSON够用）
            while [[ "$items" == *"{"* ]]; do
                local obj="${items#*\{}"
                obj="{${obj%%\}*}"
                if [[ $i -eq $idx ]]; then
                    found="$obj}"
                    break
                fi
                items="${items#*\}}"
                ((i++))
            done
            if [[ -n "$found" ]]; then
                current="$found"
            else
                echo "$default"
                return 1
            fi
        else
            # 提取普通字段
            # 先尝试提取字符串值
            local pattern="\"$field\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\""
            local val=$(echo "$current" | sed -n "s/.*$pattern.*/\1/p" | head -1)
            if [[ -n "$val" ]]; then
                if [[ "$field" == "${fields[-1]}" ]]; then
                    echo "$val"
                    return 0
                fi
                # 不是最后一个字段，继续解析
                current="{\"$field\":\"$val\"}"
                continue
            fi
            
            # 尝试提取对象值
            local obj_pattern="\"$field\"[[:space:]]*:[[:space:]]*\({[^{}]*}\)"
            local obj=$(echo "$current" | sed -n "s/.*$obj_pattern.*/\1/p" | head -1)
            if [[ -n "$obj" ]]; then
                current="$obj"
            else
                echo "$default"
                return 1
            fi
        fi
    done
    
    echo "$current"
}

# 检查 JSON 中是否存在某字段 (简单检查)
json_has_field() {
    local json="$1"
    local field="$2"
    echo "$json" | grep -q "\"$field\"[[:space:]]*:"
}

# 提取 tool_calls 数组中的第一个对象的指定字段
json_get_tool_call_field() {
    local json="$1"
    local field="$2"
    
    # 先提取 tool_calls 数组
    local arr=$(echo "$json" | grep -o '"tool_calls"[[:space:]]*:[[:space:]]*\[[^]]*\]' | head -1)
    if [[ -z "$arr" ]]; then
        return 1
    fi
    
    # 提取第一个对象中的字段
    # 支持两种格式: "name": "value" 或 "function": {"name": "value"}
    local val
    
    # 尝试直接提取
    val=$(echo "$arr" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed 's/.*:"\(.*\)".*/\1/')
    if [[ -n "$val" ]]; then
        echo "$val"
        return 0
    fi
    
    # 尝试从 function 对象中提取
    local func=$(echo "$arr" | grep -o '"function"[[:space:]]*:[[:space:]]*{[^}]*}' | head -1)
    if [[ -n "$func" ]]; then
        val=$(echo "$func" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed 's/.*:"\(.*\)".*/\1/')
        if [[ -n "$val" ]]; then
            echo "$val"
            return 0
        fi
    fi
    
    return 1
}

# 计算 tool_calls 数组长度
# 通过统计 tool_calls 数组中的 "function" 字段数量来确定
json_tool_calls_length() {
    local json="$1"

    # 方法：提取 tool_calls 数组内容，统计其中的 function 字段数量
    # 每个 tool_calls 元素都有一个 function 字段
    local arr=$(echo "$json" | grep -o '"tool_calls"[[:space:]]*:[[:space:]]*\[.*\]' | head -1)
    if [[ -z "$arr" ]]; then
        echo "0"
        return
    fi

    # 统计 "function": 的出现次数（在 tool_calls 数组内）
    local count=$(echo "$arr" | grep -o '"function"[[:space:]]*:' | wc -l | tr -d ' ')
    echo "$count"
}

# 从 chunk 中提取 arguments 字段
json_get_arguments() {
    local json="$1"

    # 使用 sed 提取 arguments 字段的值
    # 首先找到 "arguments": 开头的部分，然后提取双引号之间的内容
    local args=$(echo "$json" | sed -n 's/.*"arguments"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)

    # 如果提取为空，可能是因为 arguments 值中包含转义引号
    # 这种情况下，只要检测到 "arguments": 且不是空字符串就认为有值
    if [[ -z "$args" ]]; then
        # 检查是否是 "arguments":"" 这种空值
        if echo "$json" | grep -q '"arguments"[[:space:]]*:[[:space:]]*""'; then
            # 空值，不返回
            return 1
        fi
        # 否则检查是否存在非空的 arguments 字段（有内容但含转义引号）
        if echo "$json" | grep -q '"arguments"[[:space:]]*:[[:space:]]*"[^"]'; then
            # 有非空的 arguments
            echo "has_value"
            return 0
        fi
        return 1
    fi

    if [[ -n "$args" ]]; then
        echo "$args"
        return 0
    fi

    return 1
}

# 格式化 JSON (美化输出)
format_json() {
    # 不使用 jq，直接原样输出
    echo "$1"
}

# 检查 OpenAI 格式响应
check_openai_response() {
    local response_file="$1"
    
    print_title "开始检查 OpenAI 格式"
    
    # 检查文件是否为空
    if [[ ! -s "$response_file" ]]; then
        print_error "响应文件为空"
        return 1
    fi
    
    # 读取所有 data: 行
    local data_lines=()
    while IFS= read -r line; do
        if [[ "$line" == data:* ]]; then
            data_lines+=("$line")
        fi
    done < "$response_file"
    
    if [[ ${#data_lines[@]} -eq 0 ]]; then
        print_error "未找到任何 data: 行,响应格式不正确"
        print_info "当前响应内容:"
        cat "$response_file"
        return 1
    fi
    
    # 提取包含 tool_calls 的 chunks
    local found_tool_calls=0
    local first_tool_call_chunk=""
    local tool_call_chunks=()
    
    for line in "${data_lines[@]}"; do
        local json_data="${line#data: }"
        json_data=$(echo "$json_data" | sed 's/\r$//')
        
        if [[ "$json_data" == "[DONE]" ]]; then
            continue
        fi
        
        # 检查是否包含 tool_calls 字段 (用 '"tool_calls":' 避免匹配到 finish_reason 中的值)
        if echo "$json_data" | grep -q '"tool_calls":'; then
            if [[ $found_tool_calls -eq 0 ]]; then
                first_tool_call_chunk="$json_data"
            fi
            found_tool_calls=1
            tool_call_chunks+=("$json_data")
        fi
    done
    
    # 只展示包含 tool_calls 的 chunks
    print_title "包含 tool_calls 的响应内容"
    print_info "共找到 ${#tool_call_chunks[@]} 个包含 tool_calls 的 chunks"
    echo ""
    local idx=1
    for chunk in "${tool_call_chunks[@]}"; do
        echo -e "${BLUE}[$idx]${NC}"
        format_json "$chunk"
        echo ""
        ((idx++))
    done
    
    if [[ $found_tool_calls -eq 0 ]]; then
        print_error "未找到 tool_calls 字段,模型可能不支持 FunctionCalling"
        print_info "请确保模型部署时启用了 --enable-auto-tool-choice 和 --tool-call-parser 参数"
        print_info "当前响应内容:"
        cat "$response_file"
        return 1
    fi
    
    print_success "找到 tool_calls 字段"
    
    # 检查第一个 tool_calls chunk (首个包含 tool_calls 的 chunk)
    print_title "检查规则 1: 首个包含 tool_calls 的 chunk 应包含 name 字段"
    
    # 支持两种可能的结构:
    # 1. tool_calls[0].name (标准 OpenAI 格式)
    # 2. tool_calls[0].function.name (部分模型格式,如 minimax)
    local name_value=""
    local has_name=0
    
    # 使用纯 bash 函数提取 name
    name_value=$(json_get_tool_call_field "$first_tool_call_chunk" "name")
    if [[ -n "$name_value" ]]; then
        has_name=1
    fi
    
    if [[ $has_name -eq 1 && -n "$name_value" && "$name_value" != "null" ]]; then
        print_success "首个包含 tool_calls 的 chunk 包含 name 字段,值为: $name_value"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 首个包含 tool_calls 的 chunk 应包含 name 字段"
        print_info "应该是什么样:"
        print_info '  结构1: {"choices":[{"delta":{"tool_calls":[{"name":"get_weather",...}]}}]}'
        print_info '  结构2: {"choices":[{"delta":{"tool_calls":[{"function":{"name":"get_weather",...}}]}}]}'
        print_info "当前输出 (首个包含 tool_calls 的 chunk):"
        format_json "$first_tool_call_chunk"
        ((CHECK_FAILED++))
    fi
    
    # 检查首个包含 tool_calls 的 chunk 只有一个元素
    print_title "检查规则 2: 首个包含 tool_calls 的 chunk 应只有一个元素"
    
    local tool_calls_count=$(json_tool_calls_length "$first_tool_call_chunk")
    if [[ "$tool_calls_count" -eq 1 ]]; then
        print_success "首个包含 tool_calls 的 chunk 只有一个 tool_calls 元素"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 首个包含 tool_calls 的 chunk 应只有一个元素"
        print_info "应该是什么样:"
        print_info "  tool_calls 数组长度为 1"
        print_info "当前输出:"
        print_info "  tool_calls 数组长度为 $tool_calls_count"
        format_json "$first_tool_call_chunk"
        ((CHECK_FAILED++))
    fi
    
    # 检查后续包含 tool_calls 的 chunks (从第二个 tool_calls chunk 开始)
    print_title "检查规则 3: 后续包含 tool_calls 的 chunks 不应包含 name 字段"
    
    local subsequent_violations=0
    local first_violation_chunk=""
    for ((i=1; i<${#tool_call_chunks[@]}; i++)); do
        local chunk="${tool_call_chunks[$i]}"
        # 检查 name 字段是否存在
        local name_val=$(json_get_tool_call_field "$chunk" "name")
        if [[ -n "$name_val" ]]; then
            if [[ $subsequent_violations -eq 0 ]]; then
                first_violation_chunk="$chunk"
            fi
            ((subsequent_violations++))
        fi
    done
    
    if [[ $subsequent_violations -eq 0 ]]; then
        print_success "后续包含 tool_calls 的 chunks 均不包含 name 字段"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 后续包含 tool_calls 的 chunks 不应包含 name 字段"
        print_info "应该是什么样:"
        print_info '  后续 chunks 只包含 arguments 字段,如: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{..."}}]}}]}'
        print_info "发现问题数: $subsequent_violations 个 chunks 包含 name 字段"
        print_info "第一个有问题的 chunk:"
        format_json "$first_violation_chunk"
        ((CHECK_FAILED++))
    fi
    
    # 检查后续包含 tool_calls 的 chunks 包含 arguments 字段
    print_title "检查规则 4: 后续包含 tool_calls 的 chunks 应包含非空的 arguments 字段"
    
    local args_violations=0
    local found_args=0
    local first_args_violation_chunk=""
    
    # 从第二个 chunk 开始遍历 (i=1)
    for ((i=1; i<${#tool_call_chunks[@]}; i++)); do
        local chunk="${tool_call_chunks[$i]}"
        
        # 检查 arguments 字段
        local args_value=$(json_get_arguments "$chunk")
        
        if [[ -n "$args_value" ]]; then
            found_args=1
        else
            if [[ $args_violations -eq 0 ]]; then
                first_args_violation_chunk="$chunk"
            fi
            ((args_violations++))
        fi
    done
    
    if [[ $found_args -eq 1 && $args_violations -eq 0 ]]; then
        print_success "后续包含 tool_calls 的 chunks 正确包含 arguments 字段"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 后续包含 tool_calls 的 chunks 应包含非空的 arguments 字段"
        print_info "应该是什么样:"
        print_info '  每个包含 tool_calls 的后续 chunk 都应包含 arguments 字段,逐步构建 JSON 参数'
        print_info "当前输出:"
        print_info "  包含 tool_calls 的 chunks 数: ${#tool_call_chunks[@]}"
        if [[ $args_violations -gt 0 ]]; then
            print_info "  缺少 arguments 字段的 chunks 数: $args_violations"
            print_info "  第一个缺少 arguments 的 chunk:"
            format_json "$first_args_violation_chunk"
        fi
        ((CHECK_FAILED++))
    fi
    
    # 检查流式返回 (多个 tool_calls chunks)
    print_title "检查规则 5: 应支持流式返回 (多个包含 tool_calls 的 chunks)"
    
    if [[ ${#tool_call_chunks[@]} -gt 1 ]]; then
        print_success "响应以流式方式返回,共 ${#tool_call_chunks[@]} 个包含 tool_calls 的 chunks"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 应支持流式返回"
        print_info "应该是什么样:"
        print_info "  多个包含 tool_calls 的 data: 行,逐步返回 tool_calls 的各部分"
        print_info "当前输出:"
        print_info "  仅找到 ${#tool_call_chunks[@]} 个包含 tool_calls 的 chunk"
        ((CHECK_FAILED++))
    fi
    
    # 检查 finish_reason (在所有 data_lines 中查找)
    print_title "检查规则 6: 最后一个 chunk 应包含 finish_reason: tool_calls"
    
    local last_data_line="${data_lines[${#data_lines[@]}-1]}"
    local last_json="${last_data_line#data: }"
    last_json=$(echo "$last_json" | sed 's/\r$//')
    
    if [[ "$last_json" == "[DONE]" && ${#data_lines[@]} -gt 1 ]]; then
        last_data_line="${data_lines[${#data_lines[@]}-2]}"
        last_json="${last_data_line#data: }"
        last_json=$(echo "$last_json" | sed 's/\r$//')
    fi
    
    if echo "$last_json" | grep -q '"finish_reason":"tool_calls"'; then
        print_success "最后一个 chunk 包含 finish_reason: tool_calls"
        ((CHECK_PASSED++))
    elif json_has_field "$last_json" "finish_reason"; then
        # 提取 finish_reason 值
        local finish_reason=$(echo "$last_json" | grep -o '"finish_reason"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
        if [[ "$finish_reason" == "tool_calls" ]]; then
            print_success "最后一个 chunk 包含 finish_reason: tool_calls"
            ((CHECK_PASSED++))
        else
            print_warn "最后一个 chunk 的 finish_reason 为: $finish_reason (期望: tool_calls)"
        fi
    else
        print_warn "未找到明确的 finish_reason: tool_calls (这可能不是严重问题)"
    fi
    
    return 0
}

# 检查 Anthropic 格式响应
# 新逻辑：查找 "content_block":{"type":"tool_use" 作为开始和结束标记
check_anthropic_response() {
    local response_file="$1"
    
    print_title "开始检查 Anthropic 格式"
    
    # 检查文件是否为空
    if [[ ! -s "$response_file" ]]; then
        print_error "响应文件为空"
        return 1
    fi
    
    # 读取所有 SSE 事件 (event: 和 data: 配对)
    local events=()
    local current_event=""
    local current_data=""
    while IFS= read -r line; do
        line=$(echo "$line" | sed 's/\r$//')
        if [[ "$line" == event:* ]]; then
            if [[ -n "$current_event" && -n "$current_data" ]]; then
                events+=("$current_event|$current_data")
            fi
            current_event="${line#event: }"
            current_data=""
        elif [[ "$line" == data:* ]]; then
            current_data="${line#data: }"
        fi
    done < "$response_file"
    
    # 添加最后一个事件
    if [[ -n "$current_event" && -n "$current_data" ]]; then
        events+=("$current_event|$current_data")
    fi
    
    if [[ ${#events[@]} -eq 0 ]]; then
        print_error "未找到任何事件,响应格式不正确"
        print_info "当前响应内容:"
        cat "$response_file"
        return 1
    fi
    
    print_info "共找到 ${#events[@]} 个事件"
    
    # 查找 content_block_start 事件且 content_block.type=tool_use 的位置
    local start_idx=-1
    for ((i=0; i<${#events[@]}; i++)); do
        local event_pair="${events[$i]}"
        local event_type="${event_pair%%|*}"
        local event_data="${event_pair#*|}"
        
        # 检查是否是 content_block_start 且 content_block.type=tool_use
        if [[ "$event_type" == "content_block_start" ]]; then
            if echo "$event_data" | grep -q '"type":"tool_use"' || echo "$event_data" | grep -q '"type": "tool_use"'; then
                start_idx=$i
                break
            fi
        fi
    done
    
    if [[ $start_idx -eq -1 ]]; then
        print_error "未找到 content_block_start 且 content_block.type=tool_use 的事件"
        print_info "应该是什么样:"
        print_info '  event: content_block_start'
        print_info '  data: {"type":"content_block_start","content_block":{"type":"tool_use",...},...}'
        print_info "当前响应事件列表:"
        for event_pair in "${events[@]}"; do
            local et="${event_pair%%|*}"
            print_info "  - $et"
        done
        return 1
    fi
    
    # 从 start_idx 开始到结尾的所有 chunks
    local check_chunks=()
    local tool_use_chunks=()
    
    for ((i=start_idx; i<${#events[@]}; i++)); do
        local event_pair="${events[$i]}"
        local event_data="${event_pair#*|}"
        # 将 event_type 也添加到 json 中
        local event_type="${event_pair%%|*}"
        local json_with_type="${event_data%\}*},\"_event_type\":\"$event_type\"}"
        check_chunks+=("$json_with_type")
        
        # 收集 tool_use 相关的 chunks
        if echo "$event_data" | grep -q '"type":"tool_use"' || echo "$event_data" | grep -q '"type": "tool_use"'; then
            tool_use_chunks+=("$json_with_type")
        fi
    done
    
    print_success "找到 content_block_start (tool_use) 起始位置: $start_idx"
    print_info "校验范围: 从位置 ${start_idx} 到结尾，共 ${#check_chunks[@]} 个数据块"
    
    # 展示校验范围内的所有 chunks
    print_title "校验范围内的响应内容（从 content_block_start 到响应结尾）"
    local idx=1
    for chunk in "${check_chunks[@]}"; do
        echo -e "${BLUE}[$idx]${NC}"
        format_json "$chunk"
        echo ""
        ((idx++))
    done
    
    # 检查规则 1: 首个 tool_use 应包含 name 和 id 字段
    print_title "检查规则 1: 首个 tool_use 应包含 name 和 id 字段"
    
    local first_tool_use="${tool_use_chunks[0]}"
    local has_name=0
    local has_id=0
    local name_value=""
    local id_value=""
    
    if echo "$first_tool_use" | grep -q '"name"'; then
        has_name=1
        name_value=$(echo "$first_tool_use" | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
    fi
    if echo "$first_tool_use" | grep -q '"id"'; then
        has_id=1
        id_value=$(echo "$first_tool_use" | grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
    fi
    
    if [[ $has_name -eq 1 && $has_id -eq 1 ]]; then
        print_success "首个 tool_use 包含 name ($name_value) 和 id ($id_value) 字段"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 首个 tool_use 应包含 name 和 id 字段"
        print_info "应该是什么样:"
        print_info '  {"content_block":{"type":"tool_use","id":"call_xxx","name":"list_dir",...}}'
        print_info "当前输出:"
        format_json "$first_tool_use"
        if [[ $has_name -eq 0 ]]; then
            print_info "  - 缺少: content_block.name 字段"
        fi
        if [[ $has_id -eq 0 ]]; then
            print_info "  - 缺少: content_block.id 字段"
        fi
        ((CHECK_FAILED++))
    fi
    
    # 检查规则 2: 检查中间数据块（非 tool_use 的数据块）
    print_title "检查规则 2: 检查中间数据块"
    
    local middle_chunks=()
    # 从 check_chunks 中提取非 tool_use 的数据块
    for chunk in "${check_chunks[@]}"; do
        if ! (echo "$chunk" | grep -q '"type":"tool_use"' || echo "$chunk" | grep -q '"type": "tool_use"'); then
            middle_chunks+=("$chunk")
        fi
    done
    
    if [[ ${#middle_chunks[@]} -gt 0 ]]; then
        print_success "找到 ${#middle_chunks[@]} 个中间数据块（非 tool_use）"
        ((CHECK_PASSED++))
        
        # 检查中间数据块是否包含 partial_json
        print_title "检查规则 3: 中间数据块应使用 partial_json 传递参数"
        
        local has_partial_json=0
        local partial_values=()
        
        for chunk in "${middle_chunks[@]}"; do
            if echo "$chunk" | grep -q '"partial_json"'; then
                has_partial_json=1
                local partial_value=$(echo "$chunk" | grep -o '"partial_json"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
                partial_values+=("$partial_value")
            fi
        done
        
        if [[ $has_partial_json -eq 1 ]]; then
            print_success "中间数据块使用 partial_json 传递参数"
            ((CHECK_PASSED++))
            
            # 显示参数构建过程
            print_info "参数逐步构建过程:"
            local concatenated=""
            for val in "${partial_values[@]}"; do
                concatenated="${concatenated}${val}"
                print_info "  + '$val' -> 当前: $concatenated"
            done
        else
            print_warn "中间数据块未使用 partial_json，可能使用其他格式传递参数"
        fi
    else
        print_warn "没有找到中间数据块"
    fi
    
    # 检查规则 4: 校验范围内的数据应能组成完整的参数
    print_title "检查规则 4: 校验 tool_use 参数传递的完整性"
    
    if [[ ${#tool_use_chunks[@]} -ge 1 ]]; then
        local last_tool_use="${tool_use_chunks[${#tool_use_chunks[@]}-1]}"
        if echo "$last_tool_use" | grep -q '"input"'; then
            local input_value=$(echo "$last_tool_use" | grep -o '"input"[[:space:]]*:[[:space:]]*[^,}]*' | head -1 | sed 's/.*:[[:space:]]*//')
            if [[ -n "$input_value" && "$input_value" != "null" ]]; then
                print_success "末个 tool_use 包含完整的 input: $input_value"
                ((CHECK_PASSED++))
            else
                print_warn "末个 tool_use 的 input 为空，可能参数是逐步传递的"
            fi
        else
            print_warn "末个 tool_use 未包含 input 字段，可能参数在后续响应中"
        fi
    fi
    if echo "$first_tool_use" | grep -q '"id"'; then
        has_id=1
        id_value=$(echo "$first_tool_use" | grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
    fi
    
    if [[ $has_name -eq 1 && $has_id -eq 1 ]]; then
        print_success "首个 tool_use 包含 name ($name_value) 和 id ($id_value) 字段"
        ((CHECK_PASSED++))
    else
        print_error "规则违反: 首个 tool_use 应包含 name 和 id 字段"
        print_info "应该是什么样:"
        print_info '  {"content_block":{"type":"tool_use","id":"call_xxx","name":"list_dir",...}}'
        print_info "当前输出:"
        format_json "$first_tool_use"
        if [[ $has_name -eq 0 ]]; then
            print_info "  - 缺少: content_block.name 字段"
        fi
        if [[ $has_id -eq 0 ]]; then
            print_info "  - 缺少: content_block.id 字段"
        fi
        ((CHECK_FAILED++))
    fi
    
    # 检查规则 2: 如果只有一个 tool_use，检查中间的 chunks
    if [[ ${#tool_use_chunks[@]} -eq 1 ]]; then
        print_title "检查规则 2: 找到 1 个 tool_use 块，检查其 input 字段"
        
        if echo "$first_tool_use" | grep -q '"input"'; then
            local input_value=$(echo "$first_tool_use" | grep -o '"input"[[:space:]]*:[[:space:]]*[^,}]*' | head -1 | sed 's/.*:[[:space:]]*//')
            print_success "tool_use 包含 input 字段: $input_value"
            ((CHECK_PASSED++))
        else
            print_warn "未找到 input 字段，这可能不是完整的 tool_use 数据"
        fi
    else
        # 多个 tool_use 块，检查中间的 chunks
        print_title "检查规则 2: 找到 ${#tool_use_chunks[@]} 个 tool_use 块，检查中间数据"
        print_info "首个 tool_use 在位置: ${tool_use_indices[0]}"
        print_info "末个 tool_use 在位置: ${tool_use_indices[-1]}"
        
        # 提取中间的 chunks（首个和末个之间的）
        local middle_chunks=()
        local first_idx=${tool_use_indices[0]}
        local last_idx=${tool_use_indices[-1]}
        
        for ((i=first_idx+1; i<last_idx; i++)); do
            middle_chunks+=("${data_lines[$i]}")
        done
        
        if [[ ${#middle_chunks[@]} -gt 0 ]]; then
            print_success "找到 ${#middle_chunks[@]} 个中间数据块"
            ((CHECK_PASSED++))
            
            # 检查中间数据块是否包含 partial_json 或其他有效数据
            print_title "检查规则 3: 中间数据块应包含参数信息"
            
            local has_partial_json=0
            local partial_values=()
            
            for chunk in "${middle_chunks[@]}"; do
                if echo "$chunk" | grep -q '"partial_json"'; then
                    has_partial_json=1
                    local partial_value=$(echo "$chunk" | grep -o '"partial_json"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
                    partial_values+=("$partial_value")
                fi
            done
            
            if [[ $has_partial_json -eq 1 ]]; then
                print_success "中间数据块使用 partial_json 传递参数"
                ((CHECK_PASSED++))
                
                # 显示参数构建过程
                print_info "参数逐步构建过程:"
                local concatenated=""
                for val in "${partial_values[@]}"; do
                    concatenated="${concatenated}${val}"
                    print_info "  + '$val' -> 当前: $concatenated"
                done
            else
                print_warn "中间数据块未使用 partial_json，可能使用其他格式传递参数"
            fi
        else
            print_warn "首个和末个 tool_use 之间没有中间数据块"
        fi
        
        # 检查末个 tool_use
        print_title "检查规则 4: 末个 tool_use 应包含完整的 input"
        
        local last_tool_use="${tool_use_chunks[${#tool_use_chunks[@]}-1]}"
        if echo "$last_tool_use" | grep -q '"input"'; then
            local input_value=$(echo "$last_tool_use" | grep -o '"input"[[:space:]]*:[[:space:]]*[^,}]*' | head -1 | sed 's/.*:[[:space:]]*//')
            if [[ -n "$input_value" && "$input_value" != "null" ]]; then
                print_success "末个 tool_use 包含完整的 input: $input_value"
                ((CHECK_PASSED++))
            else
                print_warn "末个 tool_use 的 input 为空"
            fi
        else
            print_warn "末个 tool_use 未包含 input 字段"
        fi
    fi
    
    # 检查 stop_reason 为 tool_use
    print_title "检查规则 5: 应包含 stop_reason 为 tool_use"
    
    local found_message_delta=0
    local found_stop_reason=0
    local stop_reason_value=""
    local message_delta_event=""
    
    for line in "${check_chunks[@]}"; do
        # 检查是否是 message_delta 事件 (通过 _event_type 或 type 字段)
        local is_message_delta=0
        if echo "$line" | grep -q '"_event_type":"message_delta"' || echo "$line" | grep -q '"type":"message_delta"'; then
            is_message_delta=1
        fi
        
        if [[ $is_message_delta -eq 1 ]]; then
            found_message_delta=1
            message_delta_event="$line"
            # 检查其中的 stop_reason
            local sr=$(echo "$line" | grep -o '"stop_reason"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
            if [[ -n "$sr" && "$sr" != "null" ]]; then
                found_stop_reason=1
                stop_reason_value="$sr"
                if [[ "$sr" == "tool_use" ]]; then
                    break
                fi
            fi
        fi
        
        # 也检查其他可能的 stop_reason 位置
        if echo "$line" | grep -q '"stop_reason"'; then
            local sr=$(echo "$line" | grep -o '"stop_reason"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:"\(.*\)".*/\1/')
            if [[ -n "$sr" && "$sr" != "null" ]]; then
                found_stop_reason=1
                stop_reason_value="$sr"
                if [[ "$sr" == "tool_use" ]]; then
                    break
                fi
            fi
        fi
    done
    
    if [[ $found_message_delta -eq 1 && $found_stop_reason -eq 1 && "$stop_reason_value" == "tool_use" ]]; then
        print_success "找到 message_delta 事件包含 stop_reason: tool_use"
        ((CHECK_PASSED++))
    elif [[ $found_stop_reason -eq 1 && "$stop_reason_value" == "tool_use" ]]; then
        print_success "找到 stop_reason: tool_use"
        ((CHECK_PASSED++))
    elif [[ $found_message_delta -eq 1 && $found_stop_reason -eq 1 ]]; then
        print_error "规则违反: stop_reason 应为 tool_use"
        print_info "应该是什么样:"
        print_info '  event: message_delta'
        print_info '  data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},...}'
        print_info "当前输出:"
        format_json "$message_delta_event"
        ((CHECK_FAILED++))
    elif [[ $found_stop_reason -eq 1 ]]; then
        print_error "规则违反: stop_reason 应为 tool_use"
        print_info "应该是什么样:"
        print_info '  stop_reason: "tool_use"'
        print_info "当前输出:"
        print_info "  stop_reason: $stop_reason_value"
        ((CHECK_FAILED++))
    else
        print_error "规则违反: 未找到 stop_reason 字段"
        print_info "应该是什么样:"
        print_info '  event: message_delta'
        print_info '  data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},...}'
        ((CHECK_FAILED++))
    fi
    
    return 0
}

# 打印检查结果摘要
print_summary() {
    print_title "检查结果摘要"
    
    print_info "通过: $CHECK_PASSED"
    print_info "失败: $CHECK_FAILED"
    
    if [[ $CHECK_FAILED -eq 0 ]]; then
        echo ""
        print_success "======================================"
        print_success "  所有检查项通过! 模型格式符合要求。"
        print_success "======================================"
        return 0
    else
        echo ""
        print_error "======================================"
        print_error "  部分检查项未通过,请检查模型配置。"
        print_error "======================================"
        print_info ""
        print_info "提示: 请确保模型部署时启用了以下参数:"
        print_info "  --enable-auto-tool-choice"
        print_info "  --tool-call-parser"
        return 1
    fi
}

# 打印原始响应
print_raw_response() {
    local response_file="$1"
    
    print_title "原始响应内容"
    cat "$response_file"
    echo ""
}

# 执行单次检查
run_single_check() {
    local url="$1"
    local format="$2"
    
    MODEL_URL="$url"
    FORMAT_TYPE="$format"
    
    print_title "开始检查 $FORMAT_TYPE 格式"
    print_info "请求地址: $MODEL_URL"
    
    # 创建临时文件
    local response_file=$(mktemp)
    
    # 发送请求
    if ! send_request "$response_file" 2>/dev/null; then
        print_error "请求失败: $MODEL_URL"
        rm -f "$response_file"
        return 1
    fi
    
    # 打印原始响应
    print_raw_response "$response_file"
    
    # 根据格式类型进行检查
    if [[ "$FORMAT_TYPE" == "openai" ]]; then
        check_openai_response "$response_file"
    else
        check_anthropic_response "$response_file"
    fi
    
    local result=$?
    rm -f "$response_file"
    return $result
}

# 主函数
main() {
    # 检查依赖
    check_dependencies
    
    # 如果没有参数,交互式询问
    if [[ $# -eq 0 ]]; then
        echo "请输入模型服务地址 (IP:端口 或完整URL):"
        echo "  示例: 10.174.145.94:8366"
        echo "  示例: http://localhost:8806"
        read -r SERVER_ADDR
        
        # 自动补充协议头
        if [[ ! "$SERVER_ADDR" =~ ^http://.*$ ]] && [[ ! "$SERVER_ADDR" =~ ^https://.*$ ]]; then
            SERVER_ADDR="http://$SERVER_ADDR"
        fi
        
        echo "请输入 API Key (直接回车使用默认: EMPTY):"
        read -r API_KEY
        API_KEY=${API_KEY:-EMPTY}
        
        echo "请输入模型名称 (必填):"
        read -r MODEL_NAME
        
        if [[ -z "$MODEL_NAME" ]]; then
            print_error "模型名称为必填项"
            exit 1
        fi
        
        # 检查依赖
        check_dependencies
        
        # 分别测试两种格式
        local openai_url="${SERVER_ADDR}/v1/chat/completions"
        local anthropic_url="${SERVER_ADDR}/v1/messages"
        
        # 重置计数器
        TOTAL_PASSED=0
        TOTAL_FAILED=0
        
        # 检查 OpenAI 格式
        echo ""
        run_single_check "$openai_url" "openai"
        local openai_result=$?
        TOTAL_PASSED=$((TOTAL_PASSED + CHECK_PASSED))
        TOTAL_FAILED=$((TOTAL_FAILED + CHECK_FAILED))
        
        # 重置单次检查的计数器
        CHECK_PASSED=0
        CHECK_FAILED=0
        
        # 检查 Anthropic 格式
        echo ""
        run_single_check "$anthropic_url" "anthropic"
        local anthropic_result=$?
        TOTAL_PASSED=$((TOTAL_PASSED + CHECK_PASSED))
        TOTAL_FAILED=$((TOTAL_FAILED + CHECK_FAILED))
        
        # 打印最终摘要和推荐
        print_title "最终检查结果摘要"
        print_info "OpenAI 格式: $([[ $openai_result -eq 0 ]] && echo '[PASS] 通过' || echo '[FAIL] 未通过')"
        print_info "Anthropic 格式: $([[ $anthropic_result -eq 0 ]] && echo '[PASS] 通过' || echo '[FAIL] 未通过')"
        echo ""
        
        # 输出推荐建议
        if [[ $openai_result -eq 0 && $anthropic_result -eq 0 ]]; then
            print_success "======================================"
            print_success "  两种格式都满足要求!"
            print_success ""
            print_success "  推荐: 优先使用 Anthropic 格式"
            print_success "  地址: $anthropic_url"
            print_success "======================================"
            exit 0
        elif [[ $anthropic_result -eq 0 ]]; then
            print_success "======================================"
            print_success "  Anthropic 格式满足要求"
            print_success ""
            print_success "  推荐: 使用 Anthropic 格式"
            print_success "  地址: $anthropic_url"
            print_success "======================================"
            echo ""
            print_warn "OpenAI 格式未通过，如需使用请检查配置"
            exit 0
        elif [[ $openai_result -eq 0 ]]; then
            print_success "======================================"
            print_success "  OpenAI 格式满足要求"
            print_success ""
            print_success "  推荐: 使用 OpenAI 格式"
            print_success "  地址: $openai_url"
            print_success "======================================"
            echo ""
            print_warn "Anthropic 格式未通过，如需使用请检查配置"
            exit 0
        else
            print_error "======================================"
            print_error "  两种格式都不满足要求"
            print_error "======================================"
            echo ""
            print_info "修改建议:"
            print_info "1. 确保模型支持 Function Calling 功能"
            print_info "2. 检查模型部署参数:"
            print_info "   --enable-auto-tool-choice"
            print_info "   --tool-call-parser"
            print_info "3. 确认模型名称正确"
            print_info "4. 查看具体错误信息，检查响应格式"
            exit 1
        fi
    else
        parse_args "$@"
        
        # 检查依赖
        check_dependencies
        
        # 打印请求信息
        print_request_info
        
        # 创建临时文件
        RESPONSE_FILE=$(mktemp)
        trap "rm -f $RESPONSE_FILE" EXIT
        
        # 发送请求
        send_request "$RESPONSE_FILE"
        
        # 打印原始响应
        print_raw_response "$RESPONSE_FILE"
        
        # 根据格式类型进行检查
        if [[ "$FORMAT_TYPE" == "openai" ]]; then
            check_openai_response "$RESPONSE_FILE"
        else
            check_anthropic_response "$RESPONSE_FILE"
        fi
        
        local check_result=$?
        
        # 打印摘要
        print_summary
        
        exit $((CHECK_FAILED > 0 ? 1 : 0))
    fi
}

# 运行主函数
main "$@"

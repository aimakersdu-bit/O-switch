#!/bin/bash
# ==============================================================================
# baixin-switch 多架构打包构建脚本
# ==============================================================================
set -e

# 获取版本号参数，默认 v1.0.0
VERSION=${1:-"v1.0.0"}
DIST_DIR="dist"

echo "=================================================="
echo " 开始打包编译 baixin-switch, 版本号: ${VERSION}"
echo "=================================================="

# 清理并创建发布目录
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# 定义目标操作系统与处理器架构
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

# 确保在仓库根目录
cd "$(dirname "$0")/.."

for PLATFORM in "${PLATFORMS[@]}"; do
    # 拆分 OS 和 ARCH
    IFS="/" read -r -a parts <<< "$PLATFORM"
    GOOS="${parts[0]}"
    GOARCH="${parts[1]}"
    
    OUTPUT_NAME="baixin-switch"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi
    
    # 临时构建文件夹名称
    ARCHIVE_DIR="baixin-switch-${VERSION}-${GOOS}-${GOARCH}"
    TEMP_BUILD_DIR="${DIST_DIR}/${ARCHIVE_DIR}"
    mkdir -p "${TEMP_BUILD_DIR}"
    
    echo ">> 编译目标平台: ${GOOS}/${GOARCH}..."
    
    # 交叉编译 Go 二进制，注入 Version
    GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags "-X main.Version=${VERSION} -s -w" \
        -o "${TEMP_BUILD_DIR}/${OUTPUT_NAME}" \
        cmd/baixin-switch/main.go
    
    # 复制部署说明和配置文件模版到包内
    cp README.md "${TEMP_BUILD_DIR}/" 2>/dev/null || true
    cp docs/private-deployment-guide.md "${TEMP_BUILD_DIR}/" 2>/dev/null || true
    cp .env.example "${TEMP_BUILD_DIR}/.env.example" 2>/dev/null || true
    
    # 打包输出
    cd "${DIST_DIR}"
    if [ "$GOOS" = "windows" ]; then
        # Windows 打包为 zip
        zip -q -r "${ARCHIVE_DIR}.zip" "${ARCHIVE_DIR}"
        echo "   [OK] 生成 ${DIST_DIR}/${ARCHIVE_DIR}.zip"
    else
        # Linux/macOS 打包为 tar.gz
        tar -czf "${ARCHIVE_DIR}.tar.gz" "${ARCHIVE_DIR}"
        echo "   [OK] 生成 ${DIST_DIR}/${ARCHIVE_DIR}.tar.gz"
    fi
    
    # 清理临时编译目录
    rm -rf "${ARCHIVE_DIR}"
    cd ..
done

echo "=================================================="
echo " 编译完成！打包文件已输出到目录: ${DIST_DIR}/"
ls -lh "${DIST_DIR}"
echo "=================================================="

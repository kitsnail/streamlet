#!/bin/bash
# Streamlet 部署脚本

set -e

# 配置
VIDEO_DIR="${VIDEO_DIR:-/data/videos}"
INSTALL_DIR="/opt/streamlet"
BIN_NAME="streamlet"
SERVICE_NAME="streamlet"

echo "🚀 开始部署 Streamlet..."

# 检查 root
if [ "$EUID" -ne 0 ]; then
    echo "请使用 sudo 运行此脚本"
    exit 1
fi

# 创建目录
echo "📁 创建目录..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$VIDEO_DIR"

# 复制二进制
echo "📦 安装程序..."
cp "$BIN_NAME" "$INSTALL_DIR/"
chmod +x "$INSTALL_DIR/$BIN_NAME"

# 复制静态文件
cp -r static "$INSTALL_DIR/"

# 安装 systemd 服务
echo "⚙️ 安装 systemd 服务..."
cp deploy/streamlet.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo "✅ 部署完成!"
echo ""
echo "下一步:"
echo "1. 编辑 /etc/systemd/system/streamlet.service 配置环境变量"
echo "2. 将视频文件放到 $VIDEO_DIR 目录"
echo "3. 启动服务: systemctl start streamlet"
echo "4. 访问: http://localhost:8080"

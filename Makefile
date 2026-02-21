.PHONY: all build build-linux build-darwin build-windows clean test run package help

# 变量
BINARY_NAME=streamlet
VERSION?=1.0.0
BUILD_DIR=build
DIST_DIR=dist
MAIN_PATH=./main.go

# Go 参数
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# 构建参数
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"
BUILD_TIME=$(shell date +%Y-%m-%d_%H:%M:%S)

# 默认目标
all: clean deps build

# 下载依赖
deps:
	@echo "📦 下载依赖..."
	$(GOMOD) download
	$(GOMOD) tidy

# 本地构建 (当前平台)
build:
	@echo "🔨 构建项目 ($(shell go env GOOS)/$(shell go env GOARCH))..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(BINARY_NAME)"

# Linux 构建 (Ubuntu 部署)
build-linux:
	@echo "🐧 构建 Linux amd64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

# Linux ARM64 构建
build-linux-arm64:
	@echo "🐧 构建 Linux arm64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

# macOS 构建
build-darwin:
	@echo "🍎 构建 macOS arm64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64"

# Windows 构建
build-windows:
	@echo "🪟 构建 Windows..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "✅ 构建完成: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

# 构建所有平台
build-all: build-linux build-darwin build-windows
	@echo "✅ 所有平台构建完成"

# 测试
test:
	@echo "🧪 运行测试..."
	$(GOTEST) -v ./...

# macOS 本地测试运行
run:
	@echo "🚀 本地测试运行..."
	@echo "📁 视频目录: ./videos (创建测试视频目录)"
	@mkdir -p ./videos
	VIDEO_DIR=./videos AUTH_USER=admin AUTH_PASS=admin123 JWT_SECRET=local-test-secret PORT=8080 ENV=development $(BUILD_DIR)/$(BINARY_NAME)

# 开发模式 (热重载需要 air)
dev:
	@which air > /dev/null || (echo "安装 air..." && go install github.com/air-verse/air@latest)
	@echo "🔥 开发模式 (热重载)..."
	VIDEO_DIR=./videos AUTH_USER=admin AUTH_PASS=admin123 JWT_SECRET=dev-secret PORT=8080 air

# 清理
clean:
	@echo "🧹 清理构建产物..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(DIST_DIR)
	$(GOCLEAN)

# 打包 (Linux 部署包)
package: build-linux
	@echo "📦 打包发布版本..."
	@mkdir -p $(DIST_DIR)
	@mkdir -p $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64
	
	# 复制文件
	cp $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64/streamlet
	cp -r static $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64/
	cp -r deploy $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64/
	cp README.md $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64/
	
	# 打包
	cd $(DIST_DIR) && tar -czvf streamlet-$(VERSION)-linux-amd64.tar.gz streamlet-$(VERSION)-linux-amd64
	@rm -rf $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64
	
	@echo "✅ 打包完成: $(DIST_DIR)/streamlet-$(VERSION)-linux-amd64.tar.gz"

# 打包 macOS 版本
package-darwin: build-darwin
	@echo "📦 打包 macOS 版本..."
	@mkdir -p $(DIST_DIR)
	@mkdir -p $(DIST_DIR)/streamlet-$(VERSION)-darwin-arm64
	
	cp $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(DIST_DIR)/streamlet-$(VERSION)-darwin-arm64/streamlet
	cp -r static $(DIST_DIR)/streamlet-$(VERSION)-darwin-arm64/
	cp README.md $(DIST_DIR)/streamlet-$(VERSION)-darwin-arm64/
	
	cd $(DIST_DIR) && tar -czvf streamlet-$(VERSION)-darwin-arm64.tar.gz streamlet-$(VERSION)-darwin-arm64
	@rm -rf $(DIST_DIR)/streamlet-$(VERSION)-darwin-arm64
	
	@echo "✅ 打包完成: $(DIST_DIR)/streamlet-$(VERSION)-darwin-arm64.tar.gz"

# 安装到本地 (macOS)
install: build
	@echo "📥 安装到 /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "✅ 安装完成"

# 显示帮助
help:
	@echo "Streamlet Makefile 使用说明"
	@echo ""
	@echo "构建命令:"
	@echo "  make build           - 构建当前平台版本"
	@echo "  make build-linux     - 构建 Linux amd64 版本"
	@echo "  make build-linux-arm64 - 构建 Linux arm64 版本"
	@echo "  make build-darwin    - 构建 macOS arm64 版本"
	@echo "  make build-windows   - 构建 Windows 版本"
	@echo "  make build-all       - 构建所有平台版本"
	@echo ""
	@echo "测试命令:"
	@echo "  make test            - 运行测试"
	@echo "  make run             - 本地测试运行"
	@echo "  make dev             - 开发模式 (需要 air)"
	@echo ""
	@echo "打包命令:"
	@echo "  make package         - 打包 Linux 发布版本"
	@echo "  make package-darwin  - 打包 macOS 发布版本"
	@echo ""
	@echo "其他命令:"
	@echo "  make deps            - 下载依赖"
	@echo "  make clean           - 清理构建产物"
	@echo "  make install         - 安装到 /usr/local/bin (macOS)"
	@echo ""
	@echo "自定义版本:"
	@echo "  make package VERSION=1.2.0"

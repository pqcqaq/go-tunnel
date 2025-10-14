.PHONY: all build clean run-server run-client test install-deps

# 默认目标
all: build

# 安装依赖
install-deps:
	@echo "安装依赖..."
	go mod download
	go mod tidy

# 编译所有程序
build: build-server build-client

# 编译服务器
build-server:
	@echo "编译服务器..."
	go build -o bin/server ./server
	@echo "服务器编译完成: bin/server"

# 编译客户端
build-client:
	@echo "编译客户端..."
	go build -o bin/client ./client
	@echo "客户端编译完成: bin/client"

# 运行服务器
run-server: build-server
	@echo "启动服务器..."
	./bin/server -config config.yaml

# 运行客户端
run-client: build-client
	@echo "启动客户端..."
	./bin/client -server localhost:9000

# 清理编译文件
clean:
	@echo "清理编译文件..."
	rm -rf bin/
	rm -rf data/
	@echo "清理完成"

# 运行测试
test:
	@echo "运行测试..."
	go test -v ./...

# 创建必要的目录
init:
	@echo "初始化项目目录..."
	mkdir -p data
	mkdir -p bin
	@echo "目录创建完成"

# 交叉编译 Linux
build-linux:
	@echo "交叉编译 Linux 版本..."
	GOOS=linux GOARCH=amd64 go build -o bin/server-linux ./server
	GOOS=linux GOARCH=amd64 go build -o bin/client-linux ./client
	@echo "Linux 版本编译完成"

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...

# 代码检查
lint:
	@echo "代码检查..."
	go vet ./...

# 帮助信息
help:
	@echo "可用的 make 命令："
	@echo "  make install-deps  - 安装依赖"
	@echo "  make build         - 编译所有程序"
	@echo "  make build-server  - 编译服务器"
	@echo "  make build-client  - 编译客户端"
	@echo "  make run-server    - 运行服务器"
	@echo "  make run-client    - 运行客户端"
	@echo "  make clean         - 清理编译文件"
	@echo "  make test          - 运行测试"
	@echo "  make init          - 初始化项目目录"
	@echo "  make build-linux   - 交叉编译 Linux 版本"
	@echo "  make fmt           - 格式化代码"
	@echo "  make lint          - 代码检查"
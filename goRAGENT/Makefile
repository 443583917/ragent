.PHONY: build run test lint clean dev gen docker

APP_NAME    := ragent-server
MCP_NAME    := ragent-mcp-server
BUILD_DIR   := build
MAIN_PATH   := ./cmd/server
MCP_PATH    := ./cmd/mcp-server

# 编译
docker-up:
	docker compose -f docker/docker-compose.yml --project-name ragent up -d

docker-down:
	docker compose -f docker/docker-compose.yml --project-name ragent down

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)

build-mcp:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(MCP_NAME) $(MCP_PATH)

build-all: build build-mcp

# 运行
run:
	go run $(MAIN_PATH)/...

run-mcp:
	go run $(MCP_PATH)/...

# 开发模式 — 一键启动全部（基础设施 + Go热更新 + 提示打开前端）
dev-all: docker-up
	@echo "后端: http://localhost:9090"
	@echo "前端: cd ../frontend && npm run dev"
	@echo "Attu: http://localhost:8083"
	air -c .air.toml

# Go 热更新（需要 air 工具: go install github.com/air-verse/air@latest）
dev:
	air -c .air.toml

# 测试
test:
	go test ./... -count=1 -timeout 60s

test-race:
	go test -race ./... -timeout 120s

test-cover:
	go test ./... -coverprofile=cover.out && go tool cover -html=cover.out

# 代码检查
lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	goimports -w .

vet:
	go vet ./...

# 清理
clean:
	rm -rf $(BUILD_DIR)
	rm -f cover.out

# 代码生成
gen:
	go generate ./...

# Docker
docker:
	docker build -t ragent-go:latest .

# 依赖
tidy:
	go mod tidy

verify:
	go mod verify

# 性能剖析
pprof-cpu:
	go tool pprof http://localhost:9090/debug/pprof/profile

pprof-heap:
	go tool pprof http://localhost:9090/debug/pprof/heap

# 验收
check-phase1:
	@echo "=== Phase 1 验收检查 ==="
	@echo "1. 编译检查..."
	go build ./...
	@echo "   ✓ 编译通过"
	@echo "2. 启动检查..."
	@echo "   (请手动启动: go run cmd/server/main.go)"
	@echo "3. Health 检查..."
	@echo "   curl http://localhost:9090/api/ragent/health"
	@echo "4. SSE 格式检查..."
	@echo "   curl -H 'Authorization: Bearer <token>' 'http://localhost:9090/api/ragent/rag/v3/chat?question=test'"

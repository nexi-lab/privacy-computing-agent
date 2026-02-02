# Agent Proxy

多用户隐私计算 Agent 的代理转发服务，负责用户认证、请求转发和身份识别。

## 功能概述

Agent Proxy 是系统的入口层，提供以下核心功能：

1. **用户注册管理**: 维护用户 ID 与 Nexus API Key 的映射关系
2. **请求转发**: 将用户请求转发到后端的 Privacy Computing Agent
3. **认证信息注入**: 自动在请求中注入用户的认证信息
4. **跨用户协作**: 支持用户 A 以用户 B 的身份发起请求（用于多方协作）

## 架构设计

```
用户请求 → Agent Proxy → Privacy Computing Agent
           ↓
    用户映射管理 (mappings.json)
    服务配置 (config.json)
```

## API 接口

### 1. 用户注册

**端点**: `POST /register`

**功能**: 注册用户 ID 与 Nexus API Key 的映射关系

**请求示例**:
```bash
curl -X POST http://localhost:2024/register \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "alice",
    "nexus_key": "sk-alice-api-key"
  }'
```

**响应**:
```json
{
  "ok": true
}
```

**说明**:
- `user_id`: 用户唯一标识符
- `nexus_key`: 用户的 Nexus API Key
- 映射关系会持久化到 `config/mappings.json` 文件

### 2. 通用代理转发

**端点**: `/* (所有其他路径)`

**功能**: 转发请求到 Privacy Computing Agent，并自动处理认证

**请求示例**:
```bash
curl -X POST http://localhost:2024/runs/stream \
  -H "Content-Type: application/json" \
  -d '{
    "assistant_id": "agent",
    "input": {
      "messages": [{"role": "user", "content": "查找 Python 文件"}]
    },
    "metadata": {
      "x_auth": "Bearer sk-alice-api-key",
      "user_id": "alice",
      "target_user_id": "bob",
      "nexus_server_url": "http://nexus:8080"
    }
  }'
```

**Metadata 字段说明**:
- `x_auth`: 当前用户的 Nexus API Key（Bearer token 格式）
- `user_id`: 当前用户 ID
- `target_user_id`: 目标用户 ID（可选，用于跨用户协作）
- `nexus_server_url`: Nexus 服务器地址

**处理流程**:

1. **自动注册**: 如果 `user_id` 不存在于映射表中，自动注册
2. **身份识别**: 从 metadata 中提取 `user_id` 和 `target_user_id`
3. **认证替换**: 如果指定了 `target_user_id`，将 `x_auth` 替换为目标用户的 API Key
4. **请求转发**: 将修改后的请求转发到 Privacy Computing Agent

## 配置文件

### config/config.json

服务配置文件，定义后端服务地址：

```json
{
  "privacy_agent_url": "http://tsql:8123",
  "nexus_server_url": "http://nexus-server:8080"
}
```

**字段说明**:
- `privacy_agent_url`: Privacy Computing Agent 的服务地址
- `nexus_server_url`: Nexus 文件系统服务器地址

### config/mappings.json

用户映射配置文件，存储用户 ID 与 API Key 的映射：

```json
{
  "user_to_agent_key": {
    "alice": "sk-alice-api-key",
    "bob": "sk-bob-api-key"
  }
}
```

**说明**:
- 文件会在首次注册用户时自动创建
- 每次注册或更新都会自动保存
- 支持并发读写（使用 RWMutex 保护）

## 核心代码说明

### 主要数据结构

```go
// 服务配置
type Config struct {
    PrivacyAgentURL string `json:"privacy_agent_url"`
    NexusServerURL  string `json:"nexus_server_url"`
}

// 用户映射
type Mappings struct {
    mu             sync.RWMutex
    UserToNexusKey map[string]string `json:"user_to_agent_key"`
}

// 请求元数据
type Metadata struct {
    UserID       string `json:"user_id"`
    XAuth        string `json:"x_auth"`
    TargetUserID string `json:"target_user_id"`
}
```

### 关键函数

#### 1. registerHandler

处理用户注册请求：
- 验证请求参数
- 更新内存映射
- 持久化到文件

#### 2. GenericProxyHandler

通用代理处理器：
- 提取请求 metadata
- 自动注册新用户
- 替换目标用户认证信息
- 转发请求到后端

#### 3. modifyMetadata

修改请求 metadata：
- 替换 `x_auth` 为目标用户的 API Key
- 更新 `user_id` 为目标用户 ID
- 注入 `nexus_server_url`

## 本地开发

### 前置要求

- Go 1.21+
- 配置文件目录: `config/`

### 运行步骤

1. **安装依赖**:
```bash
go mod download
```

2. **创建配置文件**:
```bash
mkdir -p config
cat > config/config.json <<EOF
{
  "privacy_agent_url": "http://localhost:8123",
  "nexus_server_url": "http://localhost:8080"
}
EOF
```

3. **运行服务**:
```bash
go run main.go
```

服务将在 `http://localhost:2024` 启动。

### 测试

```bash
# 注册用户
curl -X POST http://localhost:2024/register \
  -H "Content-Type: application/json" \
  -d '{"user_id": "test_user", "nexus_key": "sk-test-key"}'

# 查看映射文件
cat config/mappings.json
```

## Docker 部署

### 构建镜像

```bash
docker build -t agent_proxy:latest .
```

### 运行容器

```bash
docker run -d \
  --name agent_proxy \
  -p 2024:2024 \
  -v $(pwd)/config:/app/config \
  agent_proxy:latest
```

### Dockerfile 说明

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o agent_proxy .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/agent_proxy .
EXPOSE 2024
CMD ["./agent_proxy"]
```

## 日志

日志输出到 `agent_proxy.log` 文件，包含以下信息：

- 用户注册事件
- 请求转发详情
- 认证信息替换
- 错误和警告

**日志级别**: DEBUG

**日志格式**:
```
time="2024-01-01T12:00:00Z" level=info msg="loaded mappings from config/mappings.json"
time="2024-01-01T12:00:01Z" level=info msg="proxy target: http://tsql:8123"
time="2024-01-01T12:00:02Z" level=debug msg="extractMetadata:UserID[alice] TargetUserID[bob] XAuth[Bearer sk-xxx]"
```

## 安全考虑

1. **API Key 保护**:
   - API Key 存储在文件系统中，需要适当的文件权限保护
   - 建议使用环境变量或密钥管理服务

2. **请求验证**:
   - 验证 metadata 中的必需字段
   - 检查用户是否已注册

3. **跨用户访问**:
   - 当前实现允许任何用户以其他用户身份发起请求
   - 生产环境应添加权限验证机制

## 故障排查

### 问题 1: 无法连接到后端服务

**症状**: 请求返回 502 Bad Gateway

**解决方案**:
- 检查 `config/config.json` 中的 `privacy_agent_url`
- 确认后端服务是否运行
- 检查 Docker 网络连通性

### 问题 2: 用户未注册

**症状**: 返回 "no agent key for user" 错误

**解决方案**:
- 先调用 `/register` 接口注册用户
- 或在请求中提供 `x_auth` 和 `user_id`，系统会自动注册

### 问题 3: 映射文件损坏

**症状**: 启动时报错 "failed to load mappings"

**解决方案**:
```bash
# 备份现有文件
mv config/mappings.json config/mappings.json.bak

# 创建新的空映射文件
echo '{"user_to_agent_key":{}}' > config/mappings.json

# 重启服务
```

## 性能优化

1. **并发处理**: 使用 RWMutex 支持高并发读取
2. **连接复用**: HTTP 客户端自动复用连接
3. **超时配置**: 可根据需要调整 ReadTimeout/WriteTimeout

## 未来改进

- [ ] 添加用户权限验证
- [ ] 支持 API Key 加密存储
- [ ] 添加请求限流
- [ ] 支持多后端负载均衡
- [ ] 添加监控指标（Prometheus）
- [ ] 支持 HTTPS/TLS

# 数牍隐私计算 AI Agent

基于 LangGraph 和 SecretFlow SCQL 的隐私计算智能代理系统，支持多用户协作完成联邦 SQL 查询和隐私求交任务。

## 项目简介

本项目是一个完整的隐私计算解决方案，通过 AI Agent 自动协调多方参与者完成隐私计算任务。系统采用微服务架构，包含代理转发、AI 智能协调和隐私计算引擎三大核心组件。

### 核心特性

- **智能任务协调**: AI Agent 自动识别角色（发起方/协作方），协调完整的隐私计算流程
- **多用户支持**: 通过代理层实现用户认证、请求转发和身份识别
- **联邦 SQL**: 基于 SecretFlow SCQL 实现安全的多方联合查询
- **隐私求交**: 支持 PSI（Private Set Intersection）等隐私计算协议
- **文件系统集成**: 与 Nexus 文件系统深度集成，支持数据集管理
- **容器化部署**: 完整的 Docker 容器化方案，支持一键部署

## 系统架构

```
┌─────────────┐
│   用户 A    │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────┐
│      Agent Proxy (Port 7000)        │
│  - 用户认证与注册                    │
│  - 请求转发                          │
│  - Metadata 注入                     │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│   Privacy Computing Agent (LangGraph)│
│  - ReAct Agent 智能协调              │
│  - Nexus 文件系统工具                │
│  - Docker 容器管理                   │
│  - 多方协作编排                      │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  Privacy Computing Engine (Port 8000)│
│  - 基于 SCQL 的隐私计算              │
│  - 联邦 SQL 执行                     │
│  - 加密协议实现                      │
└─────────────────────────────────────┘
```

## 项目结构

```
.
├── agent_proxy/          # 代理转发服务
│   ├── main.go          # 主程序入口
│   ├── Dockerfile       # 容器构建文件
│   └── go.mod           # Go 依赖管理
│
├── langgraph/           # AI Agent 服务
│   ├── react_agent.py   # ReAct Agent 实现
│   ├── nexus_tools.py   # Nexus 文件系统工具
│   ├── docker_tools.py  # Docker 容器管理工具
│   ├── Dockerfile       # 容器构建文件
│   └── pyproject.toml   # Python 依赖管理
│
├── privacy_computing/   # 隐私计算引擎
│   ├── main.go          # 主程序入口
│   ├── task.go          # 任务管理
│   ├── broker_util.go   # Broker 工具
│   └── nexus_client.go  # Nexus 客户端
│
├── deploy/              # 部署配置
│   ├── docker-compose.yml  # Docker Compose 配置
│   ├── .env             # 环境变量配置
│   ├── .env.example     # 环境变量示例
│   └── config/          # 配置文件目录
│       ├── config.json  # 服务配置
│       └── mappings.json # 用户映射配置
│
├── broker               # SCQL Broker 二进制文件
├── scqlengine          # SCQL Engine 二进制文件
├── init.sql            # 数据库初始化脚本
├── config.yml          # Broker 配置文件
└── supervisord.conf    # Supervisor 配置
```

## 快速开始

### 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- Go 1.21+ (开发环境)
- Python 3.11+ (开发环境)

### 部署步骤

1. **克隆项目**

```bash
git clone <repository-url>
cd privacy-computing-agent
```

2. **配置环境变量**

```bash
cd deploy
cp .env.example .env
# 编辑 .env 文件，配置 OpenAI API 密钥等参数
```

3. **构建镜像**

```bash
# 构建 agent_proxy 镜像
cd agent_proxy
docker build -t agent_proxy:latest .

# 构建 langgraph agent 镜像
cd ../langgraph
docker build -t tsql-langgraph:latest .
```

4. **启动服务**

```bash
cd ../deploy
docker-compose up -d
```

5. **验证服务**

```bash
# 检查服务状态
docker-compose ps

# 查看日志
docker-compose logs -f
```

### 服务端口

- **Agent Proxy**: 7000 (可通过 AGENT_PROXY_PORT 配置)
- **Privacy Computing Engine**: 8000
- **SCQL Broker**: 8080
- **SCQL Engine**: 8003

## 使用指南

### 用户注册

首次使用需要注册用户和 Nexus API Key 的映射关系：

```bash
curl -X POST http://localhost:7000/register \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_alice",
    "nexus_key": "sk-your-nexus-api-key"
  }'
```

### 发起隐私计算任务

通过 Agent Proxy 发送请求到 AI Agent：

```bash
curl -X POST http://localhost:7000/runs/stream \
  -H "Content-Type: application/json" \
  -d '{
    "assistant_id": "agent",
    "input": {
      "messages": [
        {
          "role": "user",
          "content": "帮我和 user_bob 完成隐私求交，我的数据集是 /workspace/alice/data.csv，求交列是 id"
        }
      ]
    },
    "metadata": {
      "x_auth": "Bearer sk-your-nexus-api-key",
      "user_id": "user_alice",
      "target_user_id": "user_bob",
      "nexus_server_url": "http://nexus-server:8080"
    }
  }'
```

### 工作流程

AI Agent 会自动协调以下流程：

1. **数据集定位**: 在双方的 Nexus 文件系统中定位 CSV 数据集
2. **容器创建**: 为双方创建隐私计算容器
3. **公钥交换**: 自动交换加密公钥
4. **SQL 生成**: 根据数据集表头自动生成联邦 SQL
5. **任务配置**: 生成双方的 Task JSON 配置
6. **用户确认**: 向用户展示配置并等待确认
7. **执行计算**: 启动隐私计算任务
8. **结果获取**: 等待计算完成并获取结果文件路径

## 组件详解

### Agent Proxy

代理转发服务，负责：
- 用户注册和 API Key 映射管理
- 请求转发到 AI Agent
- 自动注入用户认证信息
- 支持跨用户请求转发

详见: [agent_proxy/README.md](agent_proxy/README.md)

### Privacy Computing Agent

基于 LangGraph 的 AI Agent，负责：
- 智能任务协调和流程编排
- 角色识别（发起方/协作方）
- 文件系统操作（通过 Nexus Tools）
- 容器生命周期管理
- 多方通信协调

详见: [langgraph/README.md](langgraph/README.md)

### Privacy Computing Engine

隐私计算引擎，负责：
- 联邦 SQL 执行
- 隐私求交计算
- 加密协议实现
- 与 SCQL Broker/Engine 交互

详见: [privacy_computing/README.md](privacy_computing/README.md)

## 开发指南

### 本地开发

1. **Agent Proxy 开发**

```bash
cd agent_proxy
go mod download
go run main.go
```

2. **AI Agent 开发**

```bash
cd langgraph
pip install -e .
export OPENAI_API_KEY=your-key
export OPENAI_API_BASE=https://api.openai.com/v1
export OPENAI_MODEL=gpt-4
python react_agent.py
```

3. **Privacy Computing Engine 开发**

```bash
cd privacy_computing
go mod download
go run .
```

### 配置说明

#### deploy/.env

```bash
# OpenAI API 配置
OPENAI_API_KEY=your-api-key
OPENAI_API_BASE=https://api.openai.com/v1
OPENAI_MODEL=gpt-4

# Agent Proxy 端口
AGENT_PROXY_PORT=7000
```

#### deploy/config/config.json

```json
{
  "privacy_agent_url": "http://tsql:8123",
  "nexus_server_url": "http://nexus-server:8080"
}
```

## 技术栈

- **后端语言**: Go 1.21+, Python 3.11+
- **AI 框架**: LangGraph, LangChain
- **隐私计算**: SecretFlow SCQL
- **容器化**: Docker, Docker Compose
- **文件系统**: Nexus Remote FS
- **日志**: logrus (Go), logging (Python)

## 安全说明

- 所有用户请求需要通过 Nexus API Key 认证
- 隐私计算过程中数据不离开各方环境
- 使用 Ed25519 公钥加密保护数据传输
- 支持细粒度的列级权限控制

## 故障排查

### 常见问题

1. **Agent Proxy 无法连接到 AI Agent**
   - 检查 `deploy/config/config.json` 中的 `privacy_agent_url` 配置
   - 确认 Docker 网络连通性

2. **AI Agent 无法访问 Nexus**
   - 检查 metadata 中的 `x_auth` 和 `nexus_server_url`
   - 验证 Nexus API Key 是否有效

3. **隐私计算任务失败**
   - 查看容器日志: `docker logs tsql_<user>`
   - 检查双方数据集格式是否正确
   - 确认网络连通性

### 日志查看

```bash
# Agent Proxy 日志
docker-compose logs -f agent_proxy

# AI Agent 日志
docker-compose logs -f tsql

# 隐私计算容器日志
docker logs tsql_<user>
```

## 贡献指南

欢迎提交 Issue 和 Pull Request。

## 许可证

[待补充]

## 联系方式

[待补充]

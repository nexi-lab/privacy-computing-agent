# Privacy Computing Engine

基于 SecretFlow SCQL 的隐私计算引擎，提供联邦 SQL 查询和隐私求交功能。

## 功能概述

Privacy Computing Engine 是隐私计算的执行层，负责：

1. **任务管理**: 接收和处理隐私计算任务请求
2. **数据准备**: 从 Nexus 下载数据集并加载到 MySQL
3. **项目协调**: 管理 SCQL 项目和成员邀请
4. **权限控制**: 配置列级访问控制（CCL）
5. **查询执行**: 执行联邦 SQL 查询
6. **结果上传**: 将计算结果上传到 Nexus

## 架构设计

```
HTTP API (Port 8000)
       ↓
Task Handler
       ↓
┌──────────────────────────────┐
│  Privacy Computing Workflow  │
│                              │
│  1. Download Data (Nexus)    │
│  2. Load to MySQL            │
│  3. Create SCQL Project      │
│  4. Invite Partner           │
│  5. Create Table & Grant CCL │
│  6. Execute Query            │
│  7. Upload Result (Nexus)    │
└──────────────────────────────┘
       ↓
SCQL Broker (Port 8080)
       ↓
SCQL Engine (Port 8003)
```

## API 接口

### POST /api/privacy/run

启动隐私计算任务。

**请求示例**:
```bash
curl -X POST http://localhost:8000/api/privacy/run \
  -H "Content-Type: application/json" \
  -d '{
    "user": "alice",
    "data": "/workspace/alice/data.csv",
    "columns": [
      {
        "column": "id",
        "type": "string",
        "permissions": [
          {
            "user": "bob",
            "permission": "PLAINTEXT_AFTER_JOIN"
          }
        ]
      }
    ],
    "userkey": "MCowBQYDK2VwAyEA...",
    "userurl": "http://tsql_alice:8081",
    "engineURL": "tsql_alice:8003",
    "party": {
      "user": "bob",
      "pubkey": "MCowBQYDK2VwAyEA...",
      "partyURL": "http://tsql_bob:8081"
    },
    "runsql": "SELECT alice.id FROM alice INNER JOIN bob ON alice.id = bob.id"
  }'
```

**响应**:
```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "submitted"
}
```

**字段说明**:
- `user`: 当前用户标识
- `data`: Nexus 中的数据集路径
- `columns`: 列定义和权限配置
- `userkey`: 用户公钥（Ed25519）
- `userurl`: 用户 Broker URL
- `engineURL`: 用户 Engine URL
- `party`: 协作方信息
- `runsql`: 联邦 SQL 查询语句（仅发起方提供）

## 核心流程

### 1. 数据准备阶段

```go
// 从 Nexus 下载数据集
downloadFromNexus(req.Data, DATAFILE)

// 加载到 MySQL
checkDataset(req)
```

### 2. 项目初始化

```go
// 创建 SCQL 项目（发起方）
createProject()

// 邀请协作方（发起方）
inviteMember(req.Party.User)

// 接受邀请（协作方）
JoinProject()
```

### 3. 表和权限配置

```go
// 创建表
createTable(req)

// 授予列级权限
for _, col := range req.Columns {
    for _, perm := range col.Permissions {
        grantCCL(perm.User, req.User, col.Column, perm.Permission)
    }
}
```

### 4. 查询执行

```go
// 执行联邦 SQL（仅发起方）
if req.RunSQL != "" {
    runQuery(req.RunSQL, resultFile)
    uploadToNexus(resultFile, nexusResultPath)
}
```

## 权限类型

SCQL 支持以下列级权限：

- `PLAINTEXT`: 明文访问
- `PLAINTEXT_AFTER_JOIN`: JOIN 后明文访问
- `PLAINTEXT_AFTER_COMPARE`: 比较后明文访问
- `PLAINTEXT_AFTER_AGGREGATE`: 聚合后明文访问
- `ENCRYPTED_ONLY`: 仅密文访问

## 配置文件

### config.yml

SCQL Broker 配置文件（容器内路径：`/home/user/config/config.yml`）：

```yaml
intra_server:
  host: 0.0.0.0
  port: 8081

inter_server:
  host: 0.0.0.0
  port: 8080

party_code: _NODE_NAME_
engine:
  url: _NODE_ENGINE_URL_
  protocol: http
```

### party_info.json

多方信息配置（容器内路径：`/home/user/config/party_info.json`）：

```json
{
  "participants": [
    {
      "party_code": "_NODE_NAME_",
      "public_key": "_NODE_PUBKEY_",
      "url": "_NODE_SERVER_URL_"
    },
    {
      "party_code": "_PARTY_NAME_",
      "public_key": "_PARTY_PUBKEY_",
      "url": "_PARTY_SERVER_URL_"
    }
  ]
}
```

## 本地开发

### 前置要求

- Go 1.21+
- MySQL 5.7+
- SCQL Broker 和 Engine 二进制文件

### 运行步骤

1. **启动 MySQL**:
```bash
mysql -u root -p < init.sql
```

2. **启动 SCQL Broker**:
```bash
./broker --config config.yml
```

3. **启动 SCQL Engine**:
```bash
./scqlengine --flagfile gflags.conf
```

4. **启动 Privacy Computing Engine**:
```bash
cd privacy_computing
go run .
```

服务将在 `http://localhost:8000` 启动。

## Docker 部署

Privacy Computing Engine 通常作为容器运行，由 AI Agent 动态创建。

### 容器镜像

使用 `tsql:latest` 镜像，包含：
- SCQL Broker
- SCQL Engine
- MySQL
- Privacy Computing Engine
- Supervisor（进程管理）

### 容器创建

```bash
docker run -d \
  --name tsql_alice \
  -e USER=alice \
  tsql:latest
```

### 容器内服务

- **MySQL**: 3306
- **SCQL Broker (Inter)**: 8080
- **SCQL Broker (Intra)**: 8081
- **SCQL Engine**: 8003
- **Privacy Computing Engine**: 8000

## 日志

日志输出到 `tsqlctl.log` 文件，包含：

- 任务接收和处理
- 数据下载和加载
- SCQL 操作（项目、表、权限、查询）
- 结果上传

**日志级别**: DEBUG

## 故障排查

### 问题 1: 数据加载失败

**症状**: checkDataset 返回错误

**解决方案**:
```bash
# 检查 MySQL 连接
mysql -u root -e "SELECT 1"

# 检查数据文件
ls -l /home/user/data.csv

# 检查文件权限
chmod 644 /home/user/data.csv
```

### 问题 2: SCQL Broker 连接失败

**症状**: createProject 或其他 Broker 操作失败

**解决方案**:
```bash
# 检查 Broker 是否运行
curl http://localhost:8080/health

# 查看 Broker 日志
tail -f /home/user/broker.log
```

### 问题 3: 协作方无法加入项目

**症状**: JoinProject 返回 false

**解决方案**:
- 确认发起方已调用 inviteMember
- 检查网络连通性
- 验证公钥配置是否正确

## 性能优化

1. **异步处理**: 任务提交后立即返回，后台异步执行
2. **连接池**: MySQL 使用连接池
3. **批量操作**: 批量授予权限

## 安全考虑

1. **数据隔离**: 每个用户使用独立容器
2. **加密传输**: 使用 Ed25519 公钥加密
3. **权限控制**: 细粒度的列级权限
4. **审计日志**: 记录所有操作

## 未来改进

- [ ] 支持任务状态查询接口
- [ ] 添加任务取消功能
- [ ] 支持更多数据格式（JSON、Parquet）
- [ ] 优化大数据集处理
- [ ] 添加结果缓存机制

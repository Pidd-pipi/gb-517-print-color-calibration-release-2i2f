
# 印刷色彩批次校准放行

包装印刷设备、印刷批次、校样读数和放行决定平台。项目采用前后端分离和明确的领域分层，重点保证状态迁移、RBAC、审计日志、请求追踪与限流在各层保持一致。

## Docker Compose 快速启动

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps
```

- Web 工作台：http://127.0.0.1:18517
- 后端健康检查：http://127.0.0.1:19517/healthz
- 后端 API：http://127.0.0.1:19517/api
- 演示管理员：`admin` / `Admin123!`（仅限本地演示，生产环境必须更换）

演示账号共用本地密码 `Admin123!`：

| 用户名 | 角色 | 权限边界 |
|---|---|---|
| `viewer` | 只读观察员 | 查看业务数据，不能写入或访问审计 |
| `operator` | 现场操作员 | 创建、编辑和推进现场流程，不能接收校样或放行批次 |
| `reviewer` | 质量复核员 | 独立接收/拒绝校样、放行/隔离批次并查看审计 |
| `admin` | 系统管理员 | 完整权限和删除权限 |

停止并清理本项目容器与数据卷：

```bash
docker compose down -v --remove-orphans
```

## 主要功能

| 业务模块 | 后端实体 | API 前缀 | 状态流 |
|---|---|---|---|
| 印刷设备 | `PressUnit` | `/api/presses` | ready, setup, printing, maintenance |
| 印刷批次 | `PrintRun` | `/api/runs` | setup, printing, proofing, hold, released |
| 色彩校样 | `ColorProof` | `/api/proofs` | captured, review, accepted, rejected |
| 放行决定 | `ReleaseDecision` | `/api/release` | draft, release, rework, quarantine |

- JWT 登录和 viewer/operator/reviewer/admin 四级 RBAC。
- 所有状态变化使用乐观锁并写入不可覆盖的审计日志。
- 色彩配置和放行决定在同一数据库事务内追加不可变修订；每个版本保留业务证据、操作者、请求 ID 和原因。
- 已放行或隔离的决定禁止覆盖式编辑；校样接收/拒绝和批次放行只能由 `reviewer/admin` 完成。
- 请求 ID、结构化日志、全局错误映射和 Redis 分布式限流。
- 提供脱敏运行配置、当前会话、审计汇总和单实体审计历史接口。
- 业务工作台支持查询、新建、状态推进、分页、角色切换、色彩读数、版本详情及操作审计查看。

## 技术栈

| 层次 | 技术 |
|---|---|
| 前端 | React 18 + TypeScript + Vite + Ant Design |
| 后端 | Go 1.22 + Gin + GORM |
| 数据 | MySQL + Redis |
| 部署 | Docker Compose + Nginx |

## 本地开发

后端可使用 SQLite 开发模式，不需要先启动数据库：

```bash
cd backend
go mod download
DATABASE_DRIVER=sqlite DATABASE_DSN=local.db REDIS_ADDR='' \
JWT_SECRET=local-development-secret PORT=8080 go run ./cmd/server
```

前端开发服务器：

```bash
cd frontend
npm install
npm run dev
```

质量检查：

```bash
cd backend && go test ./... && go test -race ./... && go vet ./... && go build ./...
cd ../frontend && npm run typecheck && npm run build
cd .. && docker compose config --quiet
```

也可以从项目根目录执行 `./scripts/validate.sh`。脚本从构建、健康检查一直验证到 viewer/operator/reviewer 的 RBAC、校样复核和不可变放行修订链，并在结束时关闭容器。完整验收记录见 [`VALIDATION.md`](./VALIDATION.md)。

## 目录结构

```text
.
├── backend/
│   ├── cmd/server/                 # 服务入口与优雅退出
│   └── internal/
│       ├── config/                 # 环境配置
│       ├── constants/              # 状态枚举与迁移图
│       ├── database/               # 连接、迁移与演示数据
│       ├── dto/                    # 输入契约
│       ├── handler/                # HTTP 接口
│       ├── middleware/             # JWT、追踪、限流
│       ├── model/                  # GORM 实体
│       ├── repository/             # 持久化边界
│       ├── router/                 # 路由装配
│       ├── service/                # 业务规则与审计
│       └── util/                   # 统一 HTTP 响应
├── frontend/src/
│   ├── api/                        # 按实体拆分的 API
│   ├── components/common/          # 共享业务组件
│   ├── hooks/                      # 认证与分页 hooks
│   ├── pages/                      # 五个路由页面
│   ├── router/                     # 路由配置
│   ├── stores/                     # 按实体拆分的状态仓库
│   ├── types/                      # 共享类型与枚举
│   └── utils/                      # 格式化与状态工具
├── docker-compose.yml
└── runtime_smoke.json
```

## 共享枚举位置

| 枚举 | 值 | 前后端出现位置 |
|---|---|---|
| `RunState` | `setup, printing, proofing, hold, released` | `backend/internal/constants/status.go`、`frontend/src/types/status.ts` |
| `DecisionType` | `release, rework, quarantine` | `backend/internal/constants/status.go`、`frontend/src/types/status.ts` |

每个实体自己的完整迁移图同样位于 `backend/internal/constants/status.go`；页面使用的状态列表位于 `frontend/src/types/status.ts`。修改状态时必须同步两处并更新对应服务测试。

## 环境变量

| 变量 | 说明 |
|---|---|
| `COMPOSE_PROJECT_NAME` | 固定英文 Compose 项目名，支持中文父目录 |
| `DB_NAME/DB_USER/DB_PASSWORD` | 数据库名称与业务账号 |
| `DB_ROOT_PASSWORD` | MySQL 管理员密码（PostgreSQL 项目保留统一模板字段） |
| `JWT_SECRET` | JWT 签名密钥，生产环境必须替换 |
| `FRONTEND_PORT/BACKEND_PORT/DB_PORT` | 宿主机端口映射 |
| `REDIS_PORT` | Redis 宿主机端口 |
| `MINIO_*` | 证据对象存储配置（启用 MinIO 的项目） |

## API 使用示例

```bash
token=$(curl -sS -X POST http://127.0.0.1:19517/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Admin123!"}' | jq -r '.data.token')

curl -sS http://127.0.0.1:19517/api/overview \
  -H "Authorization: Bearer $token"
```

## License

MIT

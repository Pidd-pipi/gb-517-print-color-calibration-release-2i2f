请生成 `print-color-calibration-release`「印刷色彩批次校准放行」Go 全栈项目，面向包装印刷厂管理印刷机、色彩配置、校样读数和批次放行决定。不要实现订单、库存、商城、成本或普通报表系统。

## 项目主要需求

复杂度下限：核心实体不少于 3 个、核心页面不少于 4 个、横切关注点不少于 2 个、共享前端组件不少于 3 个、自定义 hooks/utils 不少于 2 个、后端中间件不少于 2 个。

### 核心实体

`PressUnit`（印刷机与色组）、`PrintRun`（印刷批次与配置）、`ColorProof`（校样读数）、`ReleaseDecision`（放行/返修/隔离）必须全链路贯穿数据库、Go 分层和前端。

### 核心页面

`/presses` 设备；`/runs` 印刷批次；`/proofs` 校样；`/release` 放行决定；`/audit` 审计。`RunStateBadge` 在批次和放行页共用，`ColorTable` 在校样和详情页共用。

### 横切关注点

RBAC 联动角色、Go auth/rbac middleware、前端路由守卫和显隐；色彩配置与放行决定版本化审计；实现全局错误处理、请求 ID、Redis 限流。

### 共享枚举/组件

同步 `RunState`（setup/printing/proofing/hold/released）与 `DecisionType`（release/rework/quarantine）。共享 `StatusBadge`、`ColorTable`、`EmptyState`，hooks 为 `useAuth`、`usePagination`。

### 技术与规模要求

前端 React 18 + TypeScript + Vite + Ant Design；后端 Go 1.22 + Gin + GORM；MySQL + Redis。目标 2700–3900 行、26–38 个 `.go` 文件。

### 文件结构强制清单

前端 `api/stores/types/components/common/hooks/pages/router/utils`；后端 `model/dto/repository/service/handler/router/middleware/constants/util`，禁止合并职责。

### 结构红线

严禁合并职责到单一文件；印刷批次、校样和放行需要独立跨层文件。

### 部署与交付

根目录必须提供 `docker-compose.yml`（顶层 `name: print-color-calibration-release`，且不写 `version:`）、`.env` 和 `.env.example`（均含 `COMPOSE_PROJECT_NAME=print-color-calibration-release`）、`README.md`、`frontend/Dockerfile`、`backend/Dockerfile` 和 `frontend/nginx.conf`。前端端口 `18517`、后端端口 `19517`；Nginx 反代 `/api`，healthcheck、命名卷和 `condition: service_healthy` 齐全，提供真实 `/healthz`、Git 初始化。

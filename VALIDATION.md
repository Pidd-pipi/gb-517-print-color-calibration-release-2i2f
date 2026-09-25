# 验收记录

验收日期：2026-08-22（Asia/Shanghai）

## 静态与测试

以下命令均以退出码 0 完成：

```bash
cd backend
go test ./...
go test -race ./...
go vet ./...
go build ./...

cd ../frontend
npm run typecheck
npm run build

cd ..
docker compose config --quiet
```

- 路由集成测试覆盖 viewer 写入 403、operator 放行 403、reviewer 放行成功、已决记录更新 409，以及色彩配置/放行决定两条不可变修订链。
- 非测试 Go 代码为 38 个文件、3143 行，符合提示词的 26-38 文件与 2700-3900 行范围。

## 空卷 Compose 与 API

先执行 `docker compose down -v --remove-orphans`，随后以 `KEEP_RUNNING=1 ./scripts/validate.sh` 从空命名卷启动。MySQL、Redis、backend、frontend 均通过 healthcheck。

脚本实际验证：

- `/healthz`、前端首页、session、runtime、overview 和四组实体列表正常。
- 创建设备并推进状态后，审计总数和迁移计数同步增加。
- viewer 对写接口和审计接口均得到 403。
- operator 可采集校样并提交 review，但接收校样得到 403；reviewer 接收成功。
- operator 创建 draft 决定后直接 release 得到 403；reviewer 放行成功并生成 v2。
- 决定详情返回 v2/v1 两条修订，操作者分别为 reviewer/operator，请求 ID 分别为 `release-review-smoke`、`release-create-smoke`。

## 内置 Browser

只使用 Codex 内置 Browser 验收，没有调用外部 Chrome。

| 页面/场景 | 实测结果 |
|---|---|
| `/presses` | 列表、搜索区、新增确认框和 `ready -> setup` 状态推进正常 |
| `/runs` | 三条批次可见，`RunStateBadge` 显示待装版/印刷中/校样中 |
| `/proofs` | `ColorTable` 展示四条读数；详情复用同一组件并显示 v3 已接收校样 |
| `/release` | 放行依据读数、相关批次状态和决定详情正常；详情显示 v2/v1 完整版本链 |
| `/audit` | 审计列表显示操作者、迁移前后状态、实体和请求 ID |
| RBAC | viewer 无新增/推进/审计入口且直达 `/audit` 被重定向；operator 隐藏复核动作；reviewer 显示放行与审计入口 |
| 响应式 | 390x844 视口下导航、指标、工具栏和滚动表格无页面级横向溢出，`documentWidth == viewport == 390` |
| 控制台 | 全流程完成后 error/warning 日志为 0 |

最终交付前执行：

```bash
docker compose down -v --remove-orphans
```

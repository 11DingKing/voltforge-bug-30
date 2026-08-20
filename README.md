# VoltForge 快充认证与运行控制平台

VoltForge 面向充电器厂商、电池实验室、线缆认证机构和质量审核人员，管理手机、充电器与线缆之间的快充协议握手、功率分配、温度保护、认证证据和运行审计。平台覆盖从能力登记、协议协商、充电会话、异常缓解到复测认证的完整链路。

## 业务流程

- 厂商登记设备的协议集合、额定功率、氮化镓器件、端口数、热限制和电芯架构。
- 实验室记录设备能力检查，系统在设备、充电器和线缆之间选择共同认证协议，并按线缆额定功率限制输出。
- 多端口适配器按照总功率预算和端口额定值分配充电功率；重复请求使用分配标识保持幂等。
- 充电过程中持续接收电池、适配器和环境温度，正常、降额和紧急关断分别进入不同状态。
- 认证审核员核对协议握手、线缆证书、热管理、功率显示和遥测新鲜度，证据不足的产品进入复测。
- 每次状态变化写入审计和遥测事件，后台任务负责超时检查、重试、失败归档和订阅游标推进。

## 身份与权限

内置演示账号：`auditor/auditor-demo`、`labreviewer/labreviewer-demo`、`vendorengineer/vendorengineer-demo`、`testengineer/testengineer-demo`。登录返回有过期时间的可撤销会话 Token，退出后立即失效。审核员可以管理产品和认证，实验室复核能力与证据，厂商工程师提交缓解材料，测试工程师登记协议与遥测结果。

## 目录

```text
cmd/voltforge/       HTTP 服务入口
cmd/voltforgectl/    维护与遥测导出命令
internal/auth/       登录、会话、角色鉴权
internal/charging/   产品、认证问题和复测状态机
internal/protocol/   快充协议协商与能力交集
internal/power/      多端口功率预算和幂等分配
internal/thermal/    温度阈值、降额和紧急关断
internal/cable/      线缆协议证书、额定功率和撤销校验
internal/certification/ 认证证据聚合与有效期规则
internal/domain/     会话、握手、事件和审计领域对象
internal/service/    事务编排、批处理、遥测和订阅
internal/storage/    SQLite repository、迁移和重启恢复
internal/httpapi/    HTTP API、请求 ID、统一错误和健康检查
internal/scheduler/  超时扫描、重试和优雅停止
```

## 运行

需要 Go 1.26 和 `GOTOOLCHAIN=local`：

```bash
export GOTOOLCHAIN=local
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
go run ./cmd/voltforge
```

默认监听 `:56058`，数据目录为 `./data`。可通过 `VOLTFORGE_PORT`、`VOLTFORGE_DATA_DIR`、`VOLTFORGE_LOG_LEVEL` 和 `VOLTFORGE_ATTESTATION_TIMEOUT_HOURS` 覆盖配置。`/healthz` 检查存活，`/readyz` 检查数据库迁移和连接。

## 公开 API

```text
POST /api/v1/auth/login
POST /api/v1/auth/logout
POST /api/v1/charging/products
GET  /api/v1/charging/products
POST /api/v1/charging/issues
POST /api/v1/charging/issues/{id}/assign
POST /api/v1/charging/issues/{id}/mitigate
POST /api/v1/charging/issues/{id}/certify
POST /api/v1/handshakes
POST /api/v1/handshakes/{id}/attestation
GET  /api/v1/sessions/{id}
GET  /api/v1/telemetry
GET  /api/v1/audit
GET  /healthz
GET  /readyz
```

## 数据与迁移

生产路径使用 SQLite WAL 和真实 SQL，内置迁移在服务启动时幂等执行。数据库包含身份、会话、产品、认证问题、握手、充电会话、缓解请求、批处理、遥测、事件、审计和失败重试等关联表；事务写入、版本条件更新和重启恢复均有集成测试覆盖。顶层 `migrations/001_init.sql` 与嵌入应用的迁移保持一致。

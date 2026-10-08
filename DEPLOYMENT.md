# us2 部署

更新日期：2026-10-08。目标服务器 `us2`，项目目录 `/opt/docker_projects/grok2api`，Compose 项目名 `grok2api-official`。

- 配置检查：`scripts/deploy-grok2api-official.sh config -q`
- 状态查询：`scripts/deploy-grok2api-official.sh ps`
- 发布：准备并验证固定本地镜像后，仅修改 `docker-compose.yml` 中 `services.grok2api.image`，执行 `scripts/deploy-grok2api-official.sh up -d --no-deps --pull never grok2api`。
- 回滚：执行对应备份目录中的 `ROLLBACK.sh`；它恢复原 Compose 镜像和原 Build 客户端设置，重建目标服务，并检查健康。不要恢复整库覆盖发布后业务数据。

入口使用 `--env-file /opt/docker_projects/grok2api/.env -f /opt/docker_projects/grok2api/docker-compose.yml`。环境和配置秘密仅保存在服务器受限文件中。

## 运行身份

- 服务名/容器名：`grok2api`；质量守护侧车：`egress-quality-guard` / `grok2api-egress-quality-guard`。
- 宿主端口：`172.17.0.1:9011`、`10.77.91.2:9011`，映射容器 `8000/tcp`。保留既有 WireGuard 接口启动依赖。
- 公网入口：`https://grok2api.codeai.de5.net`，共享 Nginx 路由源为 `/opt/docker_projects/nginx-proxy/config-src/http.d/013-server.conf`，上游为 `host.docker.internal:9011`。普通版本更新不修改或重启 Nginx。
- 数据：`official-data:/app/data`，SQLite 数据库 `official-data/backend.db`；配置 `config.yaml:/run/grok2api/config.yaml:ro`。
- 守护状态卷：`grok2api-official_quality_guard_state`，保留实际卷身份。
- 重启策略：`unless-stopped`；主服务健康检查为容器内 `/healthz`。

## 发布与验证

本次同步上游 `7c889a960e2638341b4dae9a5c81af0e0f38c87f`，修复动态 Build 能力污染其他 Provider 以及分段 SSE 首片段被误判为快速倾倒。

构建使用现有官方镜像的固定 digest、Go 1.26.4 构建的 Linux amd64 二进制及 `pnpm build` 产出的前端。镜像记录源代码树 ID、基础上游提交和源码归档 SHA-256；发布记录将最终 Git 提交的 tree 与镜像 tree 核对。

已有运行设置会覆盖源码默认值，因此发布将持久设置中原默认 `1.0.4` / `grok-shell/1.0.4 (linux; x86_64)` 更新为 `1.0.40` 对应值，保留其他字段。备份包含旧设置，回滚只恢复这两个字段。

服务器恢复材料及实际检查记录保存在 `backups/20261008-upstream/`，目录权限 `0700`；配置和 SQLite 原生一致性备份权限 `0600`。保留原镜像，不自动清理。

验收包含后端测试、前端构建与测试、部署前后 API 认证边界和真实 Build/Web/Console SSE、公共 TLS 页面及静态资源、数据库完整性、挂载身份和其他容器 ID/StartedAt/RestartCount。上游失败与程序回归分别记录；健康状态不代替推理成功。

## 历史待整改与例外

- 质量守护侧车没有 Docker healthcheck：暂用进程、状态卷和业务回归检查，下一次升级守护工具时复核。
- WireGuard 地址绑定属于既有业务例外：本次保留接口和端口；下一次网络维护核实开机依赖，不能用一次在线重启证明主机重启恢复。
- 固定容器名和既有 Compose/卷名属于外部调用依赖：保留以避免生成新卷或改变侧车目标地址。
- 基础镜像固定 digest，升级基础依赖另行验证；不因本次业务更新升级无关服务。

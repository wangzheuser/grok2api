# us2 部署

更新日期：2026-10-21。目标服务器 `us2`，项目目录 `/opt/docker_projects/grok2api`，Compose 项目名 `grok2api-official`。

- 配置检查：`scripts/deploy-grok2api-official.sh config -q`
- 状态查询：`scripts/deploy-grok2api-official.sh ps`
- 发布：准备并验证固定本地镜像后，仅修改 `docker-compose.yml` 中 `services.grok2api.image`，执行 `scripts/deploy-grok2api-official.sh up -d --no-deps --pull never grok2api`。
- 回滚：执行对应备份目录中的 `ROLLBACK.sh`；它恢复本次变更的 Compose 镜像，重建目标服务，并检查健康。不要恢复整库覆盖发布后业务数据。

入口使用 `--env-file /opt/docker_projects/grok2api/.env -f /opt/docker_projects/grok2api/docker-compose.yml`。环境和配置秘密仅保存在服务器受限文件中。

## 运行身份

- 服务名/容器名：`grok2api`；质量守护侧车：`egress-quality-guard` / `grok2api-egress-quality-guard`。
- 宿主端口：`172.17.0.1:9011`、`10.77.91.2:9011`，映射容器 `8000/tcp`。保留既有 WireGuard 接口启动依赖。
- 公网入口：`https://grok2api.codeai.de5.net`，共享 Nginx 路由源为 `/opt/docker_projects/nginx-proxy/config-src/http.d/013-server.conf`，上游为 `host.docker.internal:9011`。普通版本更新不修改或重启 Nginx。
- 数据：`official-data:/app/data`，SQLite 数据库 `official-data/backend.db`；配置 `config.yaml:/run/grok2api/config.yaml:ro`。
- 守护状态卷：`grok2api-official_quality_guard_state`，保留实际卷身份。
- 重启策略：`unless-stopped`；主服务健康检查为容器内 `/healthz`。

## 发布与验证

本次仅在 `dev` 同步上游 `e03cacb9d142e16d5a419fbf40efbd8f1b0cc819`，保留动态 Build 能力的 Provider 隔离及分段 SSE 首片段误判修复。补齐 consent_token 自闭合 input 解析；同步视频重试账号范围、Imagine 请求隔离、语音超时及计费、账号绑定查询和瞬时重试标记等改动。`main` 保持不变。

镜像在本地构建为 Linux amd64，固定运行基础镜像，打包交叉编译的 Go 二进制及 `pnpm build` 产出的前端。镜像记录源代码树 ID、基础上游提交和源码归档 SHA-256；发布记录将最终 Git 提交的 tree 与镜像 tree 核对。

已有运行设置会覆盖源码默认值。本次保留生产设置及质量守护策略，不自动开启新增的降质账号停用功能；Build 客户端版本以运行配置和业务验证结果为准，不因源码推荐值更新而覆盖持久设置。

服务器恢复材料和检查记录保存在项目目录外 `/opt/backups/grok2api-20261021-upstream/`，目录权限 `0700`；配置和 SQLite 原生一致性备份权限 `0600`。保留上一版本镜像。回滚只恢复本次改变的 Compose 镜像，不用发布前整库覆盖后续业务写入。

验收包含后端测试、前端构建与测试、部署前后 API 认证边界和真实 Build/Web/Console SSE、公共 TLS 页面及静态资源、数据库完整性、挂载身份和其他容器 ID/StartedAt/RestartCount。上游失败与程序回归分别记录；健康状态不代替推理成功。

## 长推理与分层就绪检查

- 文本流在等待选路、上游响应头或质量检查时，每 15 秒发送 SSE 注释；取得上游流后只在完整事件之间补充注释。注释不计入首 token、用量或上游有效生成活动，原有质量门禁与上游空闲超时保持不变。
- 首个心跳前仍可返回原 HTTP 错误状态；心跳提交 HTTP 200 后，失败通过对应协议的终止错误事件返回。调用方必须检查终止事件，不能仅按 HTTP 200 判断成功。
- 不通过增大共享 Nginx 超时掩盖长时间无响应。2026-10-21 的发布前探测中，直连请求在 183 秒完成，公网另一请求在 60 秒被入口切断；回归需同时记录响应头时间、心跳、完整答案和审计。
- `readyz` 每次重新检查真实账号状态，但按 Provider/额度模式复用基础资格检查，避免额度耗尽时逐模型重复扫描全池。Console 额度不足仍如实显示不可用，不自动关闭路由。
- Statsig 的 `/index` 和 `/` 页面已实测缺少 `grok-site-verification`。仅对明确的元数据缺失共享 5 分钟退避；网络或代理 403 不污染共享退避，配置更新会清除退避。状态明确显示未生成签名，不伪造签名或把该依赖标为已预热；Web 保留既有按需回退行为。
- 本次恢复材料位于 `/opt/backups/grok2api-20261021-readiness/`，上一批备份不覆盖。只发布 `dev`，`main` 不变。

## 历史待整改与例外

- 质量守护侧车没有 Docker healthcheck：暂用进程、状态卷和业务回归检查，下一次升级守护工具时复核。
- WireGuard 地址绑定属于既有业务例外：本次保留接口和端口；下一次网络维护核实开机依赖，不能用一次在线重启证明主机重启恢复。
- 固定容器名和既有 Compose/卷名属于外部调用依赖：保留以避免生成新卷或改变侧车目标地址。
- 基础镜像固定 digest，升级基础依赖另行验证；不因本次业务更新升级无关服务。

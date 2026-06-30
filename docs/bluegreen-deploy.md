# 蓝绿无感发布方案（1Panel + docker-1panel.sh）

> 目标：发布/重启 new-api 时**对线上客户零感知**（无几秒中断），用于当前单机
> 1Panel 部署。本文记录已核实的架构事实、设计、master 单例处理方案，以及落地步骤。

## 背景

- 单一大客户实时流量（高峰 ~2200 请求/分，~$48K/天），常规 `docker-1panel.sh update`
  采用「先删旧容器→再起新容器」，会有 ~4 秒中断。需要改为蓝绿（A/B）发布。
- 待用此方式无感上线的功能：**渠道 429 冷却**（按 Anthropic 限流头精确冷却，避免 429
  同时不影响超刷），代码已实现并通过构建，等无感发布通道就绪后上线。

## 已核实的架构事实

### 反向代理链路（切换点）

```
客户(外网) → openresty(host 网络，监听 0.0.0.0:80/443，server_name=43.130.52.129)
           → /opt/1panel/1panel/www/sites/43.130.52.129/proxy/root.conf
             location ^~ / { proxy_pass http://127.0.0.1:56781; ... }
           → new-api 容器(127.0.0.1:56781 → 容器内 3000)
```

- 站点主配置：容器内 `/usr/local/openresty/nginx/conf/conf.d/43.130.52.129.conf`
  （`listen 80/443 default_server; server_name 43.130.52.129;`），第 36 行
  `include /www/sites/43.130.52.129/proxy/*.conf;` 引入上面的 `root.conf`。
- 该 conf 在宿主机路径：`/opt/1panel/1panel/www/sites/43.130.52.129/proxy/root.conf`
  （openresty 容器以 host 网络运行；站点目录经 `/opt/1panel/1panel/www → /www` 挂载）。
- 反代已针对 AI 流式调优：`proxy_read_timeout 3600s`、`proxy_buffering off`、
  `proxy_http_version 1.1`、`client_max_body_size 1024m`。

**切换点 = 改 `root.conf` 里的 `proxy_pass` 端口 + 优雅重载 openresty。**
nginx `reload` 为优雅重载：旧连接由旧 worker 继续服务至完成，新连接走新配置，
**零断连、零丢请求**。

### 容器与端口

- 容器名：`new-api`，镜像：`new-api:1panel`（上一个版本保留为 `new-api:1panel-previous`）。
- 端口：宿主 `127.0.0.1:56781` → 容器 `3000`。蓝绿需第二个槽位（建议 `56782`）。
- 部署脚本：`bin/docker-1panel.sh`（`update` 会重建容器，即当前会造成中断的命令）。

### master 单例机制（蓝绿的核心约束）

- 判定：`common/init.go` → `IsMasterNode = os.Getenv("NODE_TYPE") != "slave"`，
  **不设 `NODE_TYPE` 即为 master**。当前 `new-api` 容器未设，故为 master。
- master 门控的后台任务（5 个）：
  | 任务 | 位置 | 并存双跑风险 |
  |---|---|---|
  | 渠道备货池**自动晋升** | `controller/channel_preparation_auto_promotion.go:724` | **高**：每分钟跑、会创建渠道行 |
  | 订阅额度重置 | `service/subscription_reset_task.go:31` | 低（按日/周/月、幂等置值） |
  | Codex 凭证刷新 | `service/codex_credential_refresh_task.go:37` | 低（幂等刷新） |
  | 渠道自动测试 | `controller/channel-test.go:995` | 低（重复测试无害） |
  | 上游模型更新检查 | `controller/channel_upstream_update.go:654` | 低（冗余更新无害） |
- **有利事实**：自动晋升调度器启动后**先 sleep 一个间隔（默认 60s）才首次执行**
  （`StartChannelPreparationAutoPromotionTask` 循环内先 `time.Sleep(interval)` 再 run）。
  因此绿容器启动后 60s 内不会晋升。

## 设计决策：master 处理用方案 A（切换窗口暂停自动晋升）

候选方案对比：

- **A（采用）切换窗口暂停自动晋升**：仅需管住唯一危险任务；零代码、零风险；
  切换后稳态为干净的单 master（绿）。其它 4 个任务并存几秒双跑无害，不处理。
- **B 绿起为 `NODE_TYPE=slave`**：停掉蓝(master)后**无 master**，自动晋升永久停摆，
  需再重启绿才恢复 → 不彻底，否决。
- **C Redis 锁选主**：多节点 HA 的根治方案，任意时刻自动只有一个 master，蓝绿天然无忧；
  但要改 5 处任务启动 + 锁续期，单机场景风险/工作量过大。**留作将来多节点常驻时再做。**

### 关键执行顺序（保证绿永不与蓝重复晋升）

```
1. 先在 DB 关闭自动晋升：scheduler_enabled = false
2. 再启动绿容器        → 绿启动即读到「已关」，其调度器不会跑
3. 健康检查绿(/api/status via 56782) 通过
4. 改 root.conf: proxy_pass 56781 → 56782
5. 优雅重载: docker exec 1Panel-openresty-BMnu openresty -s reload   (无感切流)
6. 排空旧连接后停蓝(new-api)
7. 重新打开自动晋升: scheduler_enabled = true   → 绿接管，恢复每分钟晋升
```

要点：**先暂停、后起绿**。绿从出生即看到晋升关闭，不可能在蓝存活期间晋升；
蓝在窗口内即便晋升一次，也是「唯一合法 master」的正常行为，并非双跑。

### 冷却功能与蓝绿的兼容性

- 429 冷却表是**各实例内存独立**的；蓝绿并存期间各自学习、互不影响；
  切换到绿后，绿在 1 分钟内自行重建冷却表，无影响。

## 落地：为 docker-1panel.sh 增加 `switch` 子命令

封装上述 7 步：

- 端口轮换：检测当前活跃端口（读 `root.conf` 的 `proxy_pass`），新槽位取另一个（56781↔56782）。
- 暂停/恢复自动晋升：通过 DB（options 表 `channel_preparation_auto_promotion_setting.scheduler_enabled`）
  或管理 API。注意多进程同步：**先写 DB 再起绿**，确保绿启动即生效。
- 起绿：用现有 `run` 逻辑但换容器名（如 `new-api-green`）与端口。
- 健康检查：复用脚本里的 `wait_for_app`（探 `/api/status`）。
- 切流：sed 改 `root.conf` 的 `proxy_pass` 端口 → `openresty -s reload`。
- 停蓝：可选「排空等待」窗口（注意长 SSE 最长 3600s，实际给一个有限 grace，
  超时后仍停，少量长连接会被切，权衡可接受）。
- 失败回滚：任意步骤失败则不改 proxy_pass / 改回原端口并 reload，绿容器删除。

### 回滚

- 切流前失败：删除绿容器即可，蓝不受影响。
- 切流后发现问题：把 `root.conf` 的 `proxy_pass` 改回旧端口 + `openresty -s reload`，
  再删绿。镜像层面仍可用 `docker-1panel.sh rollback`（`new-api:1panel-previous`）。

## 验证清单（发布后）

- `/api/status` 经 80/443 正常；GIN 日志状态码分布正常（无异常 5xx 抬升）。
- 自动晋升恢复运行（日志 `channel preparation auto promotion ...`）。
- 冷却生效：429 曲线下降、流量在多渠道间摊开（对比上线前后每渠道 RPM）。
- 仅一个 new-api 容器存活，且为 master。

## 附：429 冷却功能（随本通道上线）

- 新增 `model/channel_cooldown.go`：解析 `Retry-After` / `anthropic-ratelimit-*-reset/-remaining`，
  按上游确切重置时刻冷却渠道；只认速率头、不碰额度（超刷不减）。
- `relay/channel/api_request.go doRequest`：统一出口对每个响应调用
  `ApplyUpstreamRateLimitHeaders`。
- `model/channel_cache.go`：选渠道跳过冷却中的渠道，全冷却则降级用全集。
- 配置（`common/constants.go` + `model/option.go`，可后台 live 调）：
  `ChannelCooldownEnabled`(默认 true)、`ChannelCooldownProactiveEnabled`(true)、
  `ChannelCooldownMaxSeconds`(120)、`ChannelCooldownMinRequestsRemaining`(1)、
  `ChannelCooldownMinInputTokensRemaining`(0)。

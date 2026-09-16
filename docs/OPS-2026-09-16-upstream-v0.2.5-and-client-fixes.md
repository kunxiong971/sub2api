# OPS-2026-09-16 上游 v0.2.5 合并 + 客户端故障修复 + 首屏体验优化

> 全部由 AI 代办（用户授权免审），逐项验证后部署。
> 当前线上：主站 `sub2api:pref-20260916`、对话台 `lobe-chat:bridge-sub2api-v13`。

## 一、部署总览（按时间序）

| # | 内容 | 镜像 / 产物 | 状态 |
|---|---|---|---|
| 0 | 本地未提交改动入库（生图台 URL 载荷瘦身批次） | sub2api `d55a084b3` | ✅ |
| 1 | **上游 v0.2.5 合并**（197 提交，2 处冲突） | `sub2api:v025-20260916` | ✅ 迁移已应用 |
| 2 | **忘记密码「前置站点」配置**（settings.frontend_url） | 纯配置，无镜像 | ✅ 已实测 |
| 3 | **认证类提示中文化**（30 处英文 message） | `sub2api:i18n-20260916` | ✅ 已实测 |
| 4 | **对话台默认模型问题**（4 轮定位与修复） | `lobe-chat:bridge-sub2api-v9/v10/v12` | ✅ |
| 5 | **首屏默认模型优化**（URL 短参数 + 渲染前读取） | `sub2api:pref-20260916` + `lobe-chat:bridge-sub2api-v13` | ✅ 当前线上 |
| 6 | 服务器清理（/tmp 4.5G、历史镜像 14G） | — | ✅ |

---

## 二、上游 v0.2.5 合并

### 规模与冲突
- 上游新增 **197** 提交，本地 fork **30** 提交；交叉文件 34 个 → **真冲突仅 2 个**。
- `backend/cmd/server/wire_gen.go`：保留我方 `playgroundConfigService` / `playgroundHandler`，采纳上游 `ollamaCloudUsageService`、`leaderLockCache`。
- `backend/internal/handler/admin/channel_monitor_handler.go`：`Provider` oneof 加 `opencode_go`（上游）+ `APIMode` oneof 加 `images`、新增 `ImageMode` 字段（我方）。
- **上游完全未触碰任何 playground 文件** → 核心二开资产零冲突。

### 合并后必做
- 重跑 `go generate ./ent`（结果无变化，证明 ent 自动合并正确）；
- 重跑 `go generate ./cmd/server` 重新生成 `wire_gen.go` —— 会移除上游已不再消费的 `identityCache` / `identityService`（**上游该文件是滞后产物**，重新生成才是正确状态）。
- 验证：`go build ./...` 通过；单测结论见「五、踩坑」。

### 迁移执行
- `238_opencode_go_platform.sql`、`238_purge_unlimited_user_platform_quotas.sql` 于 00:54:27 自动应用。
- **`user_platform_quotas` 91 行 → 0**（用户确认业务未使用平台限额；备份见下）。
- **编号 238 撞车实测无害**：`schema_migrations` 主键为**完整文件名**（`migrations_runner.go:466` 用 `TrimSuffix(name,".sql")`），我方 `238_playground_app_configs.sql` 与上游两条 238 并存。

---

## 三、客户端故障修复

### 1. 忘记密码「前置站点」（settings.frontend_url）
- 现象：后台已开启忘记密码，但缺少"前端地址"，点击即失败。
- 根因：`auth_handler.go:617-622` 对 `frontend_url` 有**空值硬校验**，为空时直接返回「密码重置功能未配置」。
- 处置：`settings.frontend_url` 由空设为 `https://ai.1canc.com`（规则：绝对 http(s) URL、可带路径、不可带 query/userinfo）。
- 实测：`POST /api/v1/auth/forgot-password` → `{"code":0,...}` 正常返回。

### 2. 认证类提示中文化
- 根因：所有认证 message 硬编码英文，且前端**优先取后端 message**（无 code→文案映射层）。
- 改动（共 30 处，**错误码不变**）：`auth_service.go` 21 处、`auth_handler.go` 17 处、`auth_email_binding.go` / `auth_email_oauth_auto.go` 若干。
- 实测：登录失败 → `{"code":401,"message":"邮箱或密码错误"}`；重复注册 → `{"code":400,"message":"请先完成邮箱验证"}`。

### 3. 对话台默认模型停留在内置 DeepSeek（**四轮定位**）
| 轮次 | 判断 | 结果 |
|---|---|---|
| v9 | 认为是"注入时机早于 store 就绪"，加三次固定延迟兜底重放（1.2/3.5/8s） | 部分改善，仍需等十几秒 |
| v10 | 改为**轮询探测 store 就绪**（`waitStoresReady`），数据一到即校正 | 明显改善 |
| — | 用户反馈"仍会选中被禁用的内置模型" | 转向查写入是否真正生效 |
| v12 | **真根因**：`builtinAgentSelectors` 从 `@/store/agent` 解构 —— 该模块**只导出 `useAgentStore`**，正确路径是 `@/store/agent/selectors`。取到 `undefined` → `inboxAgentId` 立即抛 TypeError → **整段「指派默认模型」被 catch 静默吞掉，从未执行** | ✅ 功能修复 |

- 关键佐证：`[sub2api] assign default model failed TypeError: Cannot read properties of undefined (reading 'inboxAgentId')`。
- 为什么前几轮没发现：「隐藏内置模型」在**另一个独立 try 块**中（故它生效、内置模型确实被禁用变灰），而失败日志是 `console.debug`，**控制台默认不可见**。

### 4. 首屏默认模型优化（消除"先显示内置模型"）
- 残留问题：即使写入正常，lobe 在**服务端 agent 配置到达前**（实测 10s 级）仍用代码内置的 `DEFAULT_AGENT_CONFIG` 渲染。
- 实测排除服务端因素：lobehub 首页响应 **7–24ms**、CPU 0.06% —— 那 10s 是客户端多轮请求 × 香港节点网络往返累积。
- 方案（用户确认，并规避其提出的两个风险）：
  - 壳页面在 `bridge-login` 的 `callbackUrl` 附带 **`sub2apiModel` + `sub2apiGroup`**（首个 chat 模型 + 分组序号）；
    **只 2 个短字段，长度固定约 50 字节、与渠道/模型数无关** → 不重蹈生图台 URL 膨胀 414 的覆辙。
  - lobe 桥在 `initSub2ApiBridge`（React 渲染前）**同步读取**并写入 `globalThis.__sub2apiPreferredModel`（纯本地读取、**无网络请求，不影响加载耗时**）。
  - `selectors.ts` 的 `inboxAgentConfig` 在 agent 配置未到达时**优先用它**，而不是内置 `DEFAULT_AGENT_CONFIG`。
- 机制确认：`bridge-login` 由 **lobe 侧**提供（`src/app/(backend)/api/sub2api/bridge-login/route.ts`），把 `callbackUrl` 原样用于**相对 Location** 302 → 参数可安全透传，**后端零改动**。

---

## 四、镜像链与回滚

| 时间 | 镜像 tag | compose 备份 |
|---|---|---|
| 00:54 | `sub2api:v025-20260916`（v0.2.5 合并） | `docker-compose.yml.bak-v025-20260916-0051` |
| 10:44 | `sub2api:i18n-20260916`（提示中文化） | `docker-compose.yml.bak-i18n-20260916-1044` |
| 11:52 | `lobe-chat:bridge-sub2api-v9` | `docker-compose.yml.bak-lobe-v9-20260916-1152` |
| 12:45 | `lobe-chat:bridge-sub2api-v10` | `docker-compose.yml.bak-lobe-v10-20260916-1245` |
| 13:59 | `lobe-chat:bridge-sub2api-v12`（真根因修复） | `docker-compose.yml.bak-lobe-v12-20260916-1359` |
| 14:53（当前） | **`sub2api:pref-20260916`** | `docker-compose.yml.bak-pref-20260916-1453` |
| 15:33（当前） | **`lobe-chat:bridge-sub2api-v13`** | `docker-compose.yml.bak-lobe-v13-20260916-1533` |

### 数据库备份
| 备份 | 路径 | 说明 |
|---|---|---|
| 全量 | `/root/sub2api-full-20260916-0050.dump` | 1.4M，104 张表数据条目 |
| 配额表 | `/root/upq-20260916-0051.sql` | 16K，被 purge 的 91 行 |

### 回滚步骤
```bash
# 主站
cd /root/sub2api-deploy
sed -i 's|image: sub2api:pref-20260916|image: sub2api:urlslim-20260915|' docker-compose.yml
docker compose up -d sub2api
# 对话台
cd /opt/playground
sed -i 's|image: lobe-chat:bridge-sub2api-v13|image: lobe-chat:bridge-sub2api-v12|' docker-compose.yml
docker compose up -d lobehub
# 配额数据如需恢复
# docker exec -i sub2api-postgres psql -U sub2api -d sub2api < /root/upq-20260916-0051.sql
```

### 服务器清理（同日）
- `/tmp`：删除 7 个镜像 tar（约 4.5G）+ 历史诊断文件 → 占用降至 3.7M。
- Docker 镜像：删除 lobe-chat v2–v10、sub2api 早期版本 → **镜像占用 23.15GB → 7.56GB，磁盘 42% → 22%**。
- 保留回滚点：sub2api `pref` / `i18n` / `urlslim`；lobe-chat `v13` / `v12`。

---

## 五、踩坑与经验

1. **`console.debug` 级别的失败日志 ≈ 没有日志。** 默认模型校正失败被 `catch` 吞掉且用 debug 输出，导致连续三轮修复都在错误方向（调延迟）上打转。**排查前先把关键 catch 提升为 `console.warn`**，并引入 `window.__sub2apiDiag` 诊断快照（现已在 `bridge.ts` 中保留）。
2. **iframe 内的 console 日志在 DevTools 默认上下文里看不到。** 对话台跑在主站面板的 iframe 中，Console 默认只显示顶层页面 —— 用户两次抓日志为空即因此，需切 Console 左上角的 frame 选择器。
3. **单测的 A/B 对照必须保证"测试选择方式一致"**：`-count=N` 与 `-count=1`、全量与 `-run 单例` 结论可能相反。本次 `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` 一度被误判为"合并引入的回归"，实测**上游原版在全量跑时同样失败**（上游自带的包级状态污染），与本次合并无关。
4. **npm 镜像滞后会卡死 lobe 构建**：`ERR_PNPM_NO_MATCHING_VERSION @aws-sdk/token-providers@3.1132.0`，npmmirror 上最新仅 3.1131.0。**注意陷阱**：用 `curl registry.npmmirror.com/<pkg>` 查看 versions 列表会显示"包含该版本"（CDN 节点差异），容易误判成网络问题 —— **以 pnpm 实际报错为准**。已在 `lobe-chat/Dockerfile` 增加 `ARG NPM_REGISTRY`（默认值不变），构建时传官方源即可。
5. **SSH 自动化脚本的隐藏坑（已修复）**：凭据文件中密码字段为 **`SSH_PWD`**，而脚本原先只认 `SSH_PASS`/`PASSWORD` 等 → 密码解析为 `None`；此前能连上全靠 paramiko 默认 `look_for_keys=True` **自动使用 `~/.ssh` 私钥**。当沙箱开始拦截 `.ssh` 读取时即报 `Permission denied` / `No authentication methods available`。修复：补上 `SSH_PWD` 解析 + 显式 `look_for_keys=False, allow_agent=False`（纯密码认证）。
6. **并行跑两个 Docker 构建**曾导致其中之一 npm 解析失败，建议重要镜像单独构建。

---

## 六、待办

1. **四仓库共 269 个提交尚未推送到 GitHub**（sub2api 领先 232、lobe-chat 17、生图台 10、画布 10）。
   - 三个工作台仓库已配置 `mine` 远端指向自己的 fork（`kunxiong971/lobe-chat`、`.../gpt_image_playground`、`.../infinite-canvas`）；`origin` 仍是上游原作者仓库，**勿用 origin 推送**。
   - 本机无 GitHub 凭据，需用户手动执行推送（见 `_fork-backup-2026-09-16/README.md`）。
   - 已导出增量 bundle 存档：`e:/github/sub2AI/_fork-backup-2026-09-16/`（共 1.6M）。
2. 建议浏览器侧回归：三工作台注入与出图、渠道监控「图片探活」（与上游同批修复同区域）、平台配额页面。
3. 上游 `channel_monitor` 相关修复（Base URL 路径、自动刷新间隔、UTC 分桶）与本项目图片探活改动位于同一区域，后续合并时留意。

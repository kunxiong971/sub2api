# OPS-2026-09-14 工作台加固四连发部署记录（P0/P1/P2/方案五阶段1）

> 全部由 AI 代办（用户授权免审），逐项审查后部署。当前线上主站镜像：`sub2api:status-live-20260914`。

## 一、部署总览（按时间序）

| # | 内容 | 镜像/产物 | 状态 |
|---|---|---|---|
| P1 | 托管 key 生命周期治理（幂等创建/删除联动/后台管理） | `sub2api:key-gov-20260914` | ✅ 已部署，迁移 245 已应用 |
| P0 | 工作台注入渠道默认填自家 URL + 置灰（生图台+画布） | 静态产物整体替换 | ✅ 已部署 |
| P2 | 模型 kind 分流严格化（InferModelKind 三层防线 + 大小写去重） | `sub2api:p2-kind-20260914` | ✅ 已部署 |
| 方案五-1 | 监控状态实时刷新（90s 周期 + 注入指纹比对 + 生图台提示条） | `sub2api:status-live-20260914` | ✅ 已部署 |
| P3 | lobe 长上下文临界双倍计费提醒（阈值下发 + lobe 输入框提醒） | `sub2api:longctx-20260914` + `lobe-chat:bridge-sub2api-v7` | ✅ 已部署（当前线上） |

## 二、各项要点

### P1 托管 key 治理
- 迁移 `245_playground_managed_key_idempotency.sql`：先清洗存量重复（保留最早一把）再建部分唯一索引 `api_keys_playground_managed_user_group_unique`，容器启动自动执行 ✅（已验证索引存在）
- `EnsurePlaygroundKey`：精确 SQL 查询 + singleflight + 唯一冲突回查；命名统一 `Playground · <分组名>`（100 字符截断）
- 分组删除 `deleteCascade` 第 5 步接 `detachAPIKeysFromGroup`（同事务）：托管 key 软删、自建 key 解绑
- 后台新增：托管徽章/筛选、删除托管 key（`DELETE /admin/api-keys/:id`）、清理孤儿（`POST /admin/api-keys/cleanup-orphans`）、分组删除前托管数量提示
- 换组（`UpdateGroupIDByUserAndGroup`）防撞索引；`AdminUpdateAPIKeyGroupID` 对托管 key 冲突返回 400

### P0 工作台 URL 锁定
- 生图台：`presetConfig.ts` 新增 `registerManagedProfiles`，注入 profile 走 preset 锁定通道（baseUrl 置灰、仅放行 apiKey）；注入会纠正被手改过的地址（防篡改，有单测）
- 画布：渠道加 `managed` 标识 + `sub2api-managed-` id 前缀；编辑抽屉对托管渠道锁定 baseUrl 与协议；历史渠道下次注入自动补标
- 服务端 PGWProxy X-Forwarded-Host 修复**按用户决策降级不做**（风险：24h 令牌→长期 key 升级，消费都扣同一 key，可控）

### P2 kind 严格化
- `backend/internal/service/model_kind.go`：`InferModelKind` 关键词表（唯一事实源，前端镜像实现需同步）
- 三层防线：保存时自动填充（`normalizeModels`）→ 下发兜底（`filterModelsByAllowedKinds` + 应用层补录路径同样过滤）→ 前端展示层兜底
- 模型去重大小写不敏感；管理端「自动识别：X」徽章；`withMonitorStatusTag` 防双标签
- 单测：`playground_config_kind_test.go`（关键词表/空 kind 分流/显式优先/大小写去重/端到端下发）

### 方案五阶段1 状态实时化
- 壳页面 `PlaygroundView.vue`：90s 周期刷新（`document.hidden` 跳过、keep-alive 生命周期管理）
- 指纹比较用「注入产物指纹」（`buildInjectedFingerprint`：分组×apiUrl/apiKey/模型×display_name/price/monitor_status），解决旧整包指纹的跨应用误判与回退模式盲区
- chat/canvas（postMessage）：变化时原地重发（子应用桥指纹幂等，重复无害）
- image（URL 参数）：变化时顶部 amber 提示条「工作台配置已更新，点击刷新生效」（i18n `playground.notice.*`）
- 前端最终防串：`buildInjectedGroup`/回退模式都过 `filterModelsByAllowedKinds`（与后端同口径）
- 效果：监控状态变化最迟约 2 分钟内进入所有已打开的工作台

## 三、镜像链与回滚

| 时间 | 镜像 tag | compose 备份（/root/sub2api-deploy/） |
|---|---|---|
| 4h 前（会话外） | `sub2api:wb-fix-20260914` | — |
| 17:18 | `sub2api:key-gov-20260914` | `docker-compose.yml.bak-keygov-20260914171844` |
| 18:12 | `sub2api:p2-kind-20260914` | `docker-compose.yml.bak-p2kind-20260914181206` |
| 18:31 | `sub2api:status-live-20260914` | `docker-compose.yml.bak-statuslive-20260914183151` |
| 20:11 | `sub2api:longctx-20260914` | `docker-compose.yml.bak-longctx-20260914201150` |
| 21:0x | `sub2api:stage34-20260914` | `docker-compose.yml.bak-stage34-2026…` |
| 21:4x（当前） | `sub2api:hotfix-20260914` | `docker-compose.yml.bak-hotfix-2026…` |

### 用户测试反馈修复（hotfix-20260914）

| 反馈问题 | 根因 | 修复 |
|---|---|---|
| 「迁移到全局」报错 playground group not found | 全局/应用配置存在**指向已删分组的僵尸绑定**（group 5 于 9/13 删除，配置未同步清理），全量保存时被 ErrPlaygroundGroupNotFound 整单拒绝 | ListGlobalConfig / ListApps / ListEnabledApps 加载时自动过滤僵尸绑定（`groupExists` 带缓存；groupRepo 缺省的测试装配不过滤）；下次保存时僵尸行随全量替换自然清出 DB |
| 对话台/生图台出现红色「工作台连接异常」横幅 | 误报：对话台的桥从设计上就不回 ACK（仅画布回），把「未确认」误判为失败 | **撤掉该横幅与失败判定**，恢复原有静默行为；另修正 startPostMessage 只在 postMessage 应用启动（URL 应用不再空跑循环） |
| 生图台黄色「配置已更新，点击刷新」提示，刷新清空客户输入 | URL 注入无法原地更新，提示条设计反而伤害体验（用户决策：移除） | **整个移除该功能**：生图台以打开页面时的配置快照为准；90 秒周期刷新仅服务对话台/画布（无感原地更新保留） |
| 画布渠道未置灰 | 线上 JS 包已验证包含锁定代码（grep 命中 `sub2api-managed-`），属浏览器缓存旧 index.html（nginx 对 js/css 有 12h 缓存） | 无需改码；指导用户 Ctrl+F5 硬刷新。注意：仅注入渠道锁定，自建渠道不锁 |
| 孤儿 key 未清理 | 已于当日 SQL 软删并验证 0 残留 | 无需操作；界面看不到属正常（软删不出现在任何列表） |
| /pgw/v1 地址疑问 | 设计如此：密钥代理模式（真实 key 不下发浏览器，用 24h 短令牌 + 服务端转发） | 保持 |

### 四次修复（fix4-20260915 + lobe v8，用户复测反馈）

| 问题 | 根因 | 处理 |
|---|---|---|
| **对话台发消息后被刷新、丢失会话，严重时收不到模型返回**（阻塞） | 壳页面 90 秒周期刷新重新拉配置时，iframe 地址中的 **lobe 登录票据每次都变** → 响应式重算 `:src` → 浏览器重新加载 iframe → 正在流式返回的回答被中断、会话回到空白 | iframe 地址改为**只在显式重建（render）时固定**（新增 `iframeSrc` ref，周期刷新不再触碰）；周期刷新的无感注入保留。镜像 `sub2api:fix4-20260915` |
| lobe「消息发送中…」文字模糊 | 上游「闪耀文字」用 `background-clip: text` 渐变裁剪 + 遮罩扫光，小字号/高分屏/缩放下天然发虚 | `src/styles/loading.ts` 的 `shinyText` 改为**实色文字 + 轻微呼吸动画**（1.6s 透明度 60%⇄100%），清晰可读且保留"进行中"动感；影响所有使用该样式的 15+ 处状态标签。镜像 `lobe-chat:bridge-sub2api-v8` |

### 三次修复（fix3-20260915，用户复测反馈）

| 问题 | 根因 | 处理 |
|---|---|---|
| **全局配置页签永远空白**（阻塞） | `PlaygroundAdminView` 的 `activeBindings` 在 global 页签下写死返回 `[]`（既有 bug），数据其实一直在库里（2 条绑定 / 8 个模型） | 改为返回 `forms.global`，添加分组/拉取模型/保存/删除恢复正常 |
| 标签页标题是英文 | 三条路由的 `meta.title` 本身是英文（AI Chat / Image Studio / Infinite Canvas），且项目有统一标题机制（router/title.ts + 路由守卫 + App.vue）会覆盖手动设置的 document.title | 改走官方机制：路由标题改为 对话 / 生图工作台 / 无限画布；**移除**手动写 document.title 的代码（避免冲突）；生图台自身 index.html 标题改为「生图工作台」（画布本已中文） |
| 迁移到全局存在并发覆盖风险 | 连续迁移两个绑定时，两次全量替换保存可能互相覆盖（后落库覆盖前一次合并结果） | 迁移进行中禁用**全部**迁移按钮 |
| 独立审计 | code-explorer 子代理连续 3 次调用返回异常，未能完成；改为主会话人工审计 | 已修上述 3 项；核实安全项：僵尸绑定过滤对禁用分组安全/软删生效/恢复可用、SelfCheck 不含密钥、阈值富化按分组缓存无 N+1、周期刷新定时器无泄漏、已删功能无残留死代码与失效文案、i18n 中英完整。**建议后续再安排一次独立复核** |

### 二次修复（hotfix2-20260914，用户复测反馈）

| 问题 | 根因 | 处理 |
|---|---|---|
| 画布整站打不开 | nginx vhost 缓存规则正则写错（`.*\\.(js|css)?$` 双反斜杠失效）→ index.html 被启发式缓存，静态资源更新后旧页面引用已删 hash 资源 404 | 重写缓存规则：`js/css` 7 天长缓存（内容 hash 安全）+ **index.html no-cache**；生图台（image-app conf）同步加 index.html/sw.js no-cache；两站已验证响应头生效。**此类"更新后打不开/旧页面"问题根治** |
| 「迁移到全局」成功但全局看不到变化 | 属正常：应用层历史绑定自身无独立模型（模型早已全部在全局库），迁移 = 清理空壳绑定 | 迁移提示语区分场景：无东西可迁时提示"该绑定没有独立模型…已清理该历史绑定" |
| 新需求：浏览器标签页标题 | 壳页面未设置 document.title，多开窗口无法分辨 | 壳页面按应用设置标签页标题（对话 / 生图工作台 / 无限画布），离开页面恢复原标题 |
| 补充发现 | 应用层配置此时仅剩 chat/分组2 一个空绑定（0 模型），其余历史绑定已被用户迁移清理 | — |

lobe 镜像链（/opt/playground/docker-compose.yml 的 lobehub 服务）：`bridge-sub2api-v6` → **`bridge-sub2api-v7`（当前）**，备份 `docker-compose.yml.bak-lobe-v7-20260914202548`。

## 补充部署（同日晚间）：方案五阶段 3/4 + 画布修复 + 孤儿 key 清理

| 内容 | 说明 |
|---|---|
| 孤儿托管 key 清理 | 4 把已软删（与新后台清理接口同逻辑），剩余 8 把正常托管 key |
| 画布两个既有类型错误修复 | `sub2api-bridge.ts` 旧格式 models 归一化 + `model-script-editor.tsx` 移除 antd Modal 不存在的 `content` styles 键（样式由 wrapClassName 工具类承担）；tsc 错误清零；画布已重建部署 |
| 方案五阶段 3 | 管理后台工作台配置页：chat/image/canvas 页签转为「历史补录（只读）」+ 每绑定「迁移到全局」按钮（同分组合并、大小写不敏感去重、迁移后清空应用层清单并双保存）；仅全局页签可编辑保存 |
| 方案五阶段 4 | ① 后端新接口 `GET /admin/playground/self-check`（复用 ListEnabledApps 运行时组装，返回三应用实际注入清单，不含密钥）+ 管理页「注入自检」按钮与弹窗（按应用/分组展示模型徽章：监控状态色点、类型、长上下文阈值）；② 壳页面 postMessage 注入重试超限仍未 ACK → 显示「工作台连接异常」红条 + 重试按钮（原来静默失败） |
| 本地中间产物清理 | 删除 49+4 个文件（镜像 tar、构建日志、历史一次性 diag/fix/deploy 脚本、临时文本），仅保留 build_*.cmd / lobe_build_v*.cmd / upload_lobe_v5.ps1 构建脚本与连接说明文档 |

回滚：改回 compose 中 `image: sub2api:<旧tag>` → `docker compose up -d sub2api`。迁移 245 幂等且只增不改语义，回滚旧镜像无需回滚 DB。

工作台备份（服务器 /root/）：`image-app-backup-p0-20260914174427.tar.gz`、`canvas-backup-p0-20260914174427.tar.gz`
回滚工作台：解压备份到 `/var/www/image-app`（生图台）/ `/www/wwwroot/canvas.ai.1canc.com`（画布）。

## 四、验证记录
- 后端：`go build ./...` 通过；service 包全量单测通过（两轮，185s）；handler/admin、server contract 通过
- 仓库 `backup_pg_dumper_test.go` 3 个失败为 Windows 环境性（用例依赖 `sh`），与改动无关，Linux CI 通过
- 前端：构建机构建含 vue-tsc 与 i18n 完整性检查，三轮全部通过；生图台 `tsc -b` + vitest 43/43 通过
- 画布 tsc 与基线对比零新增错误；生图台/画布线上 200、新资源 hash 已生效、`config.js` 完好

## 五、P3 lobe 长上下文临界提醒（实施详情）

**效果**：用户在 lobe 对话时，当前会话「将发送完整上下文」估算达到该模型长上下文计费阈值的 95% 时，输入框上方出现黄色提醒「当前对话已约 N 万 tokens，继续发送将进入长上下文计费区间（单价跳档），建议开启新会话」+「开启新会话」按钮（归档当前对话为新 topic 并切换）。

**数据链路**（与计费同源、轻量路径、不跑计费探针）：
1. `PlaygroundConfigService` 新增可选 `pricingResolver`（wire_gen 传入 `modelPricingResolver`），下发时对 kind=chat 模型富化三字段：`long_context_pricing_enabled` / `long_context_threshold`（首次跳档阈值）/ `long_context_threshold_inclusive`。分组未启用长上下文、非 token 计费、无阈值 → 字段零值（前端不提醒）。渠道区间（多档）取首个 `MinTokens>0` 边界、inclusive=true（较真实跳档早 1 token，宁早勿晚）。**阈值是运行时富化字段，不落库**（随定价配置变）。
2. 壳页面 `buildInjectedGroup` 透传三字段 → `PlaygroundInjectedModel`。
3. lobe 桥：normalize 白名单放行三字段 + 纳入指纹；`persist` 时派发 `sub2api:playground-config-updated` 事件供 React 侧订阅。
4. lobe UI：`useLongContextNotice`（复用 `useTokenBreakdown` 的 300ms debounce 节奏）× `useSub2ApiLongContextConfig`（provider 槽位×model_id 匹配）→ `useChatInputNotice` 最低优先级档位 → `ChatInputNotice` 渲染 warning + newTopic 按钮（`createTopic()` + `switchTopic()`）。
5. i18n：`chat` namespace `input.longContextThreshold(.action)`，zh-CN/en-US 已配。

**关键文件**：backend `playground_config.go`（enrichLongContextPricing）、`playground_handler.go`、`wire_gen.go`；lobe `src/sub2api/bridge.ts`、`src/sub2api/useLongContextConfig.ts`、`src/features/ChatInput/ChatInputNotice/useLongContextNotice.ts`、`useChatInputNotice.ts`、`index.tsx`。
**测试**：`playground_config_longctx_test.go`（富化/分组关闭/缺 resolver 容错）；lobe `useChatInputNotice.test.tsx` 增两用例（提醒展示、优先级让位）。service 全量单测通过；lobe v7 镜像构建通过（Next.js 16.3.5 启动正常）。

## 六、遗留事项（下个会话按既成提示词执行）

1. **方案五阶段 3/4**（配置双轨 UI 收敛 + 注入自检面板）：用《提示词 5》阶段 3、4 部分。
2. 小项：`GetManagedByUserAndGroup` 可补 `status=active` 过滤（当前无禁用路径，低风险）；画布两个**既有**类型错误（`sub2api-bridge.ts:283` models 类型、`model-script-editor.tsx:58`）可顺手修。
3. **4 把历史孤儿托管 key**：管理后台 → 用户 API 密钥 →「清理孤儿托管密钥」一键清理（上会话清理 SQL 被审批门拦下）。
4. 本地 `E:\github\sub2AI\` 下遗留中间产物可清理：`sub2api-*.tar`、`lobe-v7.tar`、`pg-dist.tar.gz`、`canvas-dist*.tar.gz`、各 `build-*.log`、`lobe-build-v7.log`、审查临时文件，以及历史会话的 `diag-*.sh`/`fix-*.sh`/`deploy-*.sh` 等。

# 运维记录 — 图片模型监控开关（image_mode）上线（2026-09-13）

> 需求：渠道监控此前只能探活文本类模型（判定要求响应文本非空），生图模型（gpt-image 系，Responses API）的响应只有 `image_generation_call` 无文本，永远被判 `failed`，无法监控图片渠道连通性。
> 方案：`channel_monitors` 新增 `image_mode` 布尔开关（默认 false = 原文本判定，零行为变化）；开启后 replace 模式按「2xx + 响应体非空」判定。

## 一、改动清单

| 层 | 内容 |
|---|---|
| 迁移 | `243_channel_monitor_image_mode.sql`（`ADD COLUMN IF NOT EXISTS image_mode BOOLEAN NOT NULL DEFAULT FALSE`，幂等；启动时自动应用） |
| ent | `schema/channel_monitor.go` 加 `image_mode` 字段 + 代码重新生成 |
| checker | `channel_monitor_checker.go` replace 分支按 `image_mode` 分流；replace body 校验放宽（image_mode 下允许只有 `input` 的纯生图 body，默认仍要求 instructions+input） |
| 模板 | 请求模板 body 校验放宽为「或」口径（否则只有 input 的生图模板无法保存），运行时仍按监控自身 image_mode 判定 |
| 前端 | 监控表单「附加模型」下方新增「图片模型监控」开关（zh/en 文案），编辑回填/提交接通；API 类型加 `image_mode` |

测试：service/handler/repository 单测通过；含「默认模式行为不变」「image_mode 下纯图片响应 operational」「0 字节响应两模式均 failed」等断言。

## 二、部署记录

| 项 | 值 |
|---|---|
| 新镜像 | `sub2api:img-monitor-20260913`（sha256 13e374ae…） |
| 上一镜像 | `sub2api:gc-ui-20260913`（保留回滚） |
| compose 备份 | `/root/sub2api-deploy/docker-compose.yml.bak-img-monitor-20260913` |
| 构建 | 局域网构建机（192.168.3.31:2375）构建 → save 132MB → scp → load → compose 切换 → 容器 healthy |
| 迁移验证 | `channel_monitors.image_mode` 列已存在（默认 false） |

**回滚**：compose 恢复备份后 `docker compose up -d sub2api`（image_mode 列保留无害）。

## 三、构建踩坑（留档）

1. `pnpm-lock.yaml` 被本地 pnpm 10 重写（丢失 `overrides` 段）→ 构建报 `ERR_PNPM_LOCKFILE_CONFIG_MISMATCH`；git checkout 还原即可
2. 本地 pnpm 10 还生成了残缺占位文件 `frontend/pnpm-workspace.yaml`（内容为未填模板字面量）→ 拷入镜像后 pnpm 9 报 `packages field missing or empty`；该文件无作用（esbuild/vue-demi 构建脚本放行已在 package.json，commit 7e7b2e652），已删除
3. 本地 Go ent 生成需 `GOPROXY=https://goproxy.cn,direct`（默认 proxy.golang.org 被墙）

## 四、使用方法（最终版 — 09-14 更新）

1. 管理后台 → 渠道监控 → 编辑/新建监控：`OpenAI` + **API 模式选「Images API」** + 图片模型名 + **直连上游 endpoint/key**
2. Body 模式用「默认」（不需要手写 body；适配器自动发 `/v1/images/generations` + model/prompt/n/size）
3. 保存 → 「立即检测」→ 🟢（40~70 秒出结果，生图天然慢，不会误报降级）
4. 间隔建议 600~3600s（每次检测真实生成一张图，有费用）；「工作台配置」关联监控后用户端显示状态标签

---

## 五、09-14 追加：Images 探活模式 + 约束修复 + 超时放宽

### 背景（forkc2p 实测结论）

- 上游 `https://api.forkc2p.com` 支持模型：`gpt-image-2`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`
- **只支持 Images API**：`/v1/images/generations` ✅ 200（36~70s 出图）、`/v1/images/edits` ✅ 200（图生图，multipart）；
  `/v1/responses` + image_generation 工具 ❌ 503（上游不支持）→ 之前的监控走 responses 路径必然失败
- 上游存在**偶发快速失败**：`400 upstream_text_reply`（9.6s 返回，上游自身抖动，文案 "Please try again"）；同一配置复测 200/70s 成功

### 代码改动

| 层 | 内容 |
|---|---|
| checker | 新增 `api_mode=images`（`providerOpenAIImagesAdapter`）：POST `/v1/images/generations`，默认 body `{model,prompt,n:1,size:1024x1024}`；判定「2xx + data[0].url/b64_json 非空」，跳过 challenge；merge 保护字段 model/prompt/n/size；replace 模式要求 prompt 非空 |
| 超时 | Images 探活专用 `monitorImageRequestTimeout=180s`（独立 HTTP client）；runner 外层 ctx 预算同步放宽；文本监控行为不变 |
| 迁移 | `244_channel_monitor_api_mode_images.sql`：重建 `channel_monitors_api_mode_check` CHECK 约束放行 `images`（原约束只允许 chat_completions/responses，保存报 500 的根因） |
| handler/前端 | api_mode 选项加第三个「Images API」（zh/en 文案、body 占位示例） |

### 部署记录

| 项 | 值 |
|---|---|
| 最新镜像 | `sub2api:img-timeout-20260914`（`img-monitor-20260913` → `img-probe-20260913` → 本版） |
| compose 备份 | `.bak-img-monitor-20260913` / `.bak-img-probe-20260913` / `.bak-img-timeout-20260914` |
| 约束修复 | 已在服务器直接执行（幂等），244 迁移文件已入库 |

### 站点侧超时结论（无需调整）

- OpenAI 账号的上游等待响应头超时 = **0（无超时）**（`gateway.openai_response_header_timeout` 默认 0，OpenAI profile 不截断）→ 70s 生图经网关实测 200 ✅
- 下游客户端若自带 60s 级超时，需要自行调长（生图 40~70s 属正常）

### 建议

- forkc2p 偶发抖动会直接透传给下游（分组 2 仅 1 个账号、无 failover 冗余）；如需高可用建议加第二个生图上游账号
- 客户端遇 `upstream_text_reply` 直接重试即可（上游为瞬时故障）

---

## 六、09-14 追加二：监控最终修复 + 工作台/画布生图修复

### 监控失败根因链（逐层实测确认，全部已修）

1. 探活 body 用 challenge 算术题当生图 prompt → 上游模型回文字（`upstream_text_reply` 400）→ 改固定生图 prompt `monitorImagesProbePrompt`
2. 30s「等待响应头」超时（`monitorResponseHeaderTimeout`）掐断 45~70s 的生图 → Images 探活专用 client 双超时放宽到 180s
3. 结果：**12:01:37 检测 operational 🟢**

### 工作台/画布"生不出图"根因与修复

- **根因 A（分组）**：生图台/画布默认激活「列表第一个分组」= ZN 文本组（无图片模型）→ 模型选择器里没有图片模型。修复：`PlaygroundView.vue` 新增 `pickPreferredActiveGroupIndex`——image 台优先激活含 `model_kind=image` 的分组（profiles 全量清单不变，仍可手动切换）
- **根因 B（图片加载）**：画布响应为图片 URL（`gateway.change2pro.com` 外部域名），浏览器侧加载失败。修复：账号 1 开启 `images_url_to_b64_json`（网关把图片下载转 base64 内嵌返回，不再依赖外部域名）
- 证据：画布 11:47~11:48 三次请求服务器全部 200（377B URL 响应）；工作台 11:46 一次 200/2MB（b64）；服务器侧图片 URL 可下载（551KB）

### 部署记录（09-14 全天迭代）

| 镜像 | 内容 |
|---|---|
| `img-monitor-20260913` | image_mode 开关 + 迁移 243 |
| `img-probe-20260913` | Images API 探活模式 + 迁移 244（api_mode CHECK 约束放行 images） |
| `img-timeout-20260914` | Images 探活 180s 超时 |
| `img-prompt-20260914` | 固定生图 prompt |
| `img-hdr-20260914` | 响应头超时放宽（监控最终修复） |
| `wb-fix-20260914` | 生图台/画布默认激活图片分组 |
| DB | 账号 1 `images_url_to_b64_json=true` |

（compose 各阶段备份均在 `/root/sub2api-deploy/docker-compose.yml.bak-*`）

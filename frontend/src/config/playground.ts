/**
 * Playground（集成工作台）应用配置
 *
 * 当前部署形态（ai.1canc.com 实例，子域名方案，宝塔 nginx 反代）：
 *   chat.ai.1canc.com   → lobehub（fork，支持 postMessage 注入）   反代 127.0.0.1:3210
 *   draw.ai.1canc.com   → gpt_image_playground（URL 参数注入）     反代 127.0.0.1:3002
 *   canvas.ai.1canc.com → infinite-canvas（postMessage 注入）     静态托管 /www/wwwroot/canvas.ai.1canc.com
 *
 * 修改下方 DEPLOY_BASES 后需重新构建前端（产物进 backend/internal/web/dist）并重新编译带 -tags embed 的后端；
 * 也可通过 localStorage（键: playground_base_<app>）或 URL ?appBase= 临时覆盖。
 */

export type PlaygroundAppKey = 'chat' | 'image' | 'canvas'

/** 各工作台的部署地址（管理员二开时修改这里） */
const DEPLOY_BASES: Record<PlaygroundAppKey, string> = {
  chat: 'https://chat.ai.1canc.com',
  image: 'https://draw.ai.1canc.com',
  canvas: 'https://canvas.ai.1canc.com'
}

export interface PlaygroundAppMeta {
  /** 侧边栏/页面标题 */
  title: string
  /** key 注入方式：url = 拼查询参数；postMessage = iframe 加载后循环发消息 */
  injectMode: 'url' | 'postMessage'
  /**
   * 是否走 /pgw 会话代理（服务端注入真实 key，浏览器只持有 24h 短时令牌）。
   * image/canvas 的配置存浏览器本地，默认开启代理隐藏真实 key；
   * lobe 的配置存其服务端数据库（加密），保持真实 key 以免令牌过期导致对话中断。
   */
  usePgwProxy: boolean
  /** 是否通过免登录桥自动建立子应用会话（lobe 专用：bridge-login 票据跳转） */
  autoLogin?: boolean
}

export const PLAYGROUND_APP_CONFIG: Record<PlaygroundAppKey, PlaygroundAppMeta> = {
  chat: {
    title: 'AI 对话',
    injectMode: 'postMessage',
    usePgwProxy: false,
    autoLogin: true
  },
  image: {
    title: '生图工作台',
    injectMode: 'url',
    usePgwProxy: true
  },
  canvas: {
    title: '无限画布',
    injectMode: 'postMessage',
    usePgwProxy: true
  }
}

/** 读取某应用的实际部署地址：URL ?appBase= > localStorage > 管理员配置的默认值 */
export function resolvePlaygroundBase(app: PlaygroundAppKey): string {
  if (typeof window !== 'undefined') {
    const fromQuery = new URLSearchParams(window.location.search).get('appBase')
    if (fromQuery) return fromQuery
    try {
      const stored = localStorage.getItem(`playground_base_${app}`)
      if (stored) return stored
    } catch {
      // localStorage 不可用时忽略
    }
  }
  return DEPLOY_BASES[app]
}

/** 确保地址以 / 结尾（同时兼容完整 URL 与相对路径） */
export function normalizePlaygroundBase(base: string): string {
  const trimmed = base.trim().replace(/\/+$/, '')
  return `${trimmed}/`
}

/** postMessage 消息类型（lobe-chat / infinite-canvas fork 侧约定） */
export const PLAYGROUND_CONFIG_MESSAGE_TYPE = 'sub2api:playground-config'

/** 子应用成功写入配置后回发的 ACK 消息类型 */
export const PLAYGROUND_CONFIG_ACK_TYPE = 'sub2api:playground-config-ack'

/** 宿主 → 子应用的主题同步消息 */
export const PLAYGROUND_THEME_MESSAGE_TYPE = 'sub2api:theme'

/** 注入给工作台的单个模型（含展示元数据） */
export interface PlaygroundInjectedModel {
  model_id: string
  display_name?: string
  price_label?: string
  unit_hint?: string
  description?: string
  /** 模型类型标记：chat/image/video/audio；空串 = 按模型名自动推断 */
  model_kind?: string
  /**
   * 渠道监控最近检测状态（operational/degraded/failed/error）。
   * 空串/缺省 = 未关联监控或无检测数据，工作台不展示状态标签。
   */
  monitor_status?: string
  /**
   * 长上下文计费提醒（模型级，「分组×模型」粒度；阈值随定价配置由宿主后端解析下发）。
   * 透传给 lobe 桥，用于输入框临界双倍计费提醒。
   */
  long_context_pricing_enabled?: boolean
  /** 首次跳档的上下文 token 阈值（0/缺省 = 无） */
  long_context_threshold?: number
  /** true = 达到阈值即跳档（xAI 口径）；false = 严格大于 */
  long_context_threshold_inclusive?: boolean
}

/**
 * fork: 模型类型关键词表（与后端 service.InferModelKind 的 modelKindKeywordTable 保持一致）。
 * 后端是唯一事实源：保存时自动填充、下发时兜底推断都走后端；此处仅作前端兜底
 * （管理端「自动识别」徽章、展示层过滤等）。修改任一侧时必须同步另一侧。
 */
const MODEL_KIND_KEYWORDS: Array<{ kind: string; keywords: string[] }> = [
  {
    kind: 'image',
    keywords: [
      'image', 'gpt-image', 'dall-e', 'dall_e', 'flux',
      'stable-diffusion', 'sdxl', 'midjourney', 'mj-'
    ]
  },
  {
    kind: 'video',
    keywords: ['sora', 'veo', 'kling', 'runway', 'wanx', 'hunyuan-video', 'seedance']
  },
  {
    kind: 'audio',
    keywords: ['tts', 'whisper', 'speech', 'audio', 'voice', 'suno']
  }
]

/** 按模型名关键词推断模型类型（大小写不敏感，未命中返回 chat）；与后端 InferModelKind 口径一致 */
export function inferModelKind(modelId: string): string {
  const name = (modelId ?? '').trim().toLowerCase()
  if (!name) return 'chat'
  for (const group of MODEL_KIND_KEYWORDS) {
    if (group.keywords.some((kw) => name.includes(kw))) return group.kind
  }
  return 'chat'
}

/**
 * fork: 各工作台允许注入的模型类型（与后端 service.playgroundAppAllowedKinds 保持一致）：
 *   - chat（对话）   → chat
 *   - image（生图）  → chat + image（Agent 模式需要 LLM）
 *   - canvas（画布） → chat + image（文本节点需要 LLM）
 * 后端下发前已按此口径分流；此处作为前端最终防串兜底，防止旧缓存/异常数据绕过分流。
 */
const ALLOWED_KINDS_BY_APP: Record<PlaygroundAppKey, string[]> = {
  chat: ['chat'],
  image: ['chat', 'image'],
  canvas: ['chat', 'image']
}

/**
 * fork: 按应用口径过滤待注入模型（与后端 filterModelsByAllowedKinds 同名同义）。
 * 空 kind 的存量模型先走 inferModelKind 按模型名推断再过滤（显式指定的 kind 优先），
 * 保证空 kind 的图片模型不会混进对话台注入清单；过滤保持原顺序。
 */
export function filterModelsByAllowedKinds<T extends { model_id: string; model_kind?: string }>(
  app: PlaygroundAppKey,
  models: T[]
): T[] {
  const allowed = ALLOWED_KINDS_BY_APP[app]
  return models.filter((m) => {
    const kind = (m.model_kind ?? '').trim().toLowerCase() || inferModelKind(m.model_id)
    return allowed.includes(kind)
  })
}

/** 渠道监控状态 → 展示标签（彩色圆点 + 短文案，附加在模型名后） */
const MONITOR_STATUS_TAG: Record<string, string> = {
  operational: '🟢 可用',
  degraded: '🟡 降级',
  failed: '🔴 异常',
  error: '🔴 异常'
}

/**
 * 根据监控状态返回附加在模型名后的状态标签；无状态返回空串。
 * 三个工作台（对话/生图/画布）都渲染 display_name，因此把标签 bake 进
 * display_name 即可在所有模型展示位（选择器/顶栏/列表）统一出现。
 */
export function monitorStatusTag(status?: string): string {
  if (!status) return ''
  return MONITOR_STATUS_TAG[status] ?? ''
}

/**
 * 拼接展示名与状态标签，供注入时统一使用。
 */
export function withMonitorStatusTag(displayName?: string, status?: string): string {
  const tag = monitorStatusTag(status)
  const base = (displayName ?? '').trim()
  if (!tag) return base
  // fork: 防双标签——展示名里已含同款状态标签（如历史数据曾手工拼入）时不再重复追加
  if (base.includes(tag)) return base
  return base ? `${base} ${tag}` : tag
}

/** 注入给工作台的单个分组/渠道 */
export interface PlaygroundInjectedGroup {
  groupName: string
  /** 网关 OpenAI 兼容端点，如 https://api.example.com/v1 */
  apiUrl: string
  apiKey: string
  models: PlaygroundInjectedModel[]
}

export interface PlaygroundInjectedConfig {
  app: PlaygroundAppKey
  /** 兼容旧单分组字段（取第一个分组） */
  apiUrl: string
  apiKey: string
  groupName: string
  models: string[]
  /** 多分组注入（多渠道），工作台据此添加多个渠道 */
  groups?: PlaygroundInjectedGroup[]
}

/**
 * fork: 计算注入产物的内容指纹，供壳页面周期刷新时比较「实际注入内容」是否变化。
 * 指纹覆盖 apiUrl/apiKey/分组名/模型清单（含 display_name/price_label/monitor_status
 * 等展示元数据）——监控状态标签 bake 在 display_name 里，监控变红必然引起指纹变化；
 * 顶层 models（兼容旧单分组字段）也纳入。不含 lobe_ticket 等短时票据。
 * 与子应用侧（lobe/画布桥）的指纹幂等同构：内容不变时重复注入会被子应用短路。
 */
export function buildInjectedFingerprint(cfg: PlaygroundInjectedConfig | null): string {
  if (!cfg) return ''
  const groups = (cfg.groups ?? [])
    .map((g) => {
      const models = g.models
        .map(
          (m) =>
            `${m.model_id}@${m.display_name ?? ''}@${m.price_label ?? ''}@${m.unit_hint ?? ''}@${m.description ?? ''}@${m.model_kind ?? ''}@${m.monitor_status ?? ''}`
        )
        .join('|')
      return `${g.groupName}@${g.apiUrl}@${g.apiKey}[${models}]`
    })
    .join(';')
  return [cfg.app, cfg.apiUrl, cfg.apiKey, cfg.groupName, cfg.models.join(','), groups].join('#')
}

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
  /**
   * 渠道监控最近检测状态（operational/degraded/failed/error）。
   * 空串/缺省 = 未关联监控或无检测数据，工作台不展示状态标签。
   */
  monitor_status?: string
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

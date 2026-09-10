/**
 * Playground（集成工作台）应用配置
 *
 * 部署形态（子域名方案，由 Caddy 反代）：
 *   chat.<域名>   → lobehub/lobehub（fork，支持 postMessage 注入；v2 不支持子路径挂载）
 *   image.<域名>  → CookSleep/gpt_image_playground（原生 URL 参数注入，静态托管）
 *   canvas.<域名> → basketikun/infinite-canvas（构建时 VITE_BASE 支持子路径，也可子域名）
 *
 * 管理员二开时修改下方 DEPLOY_BASES 为自己的实际地址后重新构建前端；
 * 也可通过 localStorage（键: playground_base_<app>）或 URL ?appBase= 临时覆盖。
 */

export type PlaygroundAppKey = 'chat' | 'image' | 'canvas'

/** 各工作台的部署地址（管理员二开时修改这里） */
const DEPLOY_BASES: Record<PlaygroundAppKey, string> = {
  chat: 'https://chat.example.com',
  image: 'https://image.example.com',
  canvas: 'https://canvas.example.com'
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
}

export const PLAYGROUND_APP_CONFIG: Record<PlaygroundAppKey, PlaygroundAppMeta> = {
  chat: {
    title: 'AI 对话',
    injectMode: 'postMessage',
    usePgwProxy: false
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

export interface PlaygroundInjectedConfig {
  app: PlaygroundAppKey
  /** 网关 OpenAI 兼容端点，如 https://api.example.com/v1 */
  apiUrl: string
  apiKey: string
  groupName: string
  models: string[]
}

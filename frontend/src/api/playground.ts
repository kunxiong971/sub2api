/**
 * Playground integration endpoints
 * 集成工作台（对话/生图/画布）配置：用户可用分组 + 每组一把专用 key
 */

import { apiClient } from './client'

export interface PlaygroundGroupConfig {
  id: number
  name: string
  platform: string
  key: string
  /** 分组模型白名单（未启用白名单时为空数组，表示使用平台默认模型列表） */
  models: string[]
  /** /pgw 会话代理短时令牌（24h），配合 pgw_base_url 使用 */
  pgw_token: string
}

/** 管理员配置的模型展示信息（display_name / price_label 纯展示，不参与计费） */
export interface PlaygroundAppModelInfo {
  model_id: string
  display_name: string
  price_label: string
  unit_hint: string
  description: string
  sort_order: number
  /** 模型类型标记：chat/image/video/audio；空串/缺省 = 按模型名自动推断 */
  model_kind?: string
  /**
   * 关联渠道监控的最近检测状态（下发时快照）：
   * operational / degraded / failed / error；空串/缺省 = 未关联或无检测数据
   */
  monitor_status?: string
}

/** 单个应用下某个绑定分组的注入配置 */
export interface PlaygroundAppGroupConfig {
  group: PlaygroundGroupConfig
  models: PlaygroundAppModelInfo[]
}

/** 管理员为单个应用配置的注入规则（只含已启用且已绑定分组的应用，可绑定多个分组） */
export interface PlaygroundAppConfig {
  app: 'chat' | 'image' | 'canvas'
  inject_mode: string
  groups: PlaygroundAppGroupConfig[]
}

export interface PlaygroundConfig {
  /** 站点网关根地址（不含路径），如 https://api.example.com */
  gateway_base_url: string
  /** /pgw 会话代理根地址，如 https://api.example.com/pgw/v1 */
  pgw_base_url: string
  /** lobe 免登录桥票据（typ=lobe，5min 有效） */
  lobe_ticket: string
  groups: PlaygroundGroupConfig[]
  /**
   * 管理员在「工作台配置」中下发的注入规则。
   * 为空数组表示管理员尚未配置，此时壳页面回退到 groups 行为。
   */
  apps: PlaygroundAppConfig[]
}

/**
 * Get playground config for current user
 * 后端会为每个可用分组自动创建（或复用）一把绑定该分组的 key
 */
export async function getPlaygroundConfig(options?: {
  signal?: AbortSignal
}): Promise<PlaygroundConfig> {
  const { data } = await apiClient.get<PlaygroundConfig>('/keys/playground-config', {
    signal: options?.signal
  })
  return data
}

export const playgroundAPI = {
  getPlaygroundConfig
}

export default playgroundAPI

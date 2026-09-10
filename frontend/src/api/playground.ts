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
}

export interface PlaygroundConfig {
  /** 站点网关根地址（不含路径），如 https://api.example.com */
  gateway_base_url: string
  groups: PlaygroundGroupConfig[]
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

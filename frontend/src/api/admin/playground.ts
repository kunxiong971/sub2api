/**
 * Admin Playground API endpoints
 * 工作台「注入配置」：应用（对话/生图/画布）→ 多个分组（多渠道）→ 每组模型展示清单
 *
 * 这里配置的都是策略规则，不含任何真实密钥；密钥在运行时由用户端接口
 * 按「该用户在该分组下自己的 key（或 pgw 短令牌）」下发。
 */

import { apiClient } from '../client'

export interface PlaygroundAppModelConfig {
  id: number
  app_config_id: number
  model_id: string
  display_name: string
  price_label: string
  unit_hint: string
  description: string
  enabled: boolean
  sort_order: number
  /** 模型类型标记：chat/image/video/audio；空=按模型名自动推断 */
  model_kind: string
  /** 可选关联的渠道监控（用于下发模型状态标签） */
  monitor_id: number | null
  created_at: string
  updated_at: string
}

export interface PlaygroundBindingView {
  group_id: number
  group_name: string
  group_platform: string
  enabled: boolean
  models: PlaygroundAppModelConfig[]
}

export interface PlaygroundAppBundle {
  app: string
  inject_mode: string
  bindings: PlaygroundBindingView[]
}

export interface PlaygroundAppModelInput {
  model_id: string
  display_name?: string
  price_label?: string
  unit_hint?: string
  description?: string
  enabled?: boolean
  sort_order?: number
  /** 模型类型标记：chat/image/video/audio；空=自动推断 */
  model_kind?: string
  /** 可选关联的渠道监控 ID */
  monitor_id?: number | null
}

export interface PlaygroundBindingInput {
  group_id: number
  enabled: boolean
  models: PlaygroundAppModelInput[]
}

/** 全局模型库条目：模型展示信息只维护一份，各工作台按类型自动注入 */
export interface PlaygroundGlobalModelConfig {
  id: number
  model_id: string
  display_name: string
  price_label: string
  unit_hint: string
  description: string
  model_kind: string
  enabled: boolean
  sort_order: number
  monitor_id: number | null
  created_at: string
  updated_at: string
}

export interface PlaygroundGlobalModelInput {
  model_id: string
  display_name?: string
  price_label?: string
  unit_hint?: string
  description?: string
  model_kind?: string
  enabled?: boolean
  sort_order?: number
  monitor_id?: number | null
}

/** 拉取三个应用的配置总览（每个应用含绑定的多个分组） */
export async function listConfigs(
  options?: { signal?: AbortSignal }
): Promise<PlaygroundAppBundle[]> {
  const { data } = await apiClient.get<{ apps: PlaygroundAppBundle[] }>(
    '/admin/playground/configs',
    { signal: options?.signal }
  )
  return data?.apps ?? []
}

/** 全量保存某应用的绑定分组与模型清单 */
export async function updateConfig(
  app: string,
  bindings: PlaygroundBindingInput[]
): Promise<PlaygroundAppBundle | null> {
  const { data } = await apiClient.put<{ app: PlaygroundAppBundle | null }>(
    `/admin/playground/configs/${app}`,
    { bindings }
  )
  return data?.app ?? null
}

/** 拉取某分组可配置的模型清单（按分组候选，避免手填） */
export async function fetchModelCandidates(
  app: string,
  groupId: number
): Promise<string[]> {
  const { data } = await apiClient.get<{ models: string[] }>(
    `/admin/playground/configs/${app}/models/candidates`,
    { params: { group_id: groupId } }
  )
  return data?.models ?? []
}

/** 全局模型库清单 */
export async function listGlobalModels(): Promise<PlaygroundGlobalModelConfig[]> {
  const { data } = await apiClient.get<{ models: PlaygroundGlobalModelConfig[] }>(
    '/admin/playground/global-models'
  )
  return data?.models ?? []
}

/** 全量保存全局模型库 */
export async function updateGlobalModels(
  models: PlaygroundGlobalModelInput[]
): Promise<PlaygroundGlobalModelConfig[]> {
  const { data } = await apiClient.put<{ models: PlaygroundGlobalModelConfig[] }>(
    '/admin/playground/global-models',
    { models }
  )
  return data?.models ?? []
}

export const playgroundAdminAPI = {
  listConfigs,
  updateConfig,
  fetchModelCandidates,
  listGlobalModels,
  updateGlobalModels
}

export default playgroundAdminAPI

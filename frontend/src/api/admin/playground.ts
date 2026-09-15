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

/** 全局渠道配置：一个渠道（分组）绑定 + 该渠道下的模型清单，三个工作台按类型自动分流注入 */
export interface PlaygroundGlobalBindingView {
  group_id: number
  group_name: string
  group_platform: string
  enabled: boolean
  models: PlaygroundAppModelConfig[]
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

/** 全局渠道配置总览（渠道 + 模型清单） */
export async function listGlobalConfig(): Promise<PlaygroundGlobalBindingView[]> {
  const { data } = await apiClient.get<{ bindings: PlaygroundGlobalBindingView[] }>(
    '/admin/playground/global-configs'
  )
  return data?.bindings ?? []
}

/** 全量保存全局渠道配置（渠道绑定 + 各渠道模型清单） */
export async function updateGlobalConfig(
  bindings: PlaygroundBindingInput[]
): Promise<PlaygroundGlobalBindingView[]> {
  const { data } = await apiClient.put<{ bindings: PlaygroundGlobalBindingView[] }>(
    '/admin/playground/global-configs',
    { bindings }
  )
  return data?.bindings ?? []
}

/** 拉取全局配置下某渠道的候选模型（避免手填） */
export async function fetchGlobalModelCandidates(groupId: number): Promise<string[]> {
  const { data } = await apiClient.get<{ models: string[] }>(
    '/admin/playground/global-configs/models/candidates',
    { params: { group_id: groupId } }
  )
  return data?.models ?? []
}

/** 注入自检：单模型视图（含运行时富化字段，不含密钥） */
export interface PlaygroundSelfCheckModel {
  model_id: string
  display_name: string
  model_kind: string
  monitor_status: string
  long_context_pricing_enabled: boolean
  long_context_threshold?: number
  long_context_threshold_inclusive?: boolean
}

/** 注入自检：单分组视图 */
export interface PlaygroundSelfCheckGroup {
  group_id: number
  group_name: string
  models: PlaygroundSelfCheckModel[]
}

/** 注入自检：单应用视图 */
export interface PlaygroundSelfCheckApp {
  app: string
  groups: PlaygroundSelfCheckGroup[]
}

/** 注入自检：预览三个工作台实际会收到的注入清单（不含密钥） */
export async function selfCheck(): Promise<{ apps: PlaygroundSelfCheckApp[] }> {
  const { data } = await apiClient.get<{ apps: PlaygroundSelfCheckApp[] }>(
    '/admin/playground/self-check'
  )
  return data
}

export const playgroundAdminAPI = {
  listConfigs,
  updateConfig,
  fetchModelCandidates,
  listGlobalConfig,
  updateGlobalConfig,
  fetchGlobalModelCandidates,
  selfCheck
}

export default playgroundAdminAPI

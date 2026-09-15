/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type { ApiKey } from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
}

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<UpdateApiKeyGroupResult> {
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, {
    group_id: groupId === null ? 0 : groupId
  })
  return data
}

/**
 * Delete a workspace-managed API key (Playground · prefix only).
 * The workbench recreates one on the next injection, so this is safe for cleanup.
 * @param id - API Key ID
 */
export async function deleteManagedKey(id: number): Promise<{ id: number; name: string; message: string }> {
  const { data } = await apiClient.delete<{ id: number; name: string; message: string }>(`/admin/api-keys/${id}`)
  return data
}

/**
 * Remove orphan managed API keys (their group is missing or soft-deleted).
 * @returns Number of cleaned keys
 */
export async function cleanupOrphanKeys(): Promise<{ deleted: number }> {
  const { data } = await apiClient.post<{ deleted: number }>('/admin/api-keys/cleanup-orphans')
  return data
}

export const apiKeysAPI = {
  updateApiKeyGroup,
  deleteManagedKey,
  cleanupOrphanKeys
}

export default apiKeysAPI

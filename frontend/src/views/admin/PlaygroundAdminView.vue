<script setup lang="ts">
/**
 * PlaygroundAdminView - 工作台「注入配置」管理页
 *
 * 给「对话 / 生图 / 画布」分别配置注入规则：
 *   每个应用可绑定多个分组（多渠道），每个分组独立启停、独立维护模型清单。
 *
 * 这里配置的都是策略规则，全程没有粘贴密钥的输入框；
 * 运行时才取「客户自己在该分组下的 key（或 /pgw 短令牌）」注入到工作台。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { channelMonitorAPI } from '@/api/admin/channelMonitor'
import type { AdminGroup } from '@/types'
import {
  playgroundAdminAPI,
  type PlaygroundAppBundle,
  type PlaygroundAppModelInput,
  type PlaygroundGlobalBindingView,
  type PlaygroundSelfCheckApp
} from '@/api/admin/playground'
import { extractApiErrorMessage } from '@/utils/apiError'
import { inferModelKind } from '@/config/playground'

const { t } = useI18n()
const appStore = useAppStore()

type AppKey = 'chat' | 'image' | 'canvas'
const APP_KEYS: AppKey[] = ['chat', 'image', 'canvas']

interface ModelRow {
  selected: boolean
  model_id: string
  display_name: string
  price_label: string
  unit_hint: string
  description: string
  enabled: boolean
  sort_order: number
  model_kind: string
  monitor_id: number | null
  /** fork: 来自全局配置的生效模型（各应用页签只读展示，不参与保存） */
  fromGlobal?: boolean
}

interface MonitorOption {
  id: number
  name: string
  group_name: string
  primary_model: string
}

interface BindingRow {
  group_id: number
  enabled: boolean
  models: ModelRow[]
}

/** 页签：全局配置（一处维护渠道 + 模型，默认打开）+ 三个应用补录页签 */
type TabKey = AppKey | 'global'
const activeTab = ref<TabKey>('global')
const loading = ref(false)
const groups = ref<AdminGroup[]>([])
const monitors = ref<MonitorOption[]>([])
const forms = reactive<Record<TabKey, BindingRow[]>>({ chat: [], image: [], canvas: [], global: [] })
const pendingGroupId = ref<number | null>(null)
const saving = ref(false)
const fetchingGroup = ref<number | null>(null)
// fork(方案五阶段3/4): 应用页签转为「历史补录（只读）」+ 迁移到全局；注入自检面板
const migratingGroup = ref<number | null>(null)
const selfCheckOpen = ref(false)
const selfCheckLoading = ref(false)
const selfCheckData = ref<PlaygroundSelfCheckApp[] | null>(null)

/** fork: 应用页签（chat/image/canvas）为历史补录数据，整体只读 */
const legacyTab = computed(() => activeTab.value !== 'global')

const appLabel = (app: string): string => {
  const map: Record<string, string> = {
    chat: t('admin.playground.tabs.chat'),
    image: t('admin.playground.tabs.image'),
    canvas: t('admin.playground.tabs.canvas')
  }
  return map[app] ?? app
}

/** fork(方案五阶段3): 把应用层的历史补录绑定并入全局配置（同分组合并、大小写不敏感去重），
 * 然后清空该应用层绑定（下发逻辑保持兼容期双读，此后数据只在全局库一份）。 */
async function migrateBindingToGlobal(app: AppKey, binding: BindingRow) {
  migratingGroup.value = binding.group_id
  try {
    const legacyModels = binding.models.filter((m) => !m.fromGlobal)
    const globalBinding = forms.global.find((b) => b.group_id === binding.group_id)
    let mergedCount = 0
    if (globalBinding) {
      const existing = new Set(globalBinding.models.map((m) => m.model_id.toLowerCase()))
      const merged = legacyModels.filter((m) => !existing.has(m.model_id.toLowerCase()))
      mergedCount = merged.length
      globalBinding.models = [...globalBinding.models, ...merged]
    } else if (legacyModels.length > 0) {
      forms.global.push({
        group_id: binding.group_id,
        enabled: binding.enabled,
        models: [...legacyModels]
      })
    }
    // 从应用层移除该绑定，保存为剩余清单（可能为空 = 清掉历史补录）
    forms[app] = forms[app].filter((b) => b.group_id !== binding.group_id)
    await playgroundAdminAPI.updateGlobalConfig(
      forms.global.map((b) => ({
        group_id: b.group_id,
        enabled: b.enabled,
        models: b.models
          .filter((m) => m.selected && !m.fromGlobal)
          .map((m, idx): PlaygroundAppModelInput => ({
            model_id: m.model_id,
            display_name: m.display_name,
            price_label: m.price_label,
            unit_hint: m.unit_hint,
            description: m.description,
            enabled: m.enabled,
            sort_order: m.sort_order || idx + 1,
            model_kind: m.model_kind || '',
            monitor_id: m.monitor_id
          }))
      }))
    )
    await playgroundAdminAPI.updateConfig(
      app,
      forms[app].map((b) => ({
        group_id: b.group_id,
        enabled: b.enabled,
        models: b.models
          .filter((m) => m.selected && !m.fromGlobal)
          .map((m, idx): PlaygroundAppModelInput => ({
            model_id: m.model_id,
            display_name: m.display_name,
            price_label: m.price_label,
            unit_hint: m.unit_hint,
            description: m.description,
            enabled: m.enabled,
            sort_order: m.sort_order || idx + 1,
            model_kind: m.model_kind || '',
            monitor_id: m.monitor_id
          }))
      }))
    )
    // fork: 空绑定（应用层没有独立模型，模型全由全局提供）迁移 = 仅清理，明确告知
    if (mergedCount === 0) {
      appStore.showInfo(t('admin.playground.migratedNothing'))
    } else {
      appStore.showSuccess(t('admin.playground.migrated'))
    }
    await loadData()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.saveFailed')))
    console.error('Failed to migrate binding to global:', error)
    await loadData()
  } finally {
    migratingGroup.value = null
  }
}

/** fork(方案五阶段4): 注入自检——预览三个工作台实际会收到的注入清单 */
async function openSelfCheck() {
  selfCheckOpen.value = true
  selfCheckLoading.value = true
  try {
    const res = await playgroundAdminAPI.selfCheck()
    selfCheckData.value = res.apps
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.loadFailed')))
    console.error('Failed to load playground self-check:', error)
    selfCheckOpen.value = false
  } finally {
    selfCheckLoading.value = false
  }
}

async function loadMonitors() {
  try {
    const res = await channelMonitorAPI.list({ enabled: true, page_size: 200 })
    monitors.value = (res.items ?? []).map((m) => ({
      id: m.id,
      name: m.name,
      group_name: m.group_name,
      primary_model: m.primary_model
    }))
  } catch (error) {
    // 监控列表拉取失败不阻塞配置页；下拉为空即可
    console.error('Failed to load channel monitors:', error)
    monitors.value = []
  }
}

const tabs = computed(() => [
  { key: 'global' as TabKey, label: t('admin.playground.tabs.global') },
  { key: 'chat' as TabKey, label: t('admin.playground.tabs.chat') },
  { key: 'image' as TabKey, label: t('admin.playground.tabs.image') },
  { key: 'canvas' as TabKey, label: t('admin.playground.tabs.canvas') }
])

/** 当前页签的绑定列表（模板统一引用；全局页签用 forms.global） */
const activeBindings = computed<BindingRow[]>(() =>
  activeTab.value === 'global' ? forms.global : forms[activeTab.value]
)

// ==================== 全局渠道配置 ====================
// 一处维护「渠道 + 模型清单」，三个工作台按模型类型自动分流注入：
//   对话 → chat；生图 / 画布 → chat + image
function applyGlobalConfig(bindings: PlaygroundGlobalBindingView[]) {
  forms.global = (bindings ?? []).map((b) => ({
    group_id: b.group_id,
    enabled: b.enabled,
    models: (b.models ?? []).map((m) => ({
      selected: true,
      model_id: m.model_id,
      display_name: m.display_name,
      price_label: m.price_label,
      unit_hint: m.unit_hint,
      description: m.description,
      enabled: m.enabled,
      sort_order: m.sort_order,
      model_kind: m.model_kind ?? '',
      monitor_id: m.monitor_id ?? null
    }))
  }))
}

const groupName = (id: number) => groups.value.find((g) => g.id === id)?.name ?? `#${id}`
const groupPlatform = (id: number) => groups.value.find((g) => g.id === id)?.platform ?? ''

const monitorOptionTitle = (id: number | null) => {
  if (!id) return ''
  const mo = monitors.value.find((m) => m.id === id)
  return mo ? `${mo.group_name} · ${mo.primary_model}` : ''
}

/** fork: 类型展示名映射（与类型下拉选项文案一致） */
const kindLabel = (kind: string): string => {
  const map: Record<string, string> = {
    chat: t('admin.playground.kindChat'),
    image: t('admin.playground.kindImage'),
    video: t('admin.playground.kindVideo'),
    audio: t('admin.playground.kindAudio')
  }
  return map[kind] ?? kind
}

/**
 * fork: 该行的「自动识别」类型（仅当用户未显式指定 kind 时按模型名关键词推断，
 * 与后端 InferModelKind 口径一致；保存后后端会把推断结果填充落库）。
 */
const inferredKind = (row: ModelRow): string => {
  if ((row.model_kind || '').trim()) return ''
  return inferModelKind(row.model_id)
}

const addableOptions = computed(() => {
  const bound = new Set(activeBindings.value.map((b) => b.group_id))
  return groups.value
    .filter((g) => !bound.has(g.id))
    .map((g) => ({ value: g.id, label: `${g.name} · ${g.platform}` }))
})

function applyBundle(bundle: PlaygroundAppBundle) {
  const key = bundle.app as AppKey
  if (!APP_KEYS.includes(key)) return
  forms[key] = (bundle.bindings ?? []).map((b) => ({
    group_id: b.group_id,
    enabled: b.enabled,
    models: (b.models ?? []).map((m) => ({
      selected: true,
      model_id: m.model_id,
      display_name: m.display_name,
      price_label: m.price_label,
      unit_hint: m.unit_hint,
      description: m.description,
      enabled: m.enabled,
      sort_order: m.sort_order,
      model_kind: m.model_kind ?? '',
      monitor_id: m.monitor_id ?? null
    }))
  }))
}

async function loadData() {
  loading.value = true
  try {
    const [apps, allGroups, globalBindings] = await Promise.all([
      playgroundAdminAPI.listConfigs(),
      adminAPI.groups.getAll(),
      playgroundAdminAPI.listGlobalConfig()
    ])
    groups.value = allGroups
    apps.forEach(applyBundle)
    applyGlobalConfig(globalBindings)
    mergeGlobalIntoAppForms()
    void loadMonitors()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.loadFailed')))
    console.error('Failed to load playground configs:', error)
  } finally {
    loading.value = false
  }
}

// fork: 各应用页签的绑定卡片内合并展示「全局配置的生效模型」（只读、标注「全局」），
// 让既有配置在任何页签都可见；保存时这些行不提交（仍由全局页签统一管理）。
// 注入口径与运行时下发一致：chat → chat；image/canvas → chat + image（未标记类型的全放行）。
function mergeGlobalIntoAppForms() {
  const allowedByApp: Record<AppKey, Set<string>> = {
    chat: new Set(['chat']),
    image: new Set(['chat', 'image']),
    canvas: new Set(['chat', 'image'])
  }
  const globalByGroup = new Map<number, ModelRow[]>()
  for (const b of forms.global) globalByGroup.set(b.group_id, b.models)

  for (const app of APP_KEYS) {
    const allowed = allowedByApp[app]
    for (const binding of forms[app]) {
      const globalModels = globalByGroup.get(binding.group_id) ?? []
      if (!globalModels.length) continue
      // fork: 去重改为大小写不敏感，与后端 normalizeModels/mergeGlobalAndAppModels 口径一致
      const existing = new Set(binding.models.map((m) => m.model_id.toLowerCase()))
      const injected: ModelRow[] = []
      for (const m of globalModels) {
        if (existing.has(m.model_id.toLowerCase())) continue
        // fork: 空 kind 与后端同口径走关键词推断，未标记的图片模型不再混入纯文本页签
        const kind = (m.model_kind || '').trim() || inferModelKind(m.model_id)
        if (!allowed.has(kind)) continue
        existing.add(m.model_id.toLowerCase())
        injected.push({ ...m, selected: true, fromGlobal: true })
      }
      if (injected.length) binding.models = [...injected, ...binding.models]
    }
  }
}

function addBinding() {
  const id = pendingGroupId.value
  if (!id) return
  if (forms[activeTab.value].some((b) => b.group_id === id)) return
  forms[activeTab.value].push({ group_id: id, enabled: true, models: [] })
  pendingGroupId.value = null
}

function removeBinding(groupId: number) {
  forms[activeTab.value] = forms[activeTab.value].filter((b) => b.group_id !== groupId)
}

async function fetchModels(groupId: number) {
  const tab = activeTab.value
  fetchingGroup.value = groupId
  try {
    const candidates =
      tab === 'global'
        ? await playgroundAdminAPI.fetchGlobalModelCandidates(groupId)
        : await playgroundAdminAPI.fetchModelCandidates(tab, groupId)
    const binding = forms[tab].find((b) => b.group_id === groupId)
    if (!binding) return
    const existing = new Set(binding.models.map((m) => m.model_id.toLowerCase()))
    const base = binding.models.length
    let added = 0
    candidates.forEach((id) => {
      if (existing.has(id.toLowerCase())) return
      added += 1
      binding.models.push({
        selected: false,
        model_id: id,
        display_name: '',
        price_label: '',
        unit_hint: '',
        description: '',
        enabled: true,
        sort_order: base + added,
        model_kind: '',
        monitor_id: null
      })
    })
    if (added === 0) {
      appStore.showInfo(t('admin.playground.emptyHint'))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.fetchFailed')))
    console.error('Failed to fetch model candidates:', error)
  } finally {
    fetchingGroup.value = null
  }
}

async function saveAll() {
  const tab = activeTab.value
  saving.value = true
  try {
    const bindings = forms[tab].map((b) => ({
      group_id: b.group_id,
      enabled: b.enabled,
      models: b.models
        .filter((m) => m.selected && !m.fromGlobal)
        .map((m, idx): PlaygroundAppModelInput => ({
          model_id: m.model_id,
          display_name: m.display_name,
          price_label: m.price_label,
          unit_hint: m.unit_hint,
          description: m.description,
          enabled: m.enabled,
          sort_order: m.sort_order || idx + 1,
          model_kind: m.model_kind || '',
          monitor_id: m.monitor_id
        }))
    }))
    if (tab === 'global') {
      applyGlobalConfig(await playgroundAdminAPI.updateGlobalConfig(bindings))
    } else {
      const bundle = await playgroundAdminAPI.updateConfig(tab, bindings)
      if (bundle) applyBundle(bundle)
    }
    appStore.showSuccess(t('admin.playground.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.saveFailed')))
    console.error('Failed to save playground config:', error)
  } finally {
    saving.value = false
  }
}

onMounted(loadData)
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-4">
      <!-- 页头 -->
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.playground.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.playground.description') }}
          </p>
        </div>
        <!-- fork(方案五阶段4): 注入自检——一键预览三个工作台实际会收到的模型 -->
        <button class="btn btn-secondary shrink-0" :disabled="selfCheckLoading" @click="openSelfCheck">
          {{ selfCheckLoading ? t('admin.playground.selfCheckLoading') : t('admin.playground.selfCheck') }}
        </button>
      </div>

      <!-- 应用切换 -->
      <div class="tabs inline-flex" role="tablist" :aria-label="t('admin.playground.title')">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          role="tab"
          class="tab"
          :class="{ 'tab-active': activeTab === tab.key }"
          :aria-selected="activeTab === tab.key"
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- 全局页签说明：一处维护「渠道 + 模型」，三个工作台按模型类型自动分流 -->
      <div
        v-if="activeTab === 'global'"
        class="rounded-xl border border-blue-100 bg-blue-50/60 p-3 text-xs leading-relaxed text-gray-600 dark:border-blue-500/20 dark:bg-blue-500/5 dark:text-gray-300"
      >
        {{ t('admin.playground.globalHint') }}
      </div>

      <!-- fork(方案五阶段3): 应用页签为历史补录数据（只读），引导迁移到全局 -->
      <div
        v-if="legacyTab"
        class="rounded-xl border border-amber-200 bg-amber-50/70 p-3 text-xs leading-relaxed text-gray-600 dark:border-amber-500/20 dark:bg-amber-500/5 dark:text-gray-300"
      >
        {{ t('admin.playground.legacyBanner') }}
      </div>

      <!-- 渠道绑定区（全局 / 各应用共用同一套「添加渠道 + 拉取模型」编辑体验） -->
      <!-- 添加分组（应用页签只读，仅全局可添加） -->
      <div v-if="!legacyTab" class="flex items-center gap-3">
        <div class="w-72">
          <Select
            v-model="pendingGroupId"
            :options="addableOptions"
            :placeholder="t('admin.playground.groupPlaceholder')"
          />
        </div>
        <button class="btn btn-secondary" :disabled="!pendingGroupId || loading" @click="addBinding">
          {{ t('admin.playground.addGroup') }}
        </button>
        <span class="ml-auto text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.playground.bindingCount', { count: activeBindings.length }) }}
        </span>
      </div>

      <!-- 空状态 -->
      <div
        v-if="!loading && activeBindings.length === 0"
        class="rounded-2xl border border-dashed border-gray-200 py-14 text-center dark:border-dark-600"
      >
        <p class="text-sm font-medium text-gray-600 dark:text-gray-300">
          {{ t('admin.playground.noBindings') }}
        </p>
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.playground.noBindingsHint') }}</p>
      </div>

      <!-- 分组绑定卡片 -->
      <section
        v-for="binding in activeBindings"
        :key="binding.group_id"
        class="card"
      >
        <div class="card-header flex items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <span class="text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ groupName(binding.group_id) }}
            </span>
            <span
              class="rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400"
            >
              {{ groupPlatform(binding.group_id) }}
            </span>
          </div>
          <div class="flex items-center gap-3">
            <template v-if="!legacyTab">
              <label class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.playground.enabled') }}
                <Toggle v-model="binding.enabled" />
              </label>
              <button
                class="btn btn-ghost btn-sm text-red-600"
                @click="removeBinding(binding.group_id)"
              >
                {{ t('admin.playground.removeGroup') }}
              </button>
            </template>
            <!-- fork(方案五阶段3): 历史补录绑定一键并入全局配置。
                 迁移期间禁用全部迁移按钮：两次迁移并发时后落库的保存可能覆盖前一次的合并结果。 -->
            <button
              v-else
              class="btn btn-secondary btn-sm"
              :disabled="migratingGroup !== null"
              @click="migrateBindingToGlobal(activeTab as AppKey, binding)"
            >
              {{ migratingGroup === binding.group_id ? t('admin.playground.migrating') : t('admin.playground.migrateToGlobal') }}
            </button>
          </div>
        </div>

        <div class="card-body space-y-3">
          <div class="flex items-center justify-between">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.playground.priceHint') }}
            </p>
            <button
              class="btn btn-secondary btn-sm"
              :disabled="fetchingGroup === binding.group_id"
              @click="fetchModels(binding.group_id)"
            >
              {{
                fetchingGroup === binding.group_id
                  ? t('admin.playground.fetching')
                  : t('admin.playground.fetchModels')
              }}
            </button>
          </div>

          <div
            v-if="binding.models.length === 0"
            class="rounded-xl border border-dashed border-gray-200 py-8 text-center dark:border-dark-600"
          >
            <p class="text-xs text-gray-400">{{ t('admin.playground.emptyModels') }}</p>
          </div>

          <div
            v-else
            class="max-h-80 overflow-auto rounded-xl border border-gray-200 dark:border-dark-600"
          >
            <table class="w-full text-left text-xs">
              <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-800">
                <tr class="text-gray-500 dark:text-gray-400">
                  <th class="w-10 px-3 py-2 font-medium">
                    <input
                      type="checkbox"
                      class="h-3.5 w-3.5"
                      :checked="binding.models.length > 0 && binding.models.every((m) => m.selected)"
                      @change="
                        (e) =>
                          binding.models.forEach(
                            (m) => (m.selected = (e.target as HTMLInputElement).checked)
                          )
                      "
                    />
                  </th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.model') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.displayName') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.priceLabel') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.unitHint') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.description') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.kind') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.monitor') }}</th>
                  <th class="w-16 px-3 py-2 font-medium">{{ t('admin.playground.col.sortOrder') }}</th>
                  <th class="w-14 px-3 py-2 font-medium">{{ t('admin.playground.col.enabled') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in binding.models"
                  :key="row.model_id + (row.fromGlobal ? '@g' : '')"
                  class="border-t border-gray-100 dark:border-dark-700"
                  :class="row.fromGlobal ? 'bg-blue-50/40 dark:bg-blue-500/[0.04]' : ''"
                >
                  <td class="px-3 py-2">
                    <input
                      v-model="row.selected"
                      type="checkbox"
                      class="h-3.5 w-3.5"
                      :disabled="legacyTab || row.fromGlobal"
                      :title="row.fromGlobal ? t('admin.playground.globalRowHint') : ''"
                    />
                  </td>
                  <td class="px-3 py-2 font-mono text-[11px] text-gray-700 dark:text-gray-300">
                    {{ row.model_id }}
                    <span
                      v-if="row.fromGlobal"
                      class="ml-1 inline-block rounded bg-blue-100 px-1 py-0.5 align-middle font-sans text-[10px] font-medium text-blue-600 dark:bg-blue-500/15 dark:text-blue-300"
                      :title="t('admin.playground.globalRowHint')"
                    >
                      {{ t('admin.playground.globalTag') }}
                    </span>
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.display_name"
                      type="text"
                      class="input h-7 w-32 px-2 py-0.5 text-xs"
                      :disabled="legacyTab || row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.price_label"
                      type="text"
                      class="input h-7 w-28 px-2 py-0.5 text-xs"
                      placeholder="0.5 积分/次"
                      :disabled="legacyTab || row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.unit_hint"
                      type="text"
                      class="input h-7 w-20 px-2 py-0.5 text-xs"
                      :disabled="legacyTab || row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.description"
                      type="text"
                      class="input h-7 w-40 px-2 py-0.5 text-xs"
                      :disabled="legacyTab || row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <div class="flex flex-col gap-1">
                      <select
                        v-model="row.model_kind"
                        class="input h-7 w-24 px-1.5 py-0.5 text-xs"
                        :title="t('admin.playground.kindHint')"
                        :disabled="legacyTab || row.fromGlobal"
                      >
                        <option value="">{{ t('admin.playground.kindAuto') }}</option>
                        <option value="chat">{{ t('admin.playground.kindChat') }}</option>
                        <option value="image">{{ t('admin.playground.kindImage') }}</option>
                        <option value="video">{{ t('admin.playground.kindVideo') }}</option>
                        <option value="audio">{{ t('admin.playground.kindAudio') }}</option>
                      </select>
                      <!-- fork: 未显式指定类型时提示保存后将按模型名自动识别（口径与后端 InferModelKind 一致） -->
                      <span
                        v-if="inferredKind(row)"
                        class="inline-block w-fit rounded bg-amber-50 px-1 py-0.5 text-[10px] font-medium text-amber-600 dark:bg-amber-500/10 dark:text-amber-400"
                        :title="t('admin.playground.kindInferredHint')"
                      >
                        {{ t('admin.playground.kindInferredAs', { kind: kindLabel(inferredKind(row)) }) }}
                      </span>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <select
                      v-model="row.monitor_id"
                      class="input h-7 w-36 px-1.5 py-0.5 text-xs"
                      :title="monitorOptionTitle(row.monitor_id)"
                      :disabled="legacyTab || row.fromGlobal"
                    >
                      <option :value="null">{{ t('admin.playground.monitorNone') }}</option>
                      <option v-for="mo in monitors" :key="mo.id" :value="mo.id">
                        {{ mo.name }}（{{ mo.primary_model }}）
                      </option>
                    </select>
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model.number="row.sort_order"
                      type="number"
                      class="input h-7 w-14 px-2 py-0.5 text-xs"
                      :disabled="legacyTab || row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.enabled"
                      type="checkbox"
                      class="h-3.5 w-3.5"
                      :disabled="legacyTab || row.fromGlobal"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <!-- 保存（fork: 应用页签只读，仅全局页签可保存） -->
      <div v-if="!legacyTab" class="flex justify-end">
        <button class="btn btn-primary" :disabled="saving || loading" @click="saveAll">
          {{ saving ? t('admin.playground.saving') : t('admin.playground.save') }}
        </button>
      </div>

      <!-- fork(方案五阶段4): 注入自检弹窗——三个工作台实际收到的注入清单 -->
      <div
        v-if="selfCheckOpen"
        class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4 sm:p-8"
        @click.self="selfCheckOpen = false"
      >
        <div class="w-full max-w-4xl rounded-2xl bg-white p-5 shadow-xl dark:bg-dark-800">
          <div class="mb-3 flex items-center justify-between gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.playground.selfCheckTitle') }}
            </h2>
            <button
              class="flex h-7 w-7 items-center justify-center rounded-lg text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              :title="t('admin.playground.selfCheckClose')"
              @click="selfCheckOpen = false"
            >
              <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18 18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.playground.selfCheckHint') }}
          </p>
          <div class="space-y-4">
            <section
              v-for="app in selfCheckData ?? []"
              :key="app.app"
              class="rounded-xl border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="mb-2 flex items-center gap-2">
                <span class="text-sm font-medium text-gray-900 dark:text-gray-100">
                  {{ appLabel(app.app) }}
                </span>
                <span class="rounded-full bg-gray-100 px-2 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                  {{ t('admin.playground.selfCheckModelCount', { count: app.groups.reduce((n, g) => n + g.models.length, 0) }) }}
                </span>
              </div>
              <p v-if="app.groups.length === 0" class="py-3 text-center text-xs text-gray-400">
                {{ t('admin.playground.selfCheckEmpty') }}
              </p>
              <div v-for="g in app.groups" :key="g.group_id" class="mb-2 last:mb-0">
                <div class="mb-1 text-xs font-medium text-gray-600 dark:text-gray-300">
                  {{ g.group_name || `#${g.group_id}` }}
                </div>
                <div class="flex flex-wrap gap-1.5">
                  <span
                    v-for="m in g.models"
                    :key="m.model_id"
                    class="inline-flex items-center gap-1 rounded-md border border-gray-200 px-1.5 py-0.5 text-[11px] text-gray-700 dark:border-dark-600 dark:text-gray-300"
                  >
                    <span
                      class="inline-block h-1.5 w-1.5 rounded-full"
                      :class="{
                        'bg-emerald-500': m.monitor_status === 'operational',
                        'bg-amber-500': m.monitor_status === 'degraded',
                        'bg-red-500': m.monitor_status === 'failed' || m.monitor_status === 'error',
                        'bg-gray-300 dark:bg-dark-600': !m.monitor_status
                      }"
                    />
                    {{ m.display_name || m.model_id }}
                    <span class="text-gray-400">· {{ kindLabel(m.model_kind || '') }}</span>
                    <span
                      v-if="m.long_context_pricing_enabled && (m.long_context_threshold ?? 0) > 0"
                      class="text-amber-600 dark:text-amber-400"
                      :title="t('admin.playground.selfCheckLongCtx', { threshold: m.long_context_threshold ?? 0 })"
                    >
                      · {{ t('admin.playground.selfCheckLongCtx', { threshold: m.long_context_threshold ?? 0 }) }}
                    </span>
                  </span>
                </div>
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

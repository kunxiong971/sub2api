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
  type PlaygroundGlobalBindingView
} from '@/api/admin/playground'
import { extractApiErrorMessage } from '@/utils/apiError'

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

/** 当前应用页签的绑定列表（全局页签下为空，模板统一引用） */
const activeBindings = computed<BindingRow[]>(() =>
  activeTab.value === 'global' ? [] : forms[activeTab.value]
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
      const existing = new Set(binding.models.map((m) => m.model_id))
      const injected: ModelRow[] = []
      for (const m of globalModels) {
        if (existing.has(m.model_id)) continue
        const kind = (m.model_kind || '').trim()
        if (kind && !allowed.has(kind)) continue
        existing.add(m.model_id)
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
    const existing = new Set(binding.models.map((m) => m.model_id))
    const base = binding.models.length
    let added = 0
    candidates.forEach((id) => {
      if (existing.has(id)) return
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
      <div>
        <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.playground.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.playground.description') }}
        </p>
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

      <!-- 渠道绑定区（全局 / 各应用共用同一套「添加渠道 + 拉取模型」编辑体验） -->
      <!-- 添加分组 -->
      <div class="flex items-center gap-3">
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
                      :disabled="row.fromGlobal"
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
                      :disabled="row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.price_label"
                      type="text"
                      class="input h-7 w-28 px-2 py-0.5 text-xs"
                      placeholder="0.5 积分/次"
                      :disabled="row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.unit_hint"
                      type="text"
                      class="input h-7 w-20 px-2 py-0.5 text-xs"
                      :disabled="row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.description"
                      type="text"
                      class="input h-7 w-40 px-2 py-0.5 text-xs"
                      :disabled="row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <select
                      v-model="row.model_kind"
                      class="input h-7 w-24 px-1.5 py-0.5 text-xs"
                      :title="t('admin.playground.kindHint')"
                      :disabled="row.fromGlobal"
                    >
                      <option value="">{{ t('admin.playground.kindAuto') }}</option>
                      <option value="chat">{{ t('admin.playground.kindChat') }}</option>
                      <option value="image">{{ t('admin.playground.kindImage') }}</option>
                      <option value="video">{{ t('admin.playground.kindVideo') }}</option>
                      <option value="audio">{{ t('admin.playground.kindAudio') }}</option>
                    </select>
                  </td>
                  <td class="px-3 py-2">
                    <select
                      v-model="row.monitor_id"
                      class="input h-7 w-36 px-1.5 py-0.5 text-xs"
                      :title="monitorOptionTitle(row.monitor_id)"
                      :disabled="row.fromGlobal"
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
                      :disabled="row.fromGlobal"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.enabled"
                      type="checkbox"
                      class="h-3.5 w-3.5"
                      :disabled="row.fromGlobal"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <!-- 保存 -->
      <div class="flex justify-end">
        <button class="btn btn-primary" :disabled="saving || loading" @click="saveAll">
          {{ saving ? t('admin.playground.saving') : t('admin.playground.save') }}
        </button>
      </div>
    </div>
  </AppLayout>
</template>

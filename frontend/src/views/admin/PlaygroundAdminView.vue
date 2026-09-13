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
  type PlaygroundGlobalModelConfig
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

/** 页签：三个应用 + 全局模型库 */
type TabKey = AppKey | 'global'
const activeTab = ref<TabKey>('chat')
const loading = ref(false)
const groups = ref<AdminGroup[]>([])
const monitors = ref<MonitorOption[]>([])
const forms = reactive<Record<AppKey, BindingRow[]>>({ chat: [], image: [], canvas: [] })
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
  { key: 'chat' as TabKey, label: t('admin.playground.tabs.chat') },
  { key: 'image' as TabKey, label: t('admin.playground.tabs.image') },
  { key: 'canvas' as TabKey, label: t('admin.playground.tabs.canvas') },
  { key: 'global' as TabKey, label: t('admin.playground.tabs.global') }
])

/** 当前应用页签的绑定列表（全局页签下为空，模板统一引用） */
const activeBindings = computed<BindingRow[]>(() =>
  activeTab.value === 'global' ? [] : forms[activeTab.value]
)

// ==================== 全局模型库 ====================
// 模型展示信息只维护一份，各工作台按类型自动注入：
//   对话 → chat；生图/画布 → chat + image
interface GlobalModelRow {
  model_id: string
  display_name: string
  price_label: string
  unit_hint: string
  description: string
  model_kind: string
  enabled: boolean
  sort_order: number
  monitor_id: number | null
}

const globalModels = ref<GlobalModelRow[]>([])
const globalLoading = ref(false)
const globalSaving = ref(false)

function toGlobalRow(m: PlaygroundGlobalModelConfig): GlobalModelRow {
  return {
    model_id: m.model_id,
    display_name: m.display_name,
    price_label: m.price_label,
    unit_hint: m.unit_hint,
    description: m.description,
    model_kind: m.model_kind ?? '',
    enabled: m.enabled,
    sort_order: m.sort_order,
    monitor_id: m.monitor_id ?? null
  }
}

async function loadGlobalModels() {
  globalLoading.value = true
  try {
    const models = await playgroundAdminAPI.listGlobalModels()
    globalModels.value = models.map(toGlobalRow)
  } catch (error) {
    console.error('Failed to load global models:', error)
    globalModels.value = []
  } finally {
    globalLoading.value = false
  }
}

function addGlobalModel() {
  globalModels.value.push({
    model_id: '',
    display_name: '',
    price_label: '',
    unit_hint: '',
    description: '',
    model_kind: '',
    enabled: true,
    sort_order: globalModels.value.length + 1,
    monitor_id: null
  })
}

function removeGlobalModel(index: number) {
  globalModels.value.splice(index, 1)
}

async function saveGlobalModels() {
  globalSaving.value = true
  try {
    const saved = await playgroundAdminAPI.updateGlobalModels(
      globalModels.value
        .filter((m) => m.model_id.trim() !== '')
        .map((m, idx) => ({
          model_id: m.model_id.trim(),
          display_name: m.display_name,
          price_label: m.price_label,
          unit_hint: m.unit_hint,
          description: m.description,
          model_kind: m.model_kind,
          enabled: m.enabled,
          sort_order: m.sort_order || idx + 1,
          monitor_id: m.monitor_id
        }))
    )
    globalModels.value = saved.map(toGlobalRow)
    appStore.showSuccess(t('admin.playground.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.saveFailed')))
    console.error('Failed to save global models:', error)
  } finally {
    globalSaving.value = false
  }
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
    const [apps, allGroups] = await Promise.all([
      playgroundAdminAPI.listConfigs(),
      adminAPI.groups.getAll()
    ])
    groups.value = allGroups
    apps.forEach(applyBundle)
    void loadMonitors()
    void loadGlobalModels()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.playground.errors.loadFailed')))
    console.error('Failed to load playground configs:', error)
  } finally {
    loading.value = false
  }
}

function addBinding() {
  if (activeTab.value === 'global') return
  const id = pendingGroupId.value
  if (!id) return
  if (forms[activeTab.value].some((b) => b.group_id === id)) return
  forms[activeTab.value].push({ group_id: id, enabled: true, models: [] })
  pendingGroupId.value = null
}

function removeBinding(groupId: number) {
  if (activeTab.value === 'global') return
  forms[activeTab.value] = forms[activeTab.value].filter((b) => b.group_id !== groupId)
}

async function fetchModels(groupId: number) {
  if (activeTab.value === 'global') return
  const app = activeTab.value
  fetchingGroup.value = groupId
  try {
    const candidates = await playgroundAdminAPI.fetchModelCandidates(app, groupId)
    const binding = forms[app].find((b) => b.group_id === groupId)
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
  if (activeTab.value === 'global') return
  const app = activeTab.value
  saving.value = true
  try {
    const bindings = forms[app].map((b) => ({
      group_id: b.group_id,
      enabled: b.enabled,
      models: b.models
        .filter((m) => m.selected)
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
    const bundle = await playgroundAdminAPI.updateConfig(app, bindings)
    if (bundle) applyBundle(bundle)
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

      <!-- 全局模型库：模型展示信息只维护一份，各工作台按类型自动注入 -->
      <template v-if="activeTab === 'global'">
        <div
          class="rounded-xl border border-blue-100 bg-blue-50/60 p-3 text-xs leading-relaxed text-gray-600 dark:border-blue-500/20 dark:bg-blue-500/5 dark:text-gray-300"
        >
          {{ t('admin.playground.globalHint') }}
        </div>

        <div class="card">
          <div class="card-header flex items-center justify-between gap-3">
            <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('admin.playground.globalTitle', { count: globalModels.length }) }}
            </p>
            <button class="btn btn-secondary" @click="addGlobalModel">
              {{ t('admin.playground.globalAdd') }}
            </button>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full text-xs">
              <thead class="bg-gray-50 dark:bg-dark-700">
                <tr class="text-left text-gray-500 dark:text-gray-400">
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.model') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.displayName') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.priceLabel') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.unitHint') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.description') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.kind') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('admin.playground.col.monitor') }}</th>
                  <th class="w-16 px-3 py-2 font-medium">{{ t('admin.playground.col.sortOrder') }}</th>
                  <th class="w-14 px-3 py-2 font-medium">{{ t('admin.playground.col.enabled') }}</th>
                  <th class="w-14 px-3 py-2 font-medium">{{ t('admin.playground.col.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(row, idx) in globalModels"
                  :key="idx"
                  class="border-t border-gray-100 dark:border-dark-700"
                >
                  <td class="px-3 py-2">
                    <input
                      v-model="row.model_id"
                      type="text"
                      class="input h-7 w-40 px-2 py-0.5 font-mono text-[11px]"
                      placeholder="gpt-image-2"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input v-model="row.display_name" type="text" class="input h-7 w-32 px-2 py-0.5 text-xs" />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.price_label"
                      type="text"
                      class="input h-7 w-28 px-2 py-0.5 text-xs"
                      placeholder="0.5 积分/次"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input v-model="row.unit_hint" type="text" class="input h-7 w-20 px-2 py-0.5 text-xs" />
                  </td>
                  <td class="px-3 py-2">
                    <input v-model="row.description" type="text" class="input h-7 w-40 px-2 py-0.5 text-xs" />
                  </td>
                  <td class="px-3 py-2">
                    <select
                      v-model="row.model_kind"
                      class="input h-7 w-24 px-1.5 py-0.5 text-xs"
                      :title="t('admin.playground.kindHint')"
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
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input v-model="row.enabled" type="checkbox" class="h-3.5 w-3.5" />
                  </td>
                  <td class="px-3 py-2">
                    <button
                      class="text-red-500 transition hover:text-red-600"
                      @click="removeGlobalModel(idx)"
                    >
                      {{ t('admin.playground.globalRemove') }}
                    </button>
                  </td>
                </tr>
                <tr v-if="globalModels.length === 0">
                  <td colspan="10" class="px-3 py-10 text-center text-gray-400">
                    {{ globalLoading ? t('common.loading') : t('admin.playground.globalEmpty') }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="mt-3 flex justify-end">
            <button class="btn btn-primary" :disabled="globalSaving" @click="saveGlobalModels">
              {{ globalSaving ? t('admin.playground.saving') : t('admin.playground.save') }}
            </button>
          </div>
        </div>
      </template>

      <!-- 应用绑定区 -->
      <template v-else>
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
                  :key="row.model_id"
                  class="border-t border-gray-100 dark:border-dark-700"
                >
                  <td class="px-3 py-2">
                    <input v-model="row.selected" type="checkbox" class="h-3.5 w-3.5" />
                  </td>
                  <td class="px-3 py-2 font-mono text-[11px] text-gray-700 dark:text-gray-300">
                    {{ row.model_id }}
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.display_name"
                      type="text"
                      class="input h-7 w-32 px-2 py-0.5 text-xs"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.price_label"
                      type="text"
                      class="input h-7 w-28 px-2 py-0.5 text-xs"
                      placeholder="0.5 积分/次"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.unit_hint"
                      type="text"
                      class="input h-7 w-20 px-2 py-0.5 text-xs"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input
                      v-model="row.description"
                      type="text"
                      class="input h-7 w-40 px-2 py-0.5 text-xs"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <select
                      v-model="row.model_kind"
                      class="input h-7 w-24 px-1.5 py-0.5 text-xs"
                      :title="t('admin.playground.kindHint')"
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
                    />
                  </td>
                  <td class="px-3 py-2">
                    <input v-model="row.enabled" type="checkbox" class="h-3.5 w-3.5" />
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
      </template>
    </div>
  </AppLayout>
</template>

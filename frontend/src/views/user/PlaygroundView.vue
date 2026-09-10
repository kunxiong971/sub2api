<script setup lang="ts">
/**
 * PlaygroundView - 集成工作台壳页面（/chat /image /canvas 共用）
 *
 * 职责：
 * 1. 调用 /keys/playground-config 获取用户可用分组及每组专用 key
 * 2. 以 iframe 加载同域反代部署的第三方应用（lobe-chat / gpt_image_playground / infinite-canvas）
 * 3. 按 app 类型注入配置：URL 查询参数 或 postMessage
 *
 * 新增分组无需改任何代码：配置接口实时返回最新分组列表。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getPlaygroundConfig, type PlaygroundConfig, type PlaygroundGroupConfig } from '@/api/playground'
import { buildGatewayUrl } from '@/api/url'
import {
  PLAYGROUND_APP_CONFIG,
  PLAYGROUND_CONFIG_MESSAGE_TYPE,
  normalizePlaygroundBase,
  resolvePlaygroundBase,
  type PlaygroundAppKey,
  type PlaygroundInjectedConfig
} from '@/config/playground'

const route = useRoute()
const router = useRouter()

const appKey = computed<PlaygroundAppKey>(() => {
  const value = route.meta.app
  if (value === 'image' || value === 'canvas') return value
  return 'chat'
})
const appMeta = computed(() => PLAYGROUND_APP_CONFIG[appKey.value])

const loading = ref(true)
const loadError = ref('')
const config = ref<PlaygroundConfig | null>(null)
const selectedGroupId = ref<number | null>(null)
const frameRef = ref<HTMLIFrameElement | null>(null)
const iframeKey = ref(0)

const groups = computed<PlaygroundGroupConfig[]>(() => config.value?.groups ?? [])
const selectedGroup = computed<PlaygroundGroupConfig | null>(
  () => groups.value.find((g) => g.id === selectedGroupId.value) ?? groups.value[0] ?? null
)

const STORAGE_GROUP_KEY = (app: PlaygroundAppKey) => `playground_group_${app}`

function gatewayV1URL(): string {
  const base = (config.value?.gateway_base_url || '').replace(/\/+$/, '')
  // 后端地址缺失时（异常场景）退回当前站点 origin
  return base ? `${base}/v1` : buildGatewayUrl('/v1')
}

function buildInjectedConfig(): PlaygroundInjectedConfig | null {
  const group = selectedGroup.value
  if (!group) return null
  return {
    app: appKey.value,
    apiUrl: gatewayV1URL(),
    apiKey: group.key,
    groupName: group.name,
    models: group.models ?? []
  }
}

function buildIframeSrc(): string {
  const base = normalizePlaygroundBase(resolvePlaygroundBase(appKey.value))
  if (appMeta.value.injectMode !== 'url') return base
  const cfg = buildInjectedConfig()
  if (!cfg) return base
  // gpt_image_playground 原生支持的快速配置参数
  const params = new URLSearchParams({
    apiUrl: cfg.apiUrl,
    apiKey: cfg.apiKey,
    profileName: `sub2api · ${cfg.groupName}`
  })
  if (cfg.models.length > 0) {
    params.set('model', cfg.models[0])
  }
  return `${base}?${params.toString()}`
}

// ==================== postMessage 注入 ====================

let messageTimer: number | null = null
let messageAttempts = 0
const MESSAGE_INTERVAL_MS = 1500
const MESSAGE_MAX_ATTEMPTS = 20

function stopPostMessage() {
  if (messageTimer !== null) {
    window.clearInterval(messageTimer)
    messageTimer = null
  }
}

function startPostMessage() {
  stopPostMessage()
  const cfg = buildInjectedConfig()
  if (!cfg) return
  messageAttempts = 0
  messageTimer = window.setInterval(() => {
    const frame = frameRef.value
    if (!frame?.contentWindow) return
    messageAttempts++
    frame.contentWindow.postMessage(
      { type: PLAYGROUND_CONFIG_MESSAGE_TYPE, payload: cfg },
      '*'
    )
    if (messageAttempts >= MESSAGE_MAX_ATTEMPTS) stopPostMessage()
  }, MESSAGE_INTERVAL_MS)
}

// ==================== 数据加载 ====================

async function loadConfig() {
  loading.value = true
  loadError.value = ''
  try {
    const data = await getPlaygroundConfig()
    config.value = data
    // 恢复上次选择的分组
    const saved = Number(localStorage.getItem(STORAGE_GROUP_KEY(appKey.value)) || '')
    if (saved && groups.value.some((g) => g.id === saved)) {
      selectedGroupId.value = saved
    }
    render()
  } catch (error) {
    const err = error as { message?: string }
    loadError.value = err.message || '加载配置失败'
  } finally {
    loading.value = false
  }
}

function render() {
  stopPostMessage()
  iframeKey.value++
  if (selectedGroup.value && appMeta.value.injectMode === 'postMessage') {
    // iframe 加载需要时间，稍等后开始注入
    window.setTimeout(startPostMessage, 1200)
  }
}

function onGroupChange(event: Event) {
  const value = Number((event.target as HTMLSelectElement).value)
  selectedGroupId.value = value
  try {
    localStorage.setItem(STORAGE_GROUP_KEY(appKey.value), String(value))
  } catch {
    // ignore
  }
  render()
}

function reloadApp() {
  render()
}

function goDashboard() {
  router.push('/dashboard')
}

watch(appKey, () => {
  const saved = Number(localStorage.getItem(STORAGE_GROUP_KEY(appKey.value)) || '')
  selectedGroupId.value = saved && groups.value.some((g) => g.id === saved) ? saved : null
  render()
})

onMounted(loadConfig)
onBeforeUnmount(stopPostMessage)
</script>

<template>
  <div class="flex h-screen flex-col bg-white dark:bg-dark-900">
    <!-- 顶栏 -->
    <header
      class="flex h-12 shrink-0 items-center gap-3 border-b border-gray-200 px-4 dark:border-dark-600"
    >
      <button
        class="flex items-center gap-1 text-sm text-gray-500 transition hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200"
        @click="goDashboard"
      >
        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M10.5 19.5 3 12m0 0 7.5-7.5M3 12h18" />
        </svg>
        <span class="hidden sm:inline">控制台</span>
      </button>

      <div class="h-4 w-px bg-gray-200 dark:bg-dark-600" />

      <h1 class="text-sm font-medium text-gray-900 dark:text-gray-100">{{ appMeta.title }}</h1>

      <div class="ml-auto flex items-center gap-2">
        <label v-if="groups.length > 1" class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          分组
          <select
            class="h-8 rounded-lg border border-gray-300 bg-white px-2 text-xs text-gray-800 outline-none transition focus:border-primary-500 dark:border-dark-500 dark:bg-dark-800 dark:text-gray-200"
            :value="selectedGroup?.id ?? ''"
            @change="onGroupChange"
          >
            <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>
        </label>
        <span
          v-else-if="selectedGroup"
          class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
        >
          {{ selectedGroup.name }}
        </span>

        <button
          class="flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 transition hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          title="重新加载"
          @click="reloadApp"
        >
          <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99"
            />
          </svg>
        </button>
      </div>
    </header>

    <!-- 内容区 -->
    <main class="relative min-h-0 flex-1">
      <!-- 加载中 -->
      <div v-if="loading" class="flex h-full items-center justify-center">
        <div class="flex flex-col items-center gap-3 text-gray-500 dark:text-gray-400">
          <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 0 1 8-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
          <p class="text-sm">正在获取工作台配置…</p>
        </div>
      </div>

      <!-- 加载失败 -->
      <div v-else-if="loadError" class="flex h-full items-center justify-center">
        <div class="max-w-sm text-center">
          <p class="text-sm text-red-500">{{ loadError }}</p>
          <button class="btn btn-secondary mt-4" @click="loadConfig">重试</button>
        </div>
      </div>

      <!-- 无可用分组 -->
      <div v-else-if="groups.length === 0" class="flex h-full items-center justify-center">
        <div class="max-w-sm text-center">
          <p class="text-sm text-gray-600 dark:text-gray-300">当前没有可用的分组</p>
          <p class="mt-2 text-xs text-gray-400">请联系管理员开通分组，或先前往密钥页查看</p>
          <button class="btn btn-primary mt-4" @click="goDashboard">返回控制台</button>
        </div>
      </div>

      <!-- iframe -->
      <iframe
        v-else
        :key="iframeKey"
        ref="frameRef"
        :src="buildIframeSrc()"
        class="h-full w-full border-0"
        allow="clipboard-write; microphone; camera"
        @load="startPostMessage"
      />
    </main>
  </div>
</template>

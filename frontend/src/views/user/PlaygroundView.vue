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
import { computed, onActivated, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  getPlaygroundConfig,
  type PlaygroundAppConfig,
  type PlaygroundAppModelInfo,
  type PlaygroundConfig,
  type PlaygroundGroupConfig
} from '@/api/playground'
import { buildGatewayUrl } from '@/api/url'
import {
  PLAYGROUND_APP_CONFIG,
  PLAYGROUND_CONFIG_MESSAGE_TYPE,
  PLAYGROUND_CONFIG_ACK_TYPE,
  PLAYGROUND_THEME_MESSAGE_TYPE,
  normalizePlaygroundBase,
  resolvePlaygroundBase,
  type PlaygroundAppKey,
  type PlaygroundInjectedConfig,
  type PlaygroundInjectedGroup,
  type PlaygroundInjectedModel
} from '@/config/playground'

// fork: keep-alive 按组件名缓存（与 App.vue 的 include 列表匹配），
// 保证 /chat /image /canvas 三个路由各自保活一个实例
defineOptions({ name: 'PlaygroundView' })

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

// 管理员在「工作台配置」中下发的当前应用注入规则；apps 为空表示后台尚未配置（回退旧行为）
const appConfig = computed<PlaygroundAppConfig | null>(() => {
  const apps = config.value?.apps ?? []
  if (apps.length === 0) return null
  return apps.find((a) => a.app === appKey.value) ?? null
})
// 管理配置模式下当前应用绑定的所有分组（多个，客户无需自选）
const managedGroups = computed<PlaygroundGroupConfig[]>(() =>
  (appConfig.value?.groups ?? []).map((g) => g.group)
)
// 后台已配置工作台，但当前应用未开放（未启用/未绑定分组/客户无该分组权限）
const appDisabled = computed<boolean>(() => {
  const apps = config.value?.apps ?? []
  return apps.length > 0 && appConfig.value === null
})

// 顶栏分组标签：分组较多时压缩为「首个 +N」，避免一长串撑爆顶栏（完整列表见悬停提示）
const groupSummaryLabel = computed(() => {
  const names = managedGroups.value.map((g) => g.name)
  if (names.length <= 2) return names.join(' · ')
  return `${names[0]} +${names.length - 1}`
})
const groupSummaryTooltip = computed(() => managedGroups.value.map((g) => g.name).join('\n'))

const selectedGroup = computed<PlaygroundGroupConfig | null>(
  () =>
    managedGroups.value[0] ??
    groups.value.find((g) => g.id === selectedGroupId.value) ??
    groups.value[0] ??
    null
)

const STORAGE_GROUP_KEY = (app: PlaygroundAppKey) => `playground_group_${app}`

function gatewayV1URL(): string {
  const base = (config.value?.gateway_base_url || '').replace(/\/+$/, '')
  // 后端地址缺失时（异常场景）退回当前站点 origin
  return base ? `${base}/v1` : buildGatewayUrl('/v1')
}

// 把一个分组构建为注入配置（含该分组下管理员标注的模型展示信息）
function buildInjectedGroup(
  group: PlaygroundGroupConfig,
  modelInfos?: PlaygroundAppModelInfo[]
): PlaygroundInjectedGroup {
  // /pgw 代理模式：浏览器只持有 24h 短时令牌，真实 key 留在服务端
  const usePgw = appMeta.value.usePgwProxy && !!config.value?.pgw_base_url && !!group.pgw_token
  const models: PlaygroundInjectedModel[] = (modelInfos ?? []).map((m) => ({
    model_id: m.model_id,
    display_name: m.display_name,
    price_label: m.price_label,
    unit_hint: m.unit_hint,
    description: m.description
  }))
  return {
    groupName: group.name,
    apiUrl: usePgw ? config.value!.pgw_base_url : gatewayV1URL(),
    apiKey: usePgw ? group.pgw_token : group.key,
    models
  }
}

function buildInjectedConfig(): PlaygroundInjectedConfig | null {
  // 管理模式：多分组（多渠道）
  if (managedGroups.value.length > 0 && appConfig.value) {
    const groups = appConfig.value.groups.map((ag) => buildInjectedGroup(ag.group, ag.models))
    const first = groups[0]
    return {
      app: appKey.value,
      apiUrl: first.apiUrl,
      apiKey: first.apiKey,
      groupName: first.groupName,
      models: first.models.map((m) => m.model_id),
      groups
    }
  }
  // 回退模式：单分组
  const group = selectedGroup.value
  if (!group) return null
  const g = buildInjectedGroup(group)
  return {
    app: appKey.value,
    apiUrl: g.apiUrl,
    apiKey: g.apiKey,
    groupName: g.groupName,
    models: group.models ?? [],
    groups: [g]
  }
}

function buildIframeSrc(): string {
  const base = normalizePlaygroundBase(resolvePlaygroundBase(appKey.value))
  // lobe 免登录：先经 bridge-login 以票据建立会话，再 302 回应用本体
  if (appMeta.value.injectMode === 'postMessage') {
    if (appMeta.value.autoLogin && config.value?.lobe_ticket) {
      const params = new URLSearchParams({
        ticket: config.value.lobe_ticket,
        callbackUrl: '/'
      })
      return `${base}api/sub2api/bridge-login?${params.toString()}`
    }
    return base
  }
  const cfg = buildInjectedConfig()
  if (!cfg) return base
  // gpt_image_playground 原生支持的快速配置参数（第一分组）
  const params = new URLSearchParams({
    apiUrl: cfg.apiUrl,
    apiKey: cfg.apiKey,
    profileName: `sub2api · ${cfg.groupName}`
  })
  if (cfg.models.length > 0) {
    params.set('model', cfg.models[0])
  }
  // 始终附带 profiles（每个分组一个 profile，含模型展示元数据）。
  // 注意：即使只有一个分组也要下发——否则子应用的模型选择器会退回 /v1/models
  // 全量列表，而不是工作台配置的模型清单（含展示名/价格标签）。
  if (cfg.groups && cfg.groups.length > 0) {
    params.set(
      'profiles',
      JSON.stringify(
        cfg.groups.map((g) => ({
          name: `sub2api · ${g.groupName}`,
          baseUrl: g.apiUrl,
          apiKey: g.apiKey,
          models: g.models
        }))
      )
    )
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

// 子应用收到配置后回发 ACK，宿主立即停止注入循环
function handleBridgeAck(event: MessageEvent) {
  const data = event.data as { type?: unknown } | null
  if (!data || data.type !== PLAYGROUND_CONFIG_ACK_TYPE) return
  if (event.source !== frameRef.value?.contentWindow) return
  stopPostMessage()
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
  broadcastTheme()
}

// ==================== 主题同步 ====================

let themeObserver: MutationObserver | null = null

function currentThemeMode(): 'dark' | 'light' {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

function broadcastTheme() {
  const frame = frameRef.value
  if (!frame?.contentWindow) return
  frame.contentWindow.postMessage(
    { type: PLAYGROUND_THEME_MESSAGE_TYPE, payload: { mode: currentThemeMode() } },
    '*'
  )
}

function watchPanelTheme() {
  if (themeObserver) return
  themeObserver = new MutationObserver(broadcastTheme)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
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

onMounted(() => {
  loadConfig()
  window.addEventListener('message', handleBridgeAck)
  watchPanelTheme()
})
// fork: keep-alive 恢复（从其他页面切回）：iframe 仍在后台运行，无需重建。
// 幂等重发配置注入与主题，确保管理后台的配置变更切回后即时生效。
onActivated(() => {
  if (config.value) {
    startPostMessage()
    broadcastTheme()
  }
})
onBeforeUnmount(() => {
  stopPostMessage()
  window.removeEventListener('message', handleBridgeAck)
  themeObserver?.disconnect()
  themeObserver = null
})
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

      <!-- fork: /pgw 代理模式徽标——真实 key 在服务端注入 -->
      <span
        v-if="appMeta.usePgwProxy && config?.pgw_base_url && selectedGroup?.pgw_token"
        class="flex items-center gap-1 rounded-md bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400"
        title="密钥代理已启用：真实 API Key 由服务端注入，浏览器不保存"
      >
        <svg class="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75 11.25 15 15 9.75m-3-7.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285Z" />
        </svg>
        代理
      </span>

      <div class="ml-auto flex items-center gap-2">
        <label
          v-if="managedGroups.length === 0 && groups.length > 1"
          class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400"
        >
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
          v-else-if="managedGroups.length > 1"
          class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
          :title="groupSummaryTooltip"
        >
          {{ groupSummaryLabel }}
        </span>
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

      <!-- 应用未开放（管理员未启用该工作台或客户无对应分组权限） -->
      <div v-else-if="appDisabled || !selectedGroup" class="flex h-full items-center justify-center">
        <div class="max-w-sm text-center">
          <p class="text-sm text-gray-600 dark:text-gray-300">该工作台暂未开放</p>
          <p class="mt-2 text-xs text-gray-400">管理员尚未开启此工作台，请稍后再试</p>
          <button class="btn btn-primary mt-4" @click="goDashboard">返回控制台</button>
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

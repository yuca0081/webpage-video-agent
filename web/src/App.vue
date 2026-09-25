<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  NConfigProvider, NMessageProvider, darkTheme,
} from 'naive-ui'
import HistoryRail from './components/HistoryRail.vue'
import StagePanel from './components/StagePanel.vue'
import ChatPanel from './components/ChatPanel.vue'
import NewProjectModal from './components/NewProjectModal.vue'
import MaterialCenter from './components/MaterialCenter.vue'
import { api, videoURL } from './api'
import type { AudioMeta, ChatRef, Msg, ProjectRow, ProjectView, Storyboard, StylePack, StyleSamples } from './types'
import { segAt, segTable } from './segtime'

const projects = ref<ProjectRow[]>([])
const current = ref<string>('')
const view = ref<ProjectView | null>(null)
const storyboard = ref<Storyboard | null>(null)
const style = ref<StyleSamples | null>(null)
const manuscript = ref<{ content: string; word_count: number } | null>(null)
const msgs = ref<Msg[]>([])
const chatDraft = ref('')
const chatRefs = ref<ChatRef[]>([]) // 📎 引用列表（时间轴/分镜点选加入，输入框上方展示）
const audioMeta = ref<AudioMeta | null>(null) // 时间轴字幕/配音轨数据源
// 制作管线各段状态：'' | running | done | error
const stageState = ref<Record<string, string>>({})
const agentBusy = ref(false)
const library = ref<StylePack[]>([])
const inspect = ref(false) // 检视模式（标题行按钮触发，StagePanel 消费）

let es: EventSource | null = null
let busyTimer: number | null = null

const stageOrder = ['tts', 'compositions', 'assemble', 'check', 'render']

async function loadProjects() {
  projects.value = await api.listProjects()
}

async function loadLibrary() {
  library.value = await api.listLibrary().catch(() => [])
}

async function publishPack(id: string) {
  await api.publishPack(id).catch(() => {})
  await loadLibrary()
}

async function loadProject(id: string) {
  view.value = await api.project(id)
  storyboard.value = view.value.gates.storyboard ? await api.storyboard(id).catch(() => null) : null
  style.value = view.value.gates.style_draft ? await api.style(id).catch(() => null) : null
  if (view.value.gates.manuscript) manuscript.value = await api.manuscript(id).catch(() => null)
  audioMeta.value = await api.audioMeta(id).catch(() => null)
}

async function loadMsgs() {
  if (!current.value) return
  const after = msgs.value.length ? Math.max(...msgs.value.map(m => m.id)) : 0
  const inc = await api.messages(current.value, after).catch(() => [])
  const seen = new Set(msgs.value.map(m => m.id))
  const fresh = inc.filter(m => !seen.has(m.id))
  if (fresh.length) msgs.value.push(...fresh)
}

function selectProject(id: string) {
  current.value = id
  inspect.value = false
  msgs.value = []
  chatRefs.value = []
  stageState.value = {}
}

// 回首页（素材中心）：清干净舞台态，避免旧项目标题/成片按钮残留
function goHome() {
  current.value = ''
  inspect.value = false
  view.value = null
  storyboard.value = null
  style.value = null
  manuscript.value = null
  audioMeta.value = null
  stageState.value = {}
}

// 📎 引用：点分镜/时间轴加段级（按段去重）；双击轨道/「引用此刻」加时刻级（按 ±0.75s 去重）；
// 检视点元素加元素级（按元素去重）；可单删、可清空
function addRef(idx: number, key: string) {
  if (!chatRefs.value.some(r => r.idx === idx)) chatRefs.value = [...chatRefs.value, { idx, key }]
}
function addElementRef(ref: ChatRef) {
  if (ref.elementId && chatRefs.value.some(r => r.elementId === ref.elementId)) return
  chatRefs.value = [...chatRefs.value, ref]
}
function addTimeRef(t: number) {
  const hit = segAt(segTable(storyboard.value, audioMeta.value), t)
  if (!hit) return
  if (chatRefs.value.some(r => r.idx === hit.idx && r.t != null && Math.abs(r.t - t) < 0.75)) return
  chatRefs.value = [...chatRefs.value, { idx: hit.idx, key: hit.key, t }]
}
function removeRef(ref: ChatRef) {
  chatRefs.value = chatRefs.value.filter(r =>
    ref.elementId ? r.elementId !== ref.elementId
      : ref.t != null ? !(r.idx === ref.idx && r.t != null && Math.abs(r.t - ref.t) < 0.75)
      : r.idx !== ref.idx)
}

async function refresh() {
  if (!current.value) return
  await loadProject(current.value).catch(() => {})
  await loadMsgs()
}

function markBusy() {
  agentBusy.value = true
  if (busyTimer) clearTimeout(busyTimer)
  busyTimer = window.setTimeout(() => { agentBusy.value = false }, 90_000)
}

function openSSE(id: string) {
  es?.close()
  es = new EventSource(`/api/projects/${id}/events`)
  es.addEventListener('msg', () => loadMsgs())
  es.addEventListener('agent_msg', ev => {
    agentBusy.value = false
    loadMsgs()
    loadLibrary() // Agent 可能刚起草了风格包
  })
  es.addEventListener('tool_call', () => { markBusy(); loadMsgs() })
  es.addEventListener('tool_result', () => { loadMsgs(); loadProject(id).catch(() => {}) })
  es.addEventListener('stage', ev => {
    const d = JSON.parse((ev as MessageEvent).data)
    if (d.stage === 'pipeline') return
    const st = d.detail.startsWith('running') ? 'running'
      : d.detail.startsWith('done') ? 'done'
      : d.detail.startsWith('error') || d.detail.startsWith('repair') ? 'error' : 'running'
    stageState.value = { ...stageState.value, [d.stage]: st }
    loadMsgs()
  })
  es.addEventListener('style_confirmed', () => loadProject(id).catch(() => {}))
  es.addEventListener('done', () => { stageState.value = {}; refresh() })
  es.addEventListener('cancelled', () => { stageState.value = {}; refresh() })
  es.addEventListener('error', () => refresh()) // SSE 层错误 → 全量刷新
}

watch(current, id => {
  if (id) { openSSE(id); refresh() }
  else { es?.close(); es = null } // 回首页（素材中心）：断开项目订阅
})

onBeforeUnmount(() => es?.close())

// 初始：加载列表，选中最近项目
loadProjects().then(() => {
  if (projects.value.length) selectProject(projects.value[0].id)
})
loadLibrary()

const showNew = ref(false)
async function onCreated(id: string) {
  showNew.value = false
  await loadProjects()
  selectProject(id)
}

const stageSummary = computed(() => {
  const states = stageOrder.map(s => stageState.value[s] ?? (view.value?.producing ? 'pending' : ''))
  if (!states.some(Boolean)) return null
  return stageOrder.map((s, i) => ({ key: s, state: states[i] }))
})

// naive-ui 全局主色对齐页面金色（创建按钮/单选/焦点等不再出现默认绿）
const themeOverrides = {
  common: {
    primaryColor: '#f0c674',
    primaryColorHover: '#ffdd9a',
    primaryColorPressed: '#d9ae5b',
    primaryColorSuppl: '#f0c674',
    borderRadius: '8px',
  },
}
</script>

<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <div class="shell">
        <HistoryRail :projects="projects" :current="current" @select="selectProject" @new="showNew = true" @home="goHome" />
        <main class="center">
          <header class="topbar">
            <div class="title">
              <span class="brand">帧述</span>
              <span class="name">{{ view?.name ?? '未选择项目' }}</span>
            </div>
            <!-- 成片态操作（状态 chips 在对话栏顶部） -->
            <div v-if="view?.has_video" class="ops">
              <button
                class="op-btn" :class="{ on: inspect }"
                :disabled="!!stageSummary || view.producing"
                @click="inspect = !inspect"
              >{{ inspect ? '退出检视' : '🔍 检视' }}</button>
              <a class="op-btn" :href="videoURL(current)" :download="`${view.name}.mp4`">⬇ 下载成片</a>
            </div>
          </header>
          <!-- 首页（未选项目）：素材中心（风格库=方法库飞轮入口；元素库=样张矩阵） -->
          <MaterialCenter v-if="!current" :packs="library" @publish="publishPack" />
          <StagePanel
            v-else
            :view="view" :storyboard="storyboard" :style-samples="style" :manuscript="manuscript"
            :stage-summary="stageSummary" :video-id="current" :audio-meta="audioMeta"
            :picked-idx="chatRefs.map(r => r.idx)" :inspect="inspect" @inspect-off="inspect = false"
            @confirm-style="async () => { if (current) { await api.confirmStyle(current); refresh() } }"
            @seg="addRef"
            @seg-element="addElementRef"
            @pick-time="addTimeRef"
            @cancel="async () => { if (current) { await api.cancel(current).catch(() => {}); refresh() } }"
          />
        </main>
        <ChatPanel
          :project-id="current" :msgs="msgs" :busy="agentBusy" v-model:draft="chatDraft" :refs="chatRefs" :view="view"
          @sent="markBusy" @remove-ref="removeRef" @clear-refs="chatRefs = []"
        />
      </div>
      <NewProjectModal v-model:show="showNew" @created="onCreated" />
    </n-message-provider>
  </n-config-provider>
</template>

<style>
:root { color-scheme: dark; }
* { box-sizing: border-box; margin: 0; }
html, body, #app { height: 100%; }
body {
  background: #0e0e12; color: #e6e6ea;
  font-family: 'Segoe UI', 'Microsoft YaHei', system-ui, sans-serif;
  font-size: 14px; overflow: hidden;
}
.shell { display: flex; height: 100vh; }
.center { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.topbar {
  height: 52px; flex: none; display: flex; align-items: center; justify-content: space-between;
  padding: 0 20px; border-bottom: 1px solid #232329; background: #121217;
}
.title { display: flex; align-items: baseline; gap: 12px; min-width: 0; }
.brand { font-weight: 700; letter-spacing: 3px; color: #f0c674; }
.name { font-size: 15px; color: #c9c9d1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.ops { display: flex; gap: 8px; }
.op-btn {
  border: 1px solid #5c4d24; background: #211d12; color: #f0c674;
  font-size: 12px; padding: 4px 14px; border-radius: 999px; cursor: pointer;
  text-decoration: none; display: inline-flex; align-items: center;
}
.op-btn:hover { background: #2a2416; }
.op-btn.on { background: #f0c674; color: #14140f; font-weight: 600; }
.op-btn:disabled { opacity: .4; cursor: not-allowed; }
</style>

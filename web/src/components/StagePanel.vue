<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NScrollbar, NSpin, NTag } from 'naive-ui'
import type { AudioMeta, ChatRef, ProjectView, Storyboard, StyleSamples } from '../types'
import { fmtClock, segAt, segTable } from '../segtime'
import { videoURL } from '../api'
import Timeline from './Timeline.vue'
import LiveFrame from './LiveFrame.vue'

const props = defineProps<{
  view: ProjectView | null
  storyboard: Storyboard | null
  styleSamples: StyleSamples | null
  manuscript: { content: string; word_count: number } | null
  stageSummary: { key: string; state: string }[] | null
  videoId: string
  audioMeta: AudioMeta | null
  pickedIdx: number[]
}>()
const emit = defineEmits<{ 'confirm-style': []; seg: [idx: number, key: string]; 'seg-element': [ref: ChatRef]; cancel: []; 'pick-time': [t: number] }>()

const stageName: Record<string, string> = {
  tts: '配音', compositions: '画面', assemble: '组装', check: '检查', render: '渲染',
}

// 视频播放 ↔ 时间轴播放头同步
const videoEl = ref<HTMLVideoElement | null>(null)
const cur = ref(0)
const dur = ref(0)
function onSeek(t: number) {
  cur.value = t
  // video 用 v-show（检视时隐藏）：currentTime 保持同步，退出检视不跳变
  if (videoEl.value) videoEl.value.currentTime = t
  if (inspect.value) {
    const hit = segAt(segsT.value, t)
    if (hit) { inspectSegId.value = hit.segId; inspectT.value = t }
  }
}
function pickSeg(idx: number, key: string) { emit('seg', idx, key) }
// 时刻引用：播放器「引用此刻」按钮 / 时间轴双击（全局秒，随消息结构化发出）
function pickTime(t: number) { emit('pick-time', t) }

// ── 检视模式（plan §4.3 修改态）：video ↔ 活合成物 LiveFrame ──
const inspect = ref(false)
const inspectSegId = ref('') // 当前加载的段（segNN）
const inspectT = ref(0) // 检视时刻（全局秒）
const frameRev = ref(0) // 重做完 bump，强制 LiveFrame 重载帧

const segsT = computed(() => segTable(props.storyboard, props.audioMeta))
const inspectCol = computed(() => segsT.value.find(s => s.segId === inspectSegId.value) ?? null)
const inspectLocalT = computed(() => {
  const c = inspectCol.value
  return c ? Math.max(0, inspectT.value - c.start) : 0
})

function toggleInspect() {
  if (!segsT.value.length) return
  if (inspect.value) { inspect.value = false; return }
  videoEl.value?.pause()
  const hit = segAt(segsT.value, cur.value)
  inspectSegId.value = hit?.segId ?? segsT.value[0].segId
  inspectT.value = cur.value
  inspect.value = true
}

// 元素点选 → 元素级 📎 引用（带上检视时刻）
function onPickElement(p: { segId: string; id: string; name: string }) {
  const c = segsT.value.find(s => s.segId === p.segId)
  if (!c) return
  emit('seg-element', { idx: c.idx, key: c.key, t: inspectT.value, elementId: p.id, elementName: p.name })
}

// 制作中帧文件在重写：退出检视；重做流程结束（stageSummary 消失）→ bump 重载
watch(() => props.stageSummary, (nv, ov) => {
  if (nv) inspect.value = false
  else if (ov) frameRev.value++
})
</script>

<template>
  <section class="stage">
    <!-- 舞台四态：文稿 → 分镜 → 风格样张 → 视频 -->
    <template v-if="view">
      <!-- 状态 1：文稿（前置未齐时的底稿展示） -->
      <div v-if="!view.gates.storyboard && manuscript" class="pane">
        <div class="pane-head">
          文稿 <NTag size="small" :bordered="false">{{ manuscript.word_count }} 字</NTag>
        </div>
        <NScrollbar class="ms-scroll">
          <article class="manuscript">{{ manuscript.content }}</article>
        </NScrollbar>
      </div>

      <!-- 状态 2：分镜表 -->
      <div v-else-if="view.gates.storyboard && !view.gates.style_draft && storyboard" class="pane">
        <div class="pane-head">
          分镜 <NTag size="small" :bordered="false">{{ storyboard.segments.length }} 段 / 约 {{ Math.round(view.total_hint) }} 秒</NTag>
        </div>
        <NScrollbar class="ms-scroll">
          <table class="sb">
            <thead><tr><th style="width:56px">段</th><th style="width:130px">标题</th><th>旁白</th><th style="width:170px">画面</th><th style="width:64px">秒</th></tr></thead>
            <tbody>
              <tr v-for="s in storyboard.segments" :key="s.id" class="sb-row" @click="emit('seg', s.idx, s.key)">
                <td class="c">{{ s.idx }}</td>
                <td><b>{{ s.key }}</b></td>
                <td class="narr">{{ s.narration }}</td>
                <td class="brief">{{ s.visual_brief }}</td>
                <td class="c">{{ Math.round(s.duration_hint) }}</td>
              </tr>
            </tbody>
          </table>
          <div class="hint">点任意段加入聊天引用列表（可删），随下一条消息发给 Agent</div>
        </NScrollbar>
      </div>

      <!-- 状态 3：风格样张（硬门：用户确认；制作中切到进度面板） -->
      <div v-else-if="view.gates.style_draft && styleSamples && !view.has_video && !view.producing && !stageSummary" class="pane">
        <div class="pane-head">
          风格样张 · {{ styleSamples.direction }}
          <NTag size="small" :bordered="false" :type="styleSamples.confirmed ? 'success' : 'warning'">
            {{ styleSamples.confirmed ? '已确认' : '待确认' }}
          </NTag>
        </div>
        <NScrollbar class="ms-scroll">
          <div class="samples">
            <figure v-for="s in styleSamples.samples" :key="s.tag" class="sample">
              <iframe :srcdoc="s.html" sandbox="" class="frame" :style="{ aspectRatio: view.aspect === '9:16' ? '9/16' : '16/9' }" />
              <figcaption>
                <b>{{ s.tag }}</b>
                <span>{{ s.desc }}</span>
              </figcaption>
            </figure>
          </div>
          <div class="confirm-bar">
            <p>样张与成片同一套 token，所见即所得。确认后即可开工（这是唯一必须你拍板的门）。</p>
            <NButton v-if="!styleSamples.confirmed" type="primary" size="large" @click="emit('confirm-style')">
              确认风格，开工
            </NButton>
            <NButton v-else type="success" ghost size="large" disabled>已确认</NButton>
          </div>
        </NScrollbar>
      </div>

      <!-- 状态 4：成片（含制作进度与时间轴分段条） -->
      <div v-else-if="view.has_video" class="pane video-pane" :class="{ vertical: view.aspect === '9:16' }">
        <!-- 段级重做中：细进度条（其余段的画面/音频不动） -->
        <div v-if="stageSummary" class="rework-strip">
          <div v-for="st in stageSummary" :key="st.key" class="pstage" :data-state="st.state">
            <span class="dot" />{{ stageName[st.key] ?? st.key }}
          </div>
          <span class="rework-hint">段级重做中</span>
          <button v-if="view.producing" class="stop-btn" @click="emit('cancel')">■ 停止</button>
        </div>
        <div class="tool-row">
          <button
            class="inspect-btn" :class="{ on: inspect }"
            :disabled="!!stageSummary || view.producing"
            @click="toggleInspect"
          >{{ inspect ? '退出检视' : '🔍 检视' }}</button>
          <button v-if="!inspect" class="inspect-btn time-ref" @click="pickTime(cur)">
            📎 引用此刻 {{ fmtClock(cur) }}
          </button>
          <span v-if="inspect" class="inspect-hint">活合成物：悬停显示元素名，点选加 📎 引用，时间轴拖动定位</span>
          <span v-else class="tool-hint">点「检视」指元素说话；或双击下方时间轴 / 点「引用此刻」钉住某个瞬间</span>
        </div>
        <div class="player">
          <video
            v-show="!inspect"
            ref="videoEl" :src="videoURL(videoId)" controls preload="metadata"
            @timeupdate="cur = videoEl!.currentTime" @loadedmetadata="dur = videoEl!.duration || 0"
          />
          <LiveFrame
            v-if="inspect && inspectSegId"
            :video-id="videoId" :seg-id="inspectSegId" :local-time="inspectLocalT" :rev="frameRev"
            @pick="onPickElement"
          />
        </div>
        <Timeline
          v-if="storyboard"
          :storyboard="storyboard" :audio-meta="audioMeta" :current-time="cur" :duration="dur"
          :picked-idx="pickedIdx" @seek="onSeek" @pick="pickSeg" @pick-time="pickTime"
        />
        <div class="foot-row">
          <span class="video-hint">点分镜/字幕/配音块或检视点元素加 📎 引用，聊天里说要改什么——只重做那一段</span>
          <a class="dl" :href="videoURL(videoId)" :download="`${view.name}.mp4`">下载成片 · {{ view.aspect === '9:16' ? '1080×1920' : '1080p' }}</a>
        </div>
      </div>

      <!-- 制作中：进度面板（其余状态都不满足 = 管线在跑或等待中） -->
      <div v-else class="pane producing">
        <NSpin size="large" />
        <div class="prod-title">{{ view.producing || stageSummary ? '制作管线运行中' : '准备中…' }}</div>
        <div v-if="stageSummary" class="stages">
          <div v-for="st in stageSummary" :key="st.key" class="pstage" :data-state="st.state">
            <span class="dot" />{{ stageName[st.key] ?? st.key }}
          </div>
        </div>
        <button v-if="view.producing" class="stop-btn" @click="emit('cancel')">■ 停止制作</button>
      </div>
    </template>

    <div v-else class="pane empty">
      <div class="none">← 左侧新建或选择一个项目</div>
    </div>
  </section>
</template>

<style scoped>
.stage { flex: 1; min-height: 0; padding: 16px 20px 20px; display: flex; }
.pane {
  flex: 1; min-width: 0; display: flex; flex-direction: column;
  background: #14141a; border: 1px solid #232329; border-radius: 12px; overflow: hidden;
}
.pane-head {
  flex: none; display: flex; align-items: center; gap: 10px;
  padding: 12px 18px; font-weight: 600; border-bottom: 1px solid #202027; color: #d9d9e0;
}
.ms-scroll { flex: 1; }
.manuscript { padding: 26px 34px; line-height: 2.1; font-size: 15px; color: #cfcfd8; white-space: pre-wrap; max-width: 860px; margin: 0 auto; font-family: 'KaiTi', 'STKaiti', serif; font-size: 17px; }

/* 分镜表 */
.sb { width: 100%; border-collapse: collapse; font-size: 13px; }
.sb th { position: sticky; top: 0; background: #14141a; text-align: left; padding: 10px 12px; color: #7c7c88; font-weight: 600; border-bottom: 1px solid #232329; }
.sb td { padding: 10px 12px; border-bottom: 1px solid #1d1d24; vertical-align: top; }
.sb-row { cursor: pointer; }
.sb-row:hover td { background: #191920; }
.c { color: #7c7c88; text-align: center; }
.narr { color: #c3c3cd; }
.brief { color: #8a8a96; }
.hint { padding: 10px 14px; color: #55555f; font-size: 12px; }

/* 样张 */
.samples { display: flex; gap: 16px; padding: 20px; flex-wrap: wrap; }
.sample { flex: 1; min-width: 320px; max-width: 480px; }
.frame { width: 100%; border: 1px solid #2b2b33; border-radius: 8px; background: #FDF6E3; }
figcaption { margin-top: 8px; display: flex; flex-direction: column; gap: 2px; }
figcaption b { font-size: 13px; }
figcaption span { font-size: 12px; color: #8a8a96; }
.confirm-bar { padding: 4px 20px 20px; display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.confirm-bar p { color: #8a8a96; font-size: 13px; max-width: 560px; }

/* 成片 */
.video-pane { align-items: center; justify-content: center; gap: 12px; padding: 16px 20px; }
.player { width: min(100%, 1080px); }
.player video { width: 100%; border-radius: 8px; background: #000; display: block; }
.video-pane.vertical .player { width: auto; flex: 1 1 0; min-height: 0; display: flex; justify-content: center; }
.video-pane.vertical .player video { width: auto; height: auto; max-width: 100%; max-height: 100%; }
/* 检视模式：LiveFrame 竖屏适配（横屏走组件内默认 16:9） */
.video-pane.vertical .player :deep(.live-frame),
.video-pane.vertical .player :deep(.live-loading),
.video-pane.vertical .player :deep(.live-err) {
  width: auto; height: 100%; max-width: 100%; aspect-ratio: 9 / 16;
}
.tool-row { width: min(100%, 1080px); display: flex; align-items: center; gap: 10px; min-height: 26px; }
.inspect-btn {
  flex: none; border: 1px solid #5c4d24; background: #211d12; color: #f0c674;
  font-size: 12px; padding: 3px 12px; border-radius: 999px; cursor: pointer;
}
.inspect-btn:hover { background: #2a2416; }
.inspect-btn.on { background: #f0c674; color: #14140f; font-weight: 600; }
.inspect-btn:disabled { opacity: .4; cursor: not-allowed; }
.inspect-hint { font-size: 12px; color: #f0c674; }
.tool-hint { font-size: 12px; color: #55555f; }
.rework-strip { display: flex; align-items: center; gap: 10px; }
.rework-hint { font-size: 12px; color: #f0c674; }
.foot-row { width: min(100%, 1080px); display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.video-hint { color: #55555f; font-size: 12px; }
.dl { color: #8fc7ff; font-size: 13px; text-decoration: none; border: 1px solid #2b4a66; padding: 6px 14px; border-radius: 999px; flex: none; }
.dl:hover { background: #121b24; }

/* 制作中（pstage = 进度胶囊；不能叫 .stage，会和外层舞台 section 撞类名导致 align/padding 污染） */
.producing { align-items: center; justify-content: center; gap: 18px; }
.prod-title { color: #c3c3cd; font-size: 15px; }
.stages { display: flex; gap: 10px; }
.pstage {
  display: flex; align-items: center; gap: 6px; font-size: 13px; color: #7c7c88;
  border: 1px solid #2b2b33; border-radius: 999px; padding: 5px 14px;
}
.dot { width: 8px; height: 8px; border-radius: 50%; background: #3a3a45; }
.pstage[data-state="running"] { color: #8fc7ff; border-color: #2b4a66; }
.pstage[data-state="running"] .dot { background: #8fc7ff; animation: pulse 1.2s infinite; }
.pstage[data-state="done"] { color: #7ee2a8; border-color: #2c5c40; }
.pstage[data-state="done"] .dot { background: #7ee2a8; }
.pstage[data-state="error"] { color: #ff9d9d; border-color: #66302b; }
.pstage[data-state="error"] .dot { background: #ff9d9d; }
.stop-btn {
  align-self: center; padding: 6px 16px; border-radius: 8px; cursor: pointer;
  color: #ffb3b3; background: #2a1a1c; border: 1px solid #5a3230; font-size: 13px;
}
.stop-btn:hover { background: #3a2224; border-color: #7a403e; }
@keyframes pulse { 50% { opacity: .35; } }
.empty { align-items: center; justify-content: center; }
.none { color: #55555f; }
</style>

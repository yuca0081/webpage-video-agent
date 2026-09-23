<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { NButton, NInput, NScrollbar, NSpin } from 'naive-ui'
import { api } from '../api'
import type { Msg } from '../types'

const props = defineProps<{ projectId: string; msgs: Msg[]; busy: boolean }>()
const draft = defineModel<string>('draft')
const emit = defineEmits<{ sent: [] }>()

const inputRef = ref<InstanceType<typeof NInput> | null>(null)
const listRef = ref<InstanceType<typeof NScrollbar> | null>(null)

async function send() {
  const text = (draft.value ?? '').trim()
  if (!text || !props.projectId) return
  draft.value = ''
  await api.chat(props.projectId, text).catch(() => {})
  emit('sent')
}

watch(() => props.msgs.length, () => nextTick(() => listRef.value?.scrollTo({ top: 1e9, behavior: 'smooth' })))
watch(draft, v => { // 📎 引用插入后聚焦
  if (v?.includes('📎')) inputRef.value?.focus()
})

const fmtTime = (iso: string) => (iso ? iso.slice(11, 16) : '')
</script>

<template>
  <aside class="chat">
    <div class="head">
      对话
      <span v-if="busy" class="busy"><NSpin :size="12" /> Agent 工作中</span>
    </div>
    <NScrollbar ref="listRef" class="list">
      <div v-if="!msgs.length" class="none">
        粘贴文稿建好项目后，直接说「开始」——<br />我会出分镜、出风格样张，你确认风格后开工出片。
      </div>
      <div v-for="m in msgs" :key="m.id" class="row" :class="m.role">
        <template v-if="m.type === 'tool_call'">
          <div class="tool">
            <span class="tname">🔧 {{ m.content }}</span>
          </div>
        </template>
        <template v-else-if="m.type === 'error'">
          <div class="bubble err">{{ m.content }}</div>
        </template>
        <template v-else>
          <div class="meta">{{ m.role === 'user' ? '我' : '帧述' }} · {{ fmtTime(m.created_at) }}</div>
          <div class="bubble" :class="m.role">{{ m.content }}</div>
        </template>
      </div>
      <div v-if="busy && !msgs.length" class="none"><NSpin :size="14" /></div>
    </NScrollbar>
    <div class="input">
      <NInput
        ref="inputRef" v-model:value="draft" type="textarea" :rows="3" :maxlength="4000"
        placeholder="对话或下指令（Enter 发送）｜时间轴点段可插入 📎 引用"
        @keydown.enter.exact.prevent="send"
      />
      <NButton type="primary" :disabled="!draft?.trim() || !projectId" @click="send">发送</NButton>
    </div>
  </aside>
</template>

<style scoped>
.chat {
  width: 400px; flex: none; display: flex; flex-direction: column;
  background: #121217; border-left: 1px solid #1d1d24;
}
.head {
  flex: none; height: 44px; display: flex; align-items: center; gap: 10px;
  padding: 0 16px; font-weight: 600; color: #d9d9e0; border-bottom: 1px solid #1d1d24;
}
.busy { display: flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 400; color: #8fc7ff; }
.list { flex: 1; padding: 14px 12px; }
.none { color: #55555f; font-size: 13px; line-height: 1.9; text-align: center; padding: 40px 20px; }
.row { margin-bottom: 12px; }
.meta { font-size: 11px; color: #55555f; margin: 0 6px 3px; }
.row.agent .meta { text-align: left; }
.row.user .meta { text-align: right; }
.bubble {
  padding: 9px 13px; border-radius: 12px; font-size: 13.5px; line-height: 1.75;
  white-space: pre-wrap; word-break: break-word; max-width: 92%;
}
.bubble.user { background: #2b3a52; color: #e8eef8; margin-left: auto; border-bottom-right-radius: 4px; }
.bubble.agent { background: #1c1c23; color: #c9c9d1; border-bottom-left-radius: 4px; }
.bubble.err { background: #2a1a1a; color: #ff9d9d; }
.tool { text-align: center; }
.tname {
  font-size: 11px; color: #8a8a96; background: #17171d; border: 1px dashed #2b2b33;
  border-radius: 999px; padding: 3px 12px;
}
.input { flex: none; display: flex; gap: 8px; padding: 10px; border-top: 1px solid #1d1d24; align-items: flex-end; }
</style>

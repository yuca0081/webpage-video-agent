<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NInput, NModal, NRadioButton, NRadioGroup, NSelect } from 'naive-ui'
import { api } from '../api'
import type { StylePack } from '../types'

const show = defineModel<boolean>('show')
const emit = defineEmits<{ created: [id: string] }>()
const name = ref('')
const manuscript = ref('')
const aspect = ref('9:16')
const stylepack = ref('')
const creating = ref(false)
const error = ref('')
const packs = ref<StylePack[]>([])

watch(show, async v => {
  if (v) {
    name.value = ''; manuscript.value = ''; stylepack.value = ''; error.value = ''
    packs.value = await api.listLibrary().catch(() => [])
  }
})

const packOptions = () => [
  { label: 'AI 按文稿气质现定（冷启动）', value: '' },
  ...packs.value.filter(p => p.published).map(p => ({ label: p.name, value: p.id })),
]

async function create() {
  if (!manuscript.value.trim()) { error.value = '文稿必填（前置硬门 1/3）'; return }
  creating.value = true
  error.value = ''
  try {
    const r = await api.createProject(name.value.trim(), manuscript.value, aspect.value, stylepack.value)
    emit('created', r.id)
  } catch (e: any) {
    error.value = e.message ?? String(e)
    return
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" title="新建项目 · 粘贴文稿" class="modal" :mask-closable="!creating">
    <div class="body">
      <NInput v-model:value="name" placeholder="项目名（留空取文稿首行）" />
      <NInput
        v-model:value="manuscript" type="textarea" :rows="14" :maxlength="200000"
        placeholder="把文章全文粘进来——这就是视频文稿（旁白只来自这里，忠实原文）"
      />
      <div class="opts">
        <div class="opt">
          <span class="opt-label">画幅</span>
          <NRadioGroup v-model:value="aspect" size="small">
            <NRadioButton value="9:16" title="竖屏，手机优先">9:16 竖屏</NRadioButton>
            <NRadioButton value="16:9" title="横屏，PC/大屏">16:9 横屏</NRadioButton>
          </NRadioGroup>
        </div>
        <div class="opt">
          <span class="opt-label">风格</span>
          <NSelect v-model:value="stylepack" size="small" :options="packOptions()" placeholder="选择方法库风格" style="width: 260px" />
        </div>
      </div>
      <div v-if="error" class="err">{{ error }}</div>
      <div class="foot">
        <span class="count">{{ manuscript.length }} 字 · 预计 {{ Math.max(1, Math.round(manuscript.length / 4.2 / 60)) }} 分钟</span>
        <NButton type="primary" :loading="creating" @click="create">创建</NButton>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.body { display: flex; flex-direction: column; gap: 12px; }
.opts { display: flex; justify-content: space-between; align-items: center; gap: 16px; flex-wrap: wrap; }
.opt { display: flex; align-items: center; gap: 10px; }
.opt-label { font-size: 12px; color: #7c7c88; }
.err { color: #ff9d9d; font-size: 13px; }
.foot { display: flex; justify-content: space-between; align-items: center; }
.count { color: #7c7c88; font-size: 12px; }
.modal { width: 640px; }
</style>

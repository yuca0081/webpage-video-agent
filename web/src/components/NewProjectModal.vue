<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NInput, NModal, NRadioButton, NRadioGroup, NSelect } from 'naive-ui'
import { api } from '../api'
import type { StylePack } from '../types'

const show = defineModel<boolean>('show')
const emit = defineEmits<{ created: [id: string] }>()
const mode = ref<'paste' | 'topic'>('paste')
const name = ref('')
const manuscript = ref('')
const aspect = ref('9:16')
const stylepack = ref('')
const creating = ref(false)
const error = ref('')
const packs = ref<StylePack[]>([])
// AI 起草（只有主题）
const topic = ref('')
const minutes = ref(1)
const drafting = ref(false)

watch(show, async v => {
  if (v) {
    mode.value = 'paste'
    name.value = ''; manuscript.value = ''; topic.value = ''; stylepack.value = ''; error.value = ''
    packs.value = await api.listLibrary().catch(() => [])
  }
})

const packOptions = () => [
  { label: 'AI 按文稿气质现定（冷启动）', value: '' },
  ...packs.value.filter(p => p.published).map(p => ({ label: p.name, value: p.id })),
]

async function draft() {
  if (!topic.value.trim() || drafting.value) return
  drafting.value = true
  error.value = ''
  try {
    const r = await api.draftManuscript(topic.value.trim(), minutes.value)
    manuscript.value = r.content
    mode.value = 'paste' // 起草完回到文稿框，用户可直接改
  } catch (e: any) {
    error.value = e.message ?? String(e)
  } finally {
    drafting.value = false
  }
}

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
  <NModal v-model:show="show" preset="card" title="新建视频" class="np-modal" :mask-closable="!creating">
    <div class="body">
      <NRadioGroup v-model:value="mode" size="small">
        <NRadioButton value="paste">我有文稿，直接粘贴</NRadioButton>
        <NRadioButton value="topic">只有主题，AI 帮我起草</NRadioButton>
      </NRadioGroup>

      <div class="field">
        <span class="label">项目名 <em>留空取文稿首行</em></span>
        <NInput v-model:value="name" placeholder="例：蜂巢为什么是六边形" />
      </div>

      <!-- 只有主题：主题 + 时长 → AI 起草 -->
      <template v-if="mode === 'topic'">
        <div class="field">
          <span class="label">主题 <em>一句话说清要讲什么</em></span>
          <NInput
            v-model:value="topic" placeholder="一句话主题，例：蚊子为什么嗡嗡叫"
            @keydown.enter.prevent="draft"
          />
        </div>
        <div class="opts">
          <div class="opt">
            <span class="label">时长</span>
            <NSelect
              v-model:value="minutes" size="small" style="width: 130px"
              :options="[{ label: '约 1 分钟', value: 1 }, { label: '约 2 分钟', value: 2 }, { label: '约 3 分钟', value: 3 }]"
            />
          </div>
          <NButton size="small" :loading="drafting" @click="draft">AI 起草文稿</NButton>
          <span class="draft-hint">起草后进文稿框，可随意改</span>
        </div>
      </template>

      <!-- 文稿（两种模式共用：粘贴直填，起草后回填可改） -->
      <div class="field">
        <span class="label">
          文稿 <em>旁白只来自这里，忠实原文</em>
          <button v-if="mode === 'paste' && manuscript" class="link" @click="mode = 'topic'">换个主题重起草</button>
        </span>
        <NInput
          v-model:value="manuscript" type="textarea" :rows="10" :maxlength="200000"
          :placeholder="mode === 'topic' ? '点上方「AI 起草文稿」自动填入，或直接粘贴' : '把文章全文粘进来——这就是视频文稿'"
        />
      </div>

      <div class="opts">
        <div class="opt">
          <span class="label">画幅</span>
          <NRadioGroup v-model:value="aspect" size="small">
            <NRadioButton value="9:16" title="竖屏，手机优先">9:16 竖屏</NRadioButton>
            <NRadioButton value="16:9" title="横屏，PC/大屏">16:9 横屏</NRadioButton>
          </NRadioGroup>
        </div>
        <div class="opt grow">
          <span class="label">风格</span>
          <NSelect v-model:value="stylepack" size="small" :options="packOptions()" style="flex: 1; min-width: 0" />
        </div>
      </div>
      <div v-if="error" class="err">{{ error }}</div>
      <div class="foot">
        <span class="count">{{ manuscript.length }} 字 · 预计 {{ Math.max(1, Math.round(manuscript.length / 4.2 / 60)) }} 分钟</span>
        <NButton type="primary" :disabled="!manuscript.trim()" :loading="creating" @click="create">创建</NButton>
      </div>
    </div>
  </NModal>
</template>

<!-- modal 挂在 body 下，样式需全局作用域 -->
<style>
.np-modal {
  width: 620px; background: #14141a; border: 1px solid #26262e; border-radius: 14px;
  --n-padding-left: 22px; --n-padding-right: 22px; --n-padding-top: 20px; --n-padding-bottom: 20px;
}
.np-modal .n-card-header { padding: 18px 22px 0; }
.np-modal .n-card-header__main { font-weight: 700; letter-spacing: 2px; color: #f0c674; font-size: 15px; }
.np-modal .n-card__content { padding: 14px 22px 20px; }
.np-modal .n-base-close { color: #7c7c88; top: 16px; right: 16px; }
.np-modal .n-base-close:hover { color: #e6e6ea; }
.np-modal .n-input { background: #101016; border: 1px solid #232329; border-radius: 10px; }
.np-modal .n-input:hover { border-color: #3a3a45; }
.np-modal .n-input--focus { border-color: rgba(240, 198, 116, .55); }
</style>

<style scoped>
.body { display: flex; flex-direction: column; gap: 14px; }
.field { display: flex; flex-direction: column; gap: 6px; }
.label { font-size: 12px; color: #9a9aa6; display: flex; align-items: baseline; gap: 8px; }
.label em { font-style: normal; font-size: 11px; color: #55555f; }
.link {
  border: 0; background: transparent; cursor: pointer; font-size: 11px; color: #8fc7ff; padding: 0;
}
.link:hover { text-decoration: underline; }
.opts { display: flex; align-items: center; gap: 20px; flex-wrap: wrap; }
.opt { display: flex; align-items: center; gap: 10px; }
.opt.grow { flex: 1; min-width: 260px; }
.draft-hint { font-size: 11px; color: #55555f; }
.err { color: #ff9d9d; font-size: 13px; }
.foot { display: flex; justify-content: space-between; align-items: center; margin-top: 2px; }
.count { color: #7c7c88; font-size: 12px; }
</style>

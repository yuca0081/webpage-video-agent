<script setup lang="ts">
import { NButton, NTag } from 'naive-ui'
import type { StylePack } from '../types'

defineProps<{ packs: StylePack[] }>()
const emit = defineEmits<{ publish: [id: string] }>()
</script>

<template>
  <section class="lib">
    <header class="head">
      <h2>方法库</h2>
      <p>成片提炼的风格包，确认入库后可在新建项目时复用——越用越准的飞轮。</p>
    </header>
    <div v-if="!packs.length" class="none">
      还没有条目（冷启动零预置）。出一条片、点「出片」后，Agent 会自动起草风格包，到这里确认入库。
    </div>
    <div v-else class="grid">
      <figure v-for="p in packs" :key="p.id" class="card" :class="{ draft: !p.published }">
        <iframe :srcdoc="p.sample_html" sandbox="" class="sample" />
        <figcaption>
          <div class="row1">
            <b>{{ p.name }}</b>
            <NTag size="small" :bordered="false" :type="p.published ? 'success' : 'warning'">
              {{ p.published ? '已入库' : '草稿' }}
            </NTag>
          </div>
          <span class="desc">{{ p.desc }}</span>
          <div class="row2">
            <span class="origin">来源 {{ p.origin_project }}</span>
            <NButton v-if="!p.published" size="small" type="primary" ghost @click="emit('publish', p.id)">
              入库
            </NButton>
          </div>
        </figcaption>
      </figure>
    </div>
  </section>
</template>

<style scoped>
.lib { flex: 1; min-height: 0; overflow: auto; padding: 24px 28px; }
.head h2 { font-size: 16px; color: #d9d9e0; }
.head p { font-size: 12.5px; color: #7c7c88; margin: 4px 0 18px; }
.none { color: #55555f; font-size: 13px; padding: 40px 0; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.card {
  background: #14141a; border: 1px solid #232329; border-radius: 12px; overflow: hidden;
}
.card.draft { border-color: #5c4d24; }
.sample { width: 100%; aspect-ratio: 16/9; display: block; border: 0; background: #FDF6E3; }
figcaption { padding: 10px 12px 12px; display: flex; flex-direction: column; gap: 6px; }
.row1 { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.row1 b { font-size: 13px; }
.desc { font-size: 11.5px; color: #8a8a96; }
.row2 { display: flex; align-items: center; justify-content: space-between; }
.origin { font-size: 11px; color: #55555f; }
</style>

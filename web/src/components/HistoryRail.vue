<script setup lang="ts">
import { NBadge, NButton, NIcon, NPopover, NScrollbar, NEmpty } from 'naive-ui'
import type { ProjectRow } from '../types'

defineProps<{ projects: ProjectRow[]; current: string }>()
const emit = defineEmits<{ select: [id: string]; new: [] }>()
</script>

<template>
  <aside class="rail">
    <div class="logo">帧</div>
    <NScrollbar class="list">
      <div v-if="!projects.length" class="empty">暂无项目</div>
      <NPopover v-for="p in projects" :key="p.id" placement="right" :delay="200" trigger="hover">
        <template #trigger>
          <div
            class="item" :class="{ active: p.id === current }"
            @click="emit('select', p.id)"
          >{{ p.name.slice(0, 2) }}</div>
        </template>
        <div class="pop">
          <div class="pop-name">{{ p.name }}</div>
          <div class="pop-meta">{{ p.status }}{{ p.has_video ? ' · 有成片' : '' }}</div>
        </div>
      </NPopover>
    </NScrollbar>
    <NButton quaternary circle size="large" class="new" @click="emit('new')">
      <span class="plus">＋</span>
    </NButton>
  </aside>
</template>

<style scoped>
.rail {
  width: 56px; flex: none; display: flex; flex-direction: column; align-items: center;
  padding: 10px 0; gap: 10px; background: #0a0a0e; border-right: 1px solid #1d1d24;
}
.logo {
  width: 36px; height: 36px; border-radius: 10px; display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, #f0c674, #c89b4b); color: #14140f; font-weight: 700; font-size: 18px;
}
.list { flex: 1; width: 100%; }
.empty { color: #55555f; font-size: 11px; text-align: center; padding-top: 20px; }
.item {
  width: 36px; height: 36px; margin: 4px auto; border-radius: 10px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  background: #1a1a21; color: #b9b9c4; font-size: 13px; border: 1px solid transparent;
  overflow: hidden; transition: border-color .15s;
}
.item:hover { border-color: #3a3a45; }
.item.active { border-color: #f0c674; color: #f0c674; }
.new { color: #7c7c88; }
.plus { font-size: 20px; line-height: 1; }
.pop { max-width: 220px; }
.pop-name { font-size: 13px; font-weight: 600; }
.pop-meta { font-size: 11px; color: #888; margin-top: 2px; }
</style>

import type { AudioMeta, ChatRef, ElementInfo, Msg, ProjectRow, ProjectView, Storyboard, StyleInfo, StylePack, StyleSamples } from './types'

async function j<T>(res: Response): Promise<T> {
  if (!res.ok) throw new Error((await res.json().catch(() => ({} as any))).error ?? res.statusText)
  return res.json()
}

export const api = {
  listProjects: () => fetch('/api/projects').then(r => j<ProjectRow[]>(r)),

  createProject: (name: string, manuscript: string, aspect = '9:16', stylepackId = '') =>
    fetch('/api/projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, manuscript, aspect, stylepack_id: stylepackId }),
    }).then(r => j<{ id: string; name: string }>(r)),

  project: (id: string) => fetch(`/api/projects/${id}`).then(r => j<ProjectView>(r)),

  storyboard: (id: string) => fetch(`/api/projects/${id}/storyboard`).then(r => j<Storyboard>(r)),

  audioMeta: (id: string) => fetch(`/api/projects/${id}/audio_meta`).then(r => j<AudioMeta>(r)),

  manuscript: (id: string) =>
    fetch(`/api/projects/${id}/manuscript`).then(r => j<{ content: string; word_count: number }>(r)),

  style: (id: string) => fetch(`/api/projects/${id}/style`).then(r => j<StyleSamples>(r)),

  confirmStyle: (id: string) =>
    fetch(`/api/projects/${id}/style/confirm`, { method: 'POST' }).then(r => j<{ ok: boolean }>(r)),

  chat: (id: string, content: string, refs: ChatRef[] = []) =>
    fetch(`/api/projects/${id}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, refs }),
    }).then(r => j<{ ok: boolean }>(r)),

  frameText: (id: string, seg: string) =>
    fetch(`/api/projects/${id}/frames/${seg}`).then(r => {
      if (!r.ok) throw new Error(r.statusText)
      return r.text()
    }),

  draftManuscript: (topic: string, minutes = 1) =>
    fetch('/api/draft/manuscript', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ topic, minutes }),
    }).then(r => j<{ content: string }>(r)),

  messages: (id: string, after = 0) =>
    fetch(`/api/projects/${id}/messages?after=${after}`).then(r => j<Msg[]>(r)),

  produce: (id: string) =>
    fetch(`/api/projects/${id}/produce`, { method: 'POST' }).then(r => j<{ ok: boolean }>(r)),

  cancel: (id: string) =>
    fetch(`/api/projects/${id}/cancel`, { method: 'POST' }).then(r => j<{ ok: boolean }>(r)),

  listLibrary: () => fetch('/api/library').then(r => j<StylePack[]>(r)),

  publishPack: (id: string) =>
    fetch(`/api/library/${id}/publish`, { method: 'POST' }).then(r => j<StylePack>(r)),

  registry: () => fetch('/api/registry').then(r => j<{ elements: ElementInfo[]; styles: StyleInfo[] }>(r)),

  gallery: () => fetch('/api/gallery').then(r => j<Record<string, string[]>>(r)),
}

export const videoURL = (id: string) => `/api/projects/${id}/video/main.mp4`
// 活合成物壳的确定性依赖：项目内本地 gsap（与渲染引擎同一份，无 CDN）
export const assetURL = (id: string, p: string) => `/api/projects/${id}/assets/${p}`
// 素材中心样张：data/gallery/<style>/<kind>.png
export const galleryURL = (style: string, kind: string) => `/api/gallery/${style}/${kind}.png`

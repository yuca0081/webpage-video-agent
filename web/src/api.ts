import type { Msg, ProjectRow, ProjectView, Storyboard, StylePack, StyleSamples } from './types'

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

  manuscript: (id: string) =>
    fetch(`/api/projects/${id}/manuscript`).then(r => j<{ content: string; word_count: number }>(r)),

  style: (id: string) => fetch(`/api/projects/${id}/style`).then(r => j<StyleSamples>(r)),

  confirmStyle: (id: string) =>
    fetch(`/api/projects/${id}/style/confirm`, { method: 'POST' }).then(r => j<{ ok: boolean }>(r)),

  chat: (id: string, content: string) =>
    fetch(`/api/projects/${id}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content }),
    }).then(r => j<{ ok: boolean }>(r)),

  messages: (id: string, after = 0) =>
    fetch(`/api/projects/${id}/messages?after=${after}`).then(r => j<Msg[]>(r)),

  produce: (id: string) =>
    fetch(`/api/projects/${id}/produce`, { method: 'POST' }).then(r => j<{ ok: boolean }>(r)),

  listLibrary: () => fetch('/api/library').then(r => j<StylePack[]>(r)),

  publishPack: (id: string) =>
    fetch(`/api/library/${id}/publish`, { method: 'POST' }).then(r => j<StylePack>(r)),
}

export const videoURL = (id: string) => `/api/projects/${id}/video/main.mp4`

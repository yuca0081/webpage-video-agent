export interface ProjectRow {
  id: string
  name: string
  status: string
  created_at: string
  has_video: boolean
}

export interface ProjectView {
  id: string
  name: string
  aspect: string
  gates: {
    manuscript: boolean
    storyboard: boolean
    style_confirmed: boolean
    style_draft: boolean
    style_direction: string
  }
  seg_count: number
  total_hint: number
  has_video: boolean
  producing: boolean
}

export interface Segment {
  id: string
  idx: number
  key: string
  narration: string
  visual_brief: string
  duration_hint: number
}

export interface Storyboard {
  timing_basis: string
  style_id: string
  plan: string
  segments: Segment[]
}

export interface StyleSample {
  tag: string
  desc: string
  html: string
}

export interface StyleSamples {
  direction: string
  desc: string
  samples: StyleSample[]
  confirmed: boolean
}

export interface Msg {
  id: number
  role: 'user' | 'agent' | 'system'
  type: 'text' | 'tool_call' | 'progress' | 'error'
  content: string
  refs?: ChatRef[]
  tool_name?: string
  tool_args?: string
  created_at: string
}

export interface StylePack {
  id: string
  name: string
  direction: string
  desc: string
  sample_html: string
  sample_tag: string
  origin_project: string
  published: boolean
  created_at: string
}

// 音频对齐产物（audio/audio_meta.json）：时间轴字幕/音频轨的数据源
export interface AlignWord {
  text: string
  start: number
  end: number
}

export interface AudioVoice {
  id: string
  path: string
  duration_s: number
  words: AlignWord[]
}

export interface AudioMeta {
  tts_provider: string
  voice_id: string
  voices: AudioVoice[]
}

// 聊天输入框上方的引用项（时间轴/分镜/检视点选加入，随消息结构化发给 Agent）
// 三态：段级（idx+key）/ 时刻级（idx+key+t）/ 元素级（+elementId+elementName）
export interface ChatRef {
  idx: number
  key: string
  t?: number           // 全局时刻（秒），时刻/元素引用携带
  elementId?: string   // 元素 DOM id（如 seg03-arrow2）
  elementName?: string // 人话名（如 箭头：蓝光弹开）
}

// 素材中心：元素注册表条目（ai/registry/elements.json）
export interface ElementInfo {
  kind: string
  name: string
  category: string
  scene: string
  menu: string // LLM prompt 菜单行（kind 与参数约束的权威文本）
}

// 素材中心：风格注册表条目（ai/registry/styles.json 剥 tokens 后的元信息）
export interface StyleInfo {
  id: string
  direction: string
  genre: string
  keywords: string[]
  photo: string
}

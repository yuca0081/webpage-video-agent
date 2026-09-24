<script setup lang="ts">
// 修改态活合成物（plan §4.3 预览双模）：
// 把磁盘上的单段帧 HTML（<template> 片段，惰性）包进一个可执行文档（srcdoc 壳），
// 壳内引项目本地 gsap（与渲染引擎同一份）+ 激活/交互脚本 —— 时间轴 seek、悬停高亮、点选命名元素。
// sandbox="allow-scripts"（无同源）：父子只走 postMessage，协议 live:ready / live:seek / live:pick / live:error。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, assetURL } from '../api'

const props = defineProps<{
  videoId: string
  segId: string // 'seg03'
  localTime: number // 段内时刻（秒）
  rev?: number // 重做完成后 bump，强制重载帧
}>()
const emit = defineEmits<{
  ready: [segId: string, duration: number]
  pick: [p: { segId: string; id: string; name: string; t: number }]
}>()

const iframeEl = ref<HTMLIFrameElement | null>(null)
const srcdoc = ref('')
const errText = ref('')
const readySeg = ref('')

let pendingT: number | null = null
let raf = 0

function postSeek(t: number) {
  pendingT = t
  if (!raf) raf = requestAnimationFrame(flush)
}
function flush() {
  raf = 0
  if (pendingT == null) return
  if (readySeg.value !== props.segId || !iframeEl.value?.contentWindow) return // 未 ready：挂起，ready 后应用
  iframeEl.value.contentWindow.postMessage({ type: 'live:seek', t: pendingT }, '*')
  pendingT = null
}

async function load() {
  readySeg.value = ''
  errText.value = ''
  srcdoc.value = ''
  const frag = await api
    .frameText(props.videoId, props.segId)
    .catch(() => null)
  if (frag == null) {
    errText.value = `段 ${props.segId} 合成物加载失败`
    return
  }
  srcdoc.value = shellDoc(frag)
}

// srcdoc 壳：gsap 本地引 + 帧片段原样 + 激活/交互脚本（ES5 风格，避免依赖构建期语法）
function shellDoc(frag: string): string {
  const gsap = assetURL(props.videoId, 'gsap.min.js')
  return (
    '<!doctype html><html><head><meta charset="utf-8">' +
    '<script src="' + gsap + '"><\/script>' +
    '<style>' +
    'html,body{margin:0;width:100%;height:100%;background:#000;overflow:hidden}' +
    '#__viewport{position:absolute;inset:0;display:flex;align-items:center;justify-content:center}' +
    '#__stage{position:relative;transform-origin:center center}' +
    '[data-hf-hover]{outline:2px solid #f0c674!important;outline-offset:2px;cursor:pointer}' +
    '#__badge{position:fixed;z-index:9999;display:none;pointer-events:none;white-space:nowrap;' +
    'background:#f0c674;color:#14141a;font:12px/1.4 system-ui,sans-serif;padding:3px 8px;border-radius:4px}' +
    '<\/style></head><body>' +
    '<div id="__viewport"><div id="__stage"></div></div>' +
    '<div id="__badge"></div>' +
    frag +
    '<script>' + agentScript(props.segId) + '<\/script>' +
    '</body></html>'
  )
}

function agentScript(seg: string): string {
  return (
    '(function(){' +
    'var SEG=' + JSON.stringify(seg) + ';' +
    'function post(m){parent.postMessage(m,"*")}' +
    // ① 激活：__timelines 先于片段脚本初始化（index.html 同款一行）；template 内容搬家，script 重建执行
    'window.__timelines=window.__timelines||{};' +
    'var tpl=document.querySelector("template");' +
    'if(!tpl){post({type:"live:error",seg:SEG,error:"帧缺 template"});return}' +
    'var stage=document.getElementById("__stage");' +
    'Array.prototype.slice.call(tpl.content.childNodes).forEach(function(n){' +
    'if(n.nodeType!==1)return;' +
    'if(n.tagName==="SCRIPT"){var s=document.createElement("script");s.textContent=n.textContent;document.body.appendChild(s)}' +
    'else if(n.tagName==="STYLE"){document.head.appendChild(n)}' +
    'else{stage.appendChild(n)}' +
    '});' +
    'var tl=window.__timelines[SEG];' +
    'if(!tl){post({type:"live:error",seg:SEG,error:"时间轴未注册"});return}' +
    'tl.pause(0);' +
    // ② 缩放适配：合成物根 data-width/height → 等比放进 iframe 视口
    'var comp=stage.querySelector("[data-composition-id]");' +
    'var W=parseFloat(comp.getAttribute("data-width"))||1920;' +
    'var H=parseFloat(comp.getAttribute("data-height"))||1080;' +
    'comp.style.display="block";comp.style.width=W+"px";comp.style.height=H+"px";' +
    'stage.style.width=W+"px";stage.style.height=H+"px";' +
    'function fit(){var k=Math.min(innerWidth/W,innerHeight/H);stage.style.transform="scale("+k+")"}' +
    'addEventListener("resize",fit);fit();' +
    // ③ 悬停高亮 + 人话名徽标；点选上报（沿祖先找最近的 data-hf-name）
    'var badge=document.getElementById("__badge"),hovered=null;' +
    'function hitOf(el){for(var n=el;n&&n!==document;n=n.parentElement){' +
    'if(n.getAttribute&&n.getAttribute("data-hf-name"))return{el:n,name:n.getAttribute("data-hf-name")}}return null}' +
    'document.addEventListener("mousemove",function(e){' +
    'var h=hitOf(e.target);' +
    'if(h&&h.el!==hovered){if(hovered)hovered.removeAttribute("data-hf-hover");' +
    'hovered=h.el;hovered.setAttribute("data-hf-hover","1");' +
    'badge.textContent=h.name;badge.style.display="block"}' +
    'else if(!h&&hovered){hovered.removeAttribute("data-hf-hover");hovered=null;badge.style.display="none"}' +
    'if(hovered){badge.style.left=Math.min(e.clientX+12,innerWidth-badge.offsetWidth-8)+"px";' +
    'badge.style.top=Math.max(8,e.clientY-28)+"px"}});' +
    'document.addEventListener("click",function(e){' +
    'var h=hitOf(e.target);if(!h)return;' +
    'e.preventDefault();e.stopPropagation();' +
    'post({type:"live:pick",seg:SEG,id:h.el.id||"",name:h.name,t:tl.time()})});' +
    // ④ seek 协议（rAF 合并拖动擦洗）
    'var pend=null,raf=0;' +
    'function flush(){raf=0;if(pend!=null){tl.pause(pend);pend=null}}' +
    'addEventListener("message",function(e){var d=e.data||{};' +
    'if(d.type==="live:seek"){pend=d.t;if(!raf)raf=requestAnimationFrame(flush)}});' +
    'post({type:"live:ready",seg:SEG,duration:tl.duration()});' +
    '})();'
  )
}

function onMessage(e: MessageEvent) {
  if (!iframeEl.value || e.source !== iframeEl.value.contentWindow) return
  const d = e.data as { type?: string; seg?: string; duration?: number; id?: string; name?: string; t?: number; error?: string }
  switch (d.type) {
    case 'live:ready':
      if (d.seg) readySeg.value = d.seg
      emit('ready', d.seg ?? props.segId, d.duration ?? 0)
      flush() // 应用挂起的 seek
      break
    case 'live:pick':
      emit('pick', { segId: d.seg ?? props.segId, id: d.id ?? '', name: d.name ?? '', t: d.t ?? 0 })
      break
    case 'live:error':
      errText.value = d.error ?? '活合成物初始化失败'
      break
  }
}

onMounted(() => window.addEventListener('message', onMessage))
onBeforeUnmount(() => {
  window.removeEventListener('message', onMessage)
  if (raf) cancelAnimationFrame(raf)
})

watch(() => [props.segId, props.rev, props.videoId], load, { immediate: true })
watch(() => props.localTime, postSeek)
</script>

<template>
  <div v-if="errText" class="live-err">{{ errText }}</div>
  <iframe
    v-else-if="srcdoc"
    ref="iframeEl"
    :key="`${segId}-${rev ?? 0}`"
    class="live-frame"
    :srcdoc="srcdoc"
    sandbox="allow-scripts"
  />
  <div v-else class="live-loading">活合成物载入中…</div>
</template>

<style scoped>
.live-frame {
  width: 100%;
  border: 0;
  border-radius: 8px;
  background: #000;
  display: block;
  aspect-ratio: 16 / 9;
}
.live-loading,
.live-err {
  width: 100%;
  aspect-ratio: 16 / 9;
  border-radius: 8px;
  background: #0a0a0e;
  color: #7c7c88;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
}
.live-err { color: #ff9d9d; }
</style>

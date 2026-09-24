# -*- coding: utf-8 -*-
"""spec → 合成物 HTML 渲染器（M1 确定性渲染层）。

LLM 只产出语义 spec（llm/comp-segNN.spec.json：元素种类 + 坐标 + 文案 + 揭示词位），
本脚本把 spec 经 StylePack 引擎渲染成 llm/comp-segNN.json（{html, elements}），
随后 m0 run compositions 落盘到 compositions/frames/。

用法：python ai/render_spec.py <项目目录>
"""
import json
import math
import pathlib
import re
import sys

SHARED = pathlib.Path(__file__).resolve().parent.parent / 'data' / 'projects' / '_shared'
sys.path.insert(0, str(SHARED))
import stylepack as sp  # noqa: E402  默认引擎（手绘叙事）；main 按项目风格方向分发

COLORS = {}
CHART_FILLS = []
PREFIX = 's'


def init_engine(proj):
    """按项目 style_samples.json 的 direction 选风格引擎，重建色表。

    direction 含「扁平」→ stylepack_flat（厚描边·色块）；否则手绘叙事。
    """
    global sp, COLORS, CHART_FILLS
    d = ''
    try:
        with open(proj / 'style' / 'style_samples.json', encoding='utf-8') as fh:
            d = (json.load(fh) or {}).get('direction', '')
    except Exception:
        d = ''
    if '扁平' in d:
        import stylepack_flat
        sp = stylepack_flat
    else:
        import stylepack
        sp = stylepack
    COLORS = {'butter': sp.BUTTER, 'mint': sp.MINT, 'sky': sp.SKY, 'coral': sp.CORAL,
              'peach': sp.PEACH, 'pink': sp.PINK, 'turq': sp.TURQ, 'ink': sp.INK,
              'white': '#FFFFFF', 'navy': getattr(sp, 'NAVY', sp.INK),
              'green': getattr(sp, 'GREEN', sp.TURQ)}
    CHART_FILLS = [sp.MINT, sp.SKY, sp.BUTTER, sp.CORAL, sp.PEACH, sp.PINK]


def col(name, default):
    return COLORS.get(name or '', default)


def reveal_t(words, idx):
    """词位 → 揭示时刻（提前 0.12s，留入场时间）。"""
    if not words:
        return 0.05
    idx = max(0, min(int(idx), len(words) - 1))
    return max(0.05, round(words[idx]['start'] - 0.12, 2))


def render_element(f, sid, e, i, words, W=1920):
    """spec 元素 → (html 片段, js 片段)。id 规则 {sid}-{kind}{i}。"""
    kind = e.get('kind')
    eid = f"{sid}-{kind}{i}"
    t = reveal_t(words, e.get('reveal', 0))
    text = str(e.get('text', '')).strip()
    if kind == 'title':
        fs = e.get('fs', 84)
        if W <= 1200 and fs > 72:  # 竖屏窄幅：标题降字号防溢出
            fs = 72
        html = (f'<div class="{f}-title {f}-el" id="{eid}" data-hf-name="标题：{text[:12]}" '
                f'style="top:{e["y"]}px;font-size:{fs}px;">{text}</div>')
        return html, sp.rise(f'#{eid}', t, dy=26)
    if kind == 'note':
        bg, fs, rot = col(e.get('bg'), sp.BUTTER), e.get('fs', 40), e.get('rot', -1.5)
        html = sp.note(f, eid, e['x'], e['y'], text, bg=bg, rot=rot, fs=fs)
        return html, sp.rise(f'#{eid}', t)
    if kind == 'label':
        fs = e.get('fs', 38)
        color = col(e.get('color'), sp.INK)
        html = sp.label(f, eid, e['x'], e['y'], text, color=color, fs=fs)
        return html, sp.fade(f'#{eid}', t)
    if kind == 'big':
        fs = e.get('fs', 110)
        color = col(e.get('color'), getattr(sp, 'BIG', '#B45309'))
        html = sp.big(f, eid, e['x'], e['y'], text, fs=fs, color=color)
        return html, sp.pop(f'#{eid}', t)
    if kind == 'beam':
        bg, rot = col(e.get('bg'), '#FFFFFF'), e.get('rot', 0)
        html = sp.beam(f, eid, e['x'], e['y'], e['w'], h=e.get('h', 22), bg=bg, rot=rot)
        js = sp.draw_x(f'#{eid}', t) if abs(rot) < 0.5 else sp.fade(f'#{eid}', t)
        return html, js
    if kind == 'disc':
        html = sp.disc(f, eid, e['cx'], e['cy'], e['r'], bg=col(e.get('bg'), sp.MINT))
        return html, sp.pop(f'#{eid}', t, scale=.4)
    if kind == 'circle':  # 实心圆点（替代 emoji 的人物/气泡等）
        r, fill = e.get('r', 36), col(e.get('fill'), '#FFFFFF')
        html = (f'<div class="{f}-el" id="{eid}" data-hf-name="圆点" '
                f'style="top:{e["cy"] - r}px;left:{e["cx"] - r}px;width:{2 * r}px;height:{2 * r}px;'
                f'background:{fill};border:3px solid {sp.INK};border-radius:50%;"></div>')
        return html, sp.fade(f'#{eid}', t)
    if kind == 'icon':
        return render_icon(f, sid, e, i, t)
    if kind == 'image':
        return render_image(f, sid, e, i, t)
    if kind == 'panel':
        return render_panel(f, sid, e, i, t)
    if kind == 'chip':
        return render_chip(f, sid, e, i, t)
    if kind == 'zone':
        return render_zone(f, sid, e, i, t)
    if kind == 'timeline':
        return render_timeline(f, sid, e, i, t)
    if kind == 'bracket':
        return render_bracket(f, sid, e, i, t)
    if kind == 'strip':
        return render_strip(f, sid, e, i, t)
    if kind == 'barrow':
        return render_barrow(f, sid, e, i, t)
    if kind == 'table':
        return render_table(f, sid, e, i, t)
    if kind == 'emoji':
        return render_emoji(f, sid, e, i, t)
    if kind == 'chart_bar':
        return render_chart_bar(f, sid, e, i, t)
    if kind == 'chart_line':
        return render_chart_line(f, sid, e, i, t)
    if kind == 'chart_pie':
        return render_chart_pie(f, sid, e, i, t)
    if kind == 'arrow':
        x1, y1, x2, y2 = e['x1'], e['y1'], e['x2'], e['y2']
        ln, ang = math.hypot(x2 - x1, y2 - y1), math.degrees(math.atan2(y2 - y1, x2 - x1))
        hw, hw2 = e.get('w', 10) / 2, 34
        c = col(e.get('color'), getattr(sp, 'ARROW', sp.INK))
        name = text[:12] if text else '箭头'
        # id + 命名挂在箭头线段上（注册表可寻址）；箭头头部为无名伴随元素，同组揭示
        line = (f'<div class="{f}-el" id="{eid}" data-hf-name="箭头：{name}" '
                f'style="left:{x1}px;top:{y1 - hw}px;width:{ln:.0f}px;height:{hw * 2}px;background:{c};'
                f'transform:rotate({ang:.1f}deg);transform-origin:left center;border-radius:6px;"></div>')
        head = (f'<div data-{sid}-arr="{i}" style="position:absolute;left:{x2 - hw2}px;top:{y2 - hw2}px;'
                f'width:{hw2}px;height:{hw2}px;background:{c};transform:rotate({ang:.1f}deg);'
                f'clip-path:polygon(0 12%, 100% 50%, 0 88%);"></div>')
        return f'{line}\n    {head}', sp.fade(f'#{eid}, [data-{sid}-arr="{i}"]', t)
    raise ValueError(f'未知元素 kind: {kind}')


def extract_elements(html):
    els = []
    for m in re.finditer(r'<\w+\b[^>]*?id="([^"]+)"[^>]*?data-hf-name="([^"]+)"[^>]*>', html):
        els.append({'id': m.group(1), 'name': m.group(2)})
    return els


# ── image / emoji / 图表（手绘风渲染层）─────────────────────

ICON_DIR = SHARED / 'assets' / 'icons'


def render_image(f, sid, e, i, t):
    """真实照片：制作期已下载到本地（assets/img/），拍立得白框+墨线阴影呈现。
    src 空 = 搜图无源 → 便签兜底（管线不死）。"""
    eid = f"{sid}-img{i}"
    q = str(e.get('query', '配图')).strip()[:10]
    w, h = e.get('w', 520), e.get('h', 360)
    src = e.get('src')
    if not src:
        note_html = sp.note(f, eid, e['x'], e['y'], q, bg=sp.SKY, rot=-1.5, fs=38)
        return note_html, sp.rise(f'#{eid}', t)
    rot = e.get('rot')
    if rot is None:
        rot = 0 if getattr(sp, 'FLAT', False) else (-1.8 if i % 2 else 1.8)
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="图片：{q}" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{w}px;height:{h}px;'
            f'background:#FFF;border:3px solid {sp.INK};padding:12px 12px 18px;'
            f'box-shadow:7px 7px 0 {sp.INK};transform:rotate({rot}deg);">'
            f'<img src="{src}" style="width:100%;height:100%;object-fit:cover;display:block;" /></div>')
    return html, sp.pop(f'#{eid}', t, scale=.85)


def render_panel(f, sid, e, i, t):
    """卡片面板：白底 + 强调色描边/标题栏（手绘/扁平引擎各有质感）。"""
    eid = f"{sid}-panel{i}"
    html = sp.panel(f, eid, e['x'], e['y'], e.get('w', 560), e.get('h', 320),
                    title=str(e.get('title', '')).strip(), text=str(e.get('text', '')).strip(),
                    bg=col(e.get('bg'), None), fs=e.get('fs', 36))
    return html, sp.rise(f'#{eid}', t)


def render_chip(f, sid, e, i, t):
    """胶囊标签：短词强调（扁平=纯色块白粗字，手绘=圆角标签）。"""
    eid = f"{sid}-chip{i}"
    html = sp.chip(f, eid, e['x'], e['y'], str(e.get('text', '')).strip(),
                   bg=col(e.get('bg'), None), fs=e.get('fs', 40))
    return html, sp.pop(f'#{eid}', t, scale=.7)


# ── 图解结构积木（zone/timeline/bracket/strip/barrow/table）──────

def render_zone(f, sid, e, i, t):
    """淡色高亮区：垫底圈住一组元素（DOM 顺序即 z 序，spec 里放最先）。"""
    eid = f"{sid}-zone{i}"
    bg = col(e.get('bg'), sp.MINT)
    dashed = f'border:3px dashed {bg};' if e.get('dashed') else ''
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="高亮区" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{e["w"]}px;height:{e["h"]}px;'
            f'background:{bg}26;border-radius:26px;{dashed}"></div>')
    return html, sp.fade(f'#{eid}', t, dur=.5)


def render_timeline(f, sid, e, i, t):
    """垂直时间线：粗竖线 + 彩色圆点 + 粗体文字（步骤/流程）。"""
    eid = f"{sid}-tl{i}"
    x, y = e['x'], e['y']
    gap = e.get('gap', 110)
    nodes = e.get('nodes') or ['步骤']
    c = col(e.get('color'), getattr(sp, 'GREEN', sp.MINT))
    fs = e.get('fs', 40)
    r = 22
    h = (len(nodes) - 1) * gap + 2 * r
    width = 60 + max((len(str(n)) for n in nodes), default=4) * fs * 1.1
    line = (f'<div data-{eid}-line style="position:absolute;top:0;left:{r + 8 - 5}px;width:10px;'
            f'height:{h}px;background:{c};border-radius:5px;"></div>')
    parts = [line]
    for k, txt in enumerate(nodes):
        cy = r + k * gap
        border = '' if getattr(sp, 'FLAT', False) else f'border:4px solid {sp.INK};'
        parts.append(f'<div data-{eid}-dot="{k}" style="position:absolute;top:{cy - r}px;'
                     f'left:{8}px;width:{2 * r}px;height:{2 * r}px;border-radius:50%;'
                     f'background:{c};{border}"></div>')
        parts.append(f'<div data-{eid}-lbl="{k}" style="position:absolute;top:{cy - fs * 0.72}px;'
                     f'left:{2 * r + 34}px;font-size:{fs}px;font-weight:900;'
                     f'color:{sp.INK};white-space:nowrap;">{txt}</div>')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="时间线" '
            f'style="top:{y - r - 8}px;left:{x - r - 8}px;width:{width:.0f}px;height:{h + 16:.0f}px;">'
            + ''.join(parts) + '</div>')
    js = (f"gsap.set('#{eid} [data-{eid}-dot],#{eid} [data-{eid}-lbl]',{{opacity:0,x:-16}});\n      "
          f"tl.to('#{eid} [data-{eid}-dot],#{eid} [data-{eid}-lbl]',{{opacity:1,x:0,duration:.35,stagger:.3,ease:'power2.out'}},{t:.2f});")
    return html, js


def render_bracket(f, sid, e, i, t):
    """大括号 + 竖排标注（圈住一组步骤，如「多次重复执行」）。"""
    eid = f"{sid}-brk{i}"
    x, y, h = e['x'], e['y'], e.get('h', 300)
    c = col(e.get('color'), getattr(sp, 'GREEN', sp.MINT))
    txt = str(e.get('text', '')).strip()
    fs = e.get('fs', 44)
    brace_char = '}'
    brace = (f'<div style="position:absolute;top:{-h * 0.08:.0f}px;left:0;height:{h}px;'
             f'line-height:{h}px;font-size:{h * 1.05:.0f}px;font-weight:900;color:{c};">{brace_char}</div>')
    lbl = ''
    if txt:
        lbl = (f'<div style="position:absolute;top:0;left:{h * 0.5:.0f}px;height:{h}px;'
               f'writing-mode:vertical-rl;text-align:center;font-size:{fs}px;font-weight:900;'
               f'color:{c};letter-spacing:8px;">{txt}</div>')
    w = h * 0.5 + fs * 1.3 + 20
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="括注：{txt[:8]}" '
            f'style="top:{y}px;left:{x}px;width:{w:.0f}px;height:{h}px;">{brace}{lbl}</div>')
    return html, sp.fade(f'#{eid}', t, dur=.5)


def render_strip(f, sid, e, i, t):
    """小方块序列 + 省略号 + 说明（向量/维度示意，如「512维」）。"""
    eid = f"{sid}-strip{i}"
    x, y = e['x'], e['y']
    n = int(e.get('n', 5))
    size = e.get('size', 44)
    gap = 14
    c = col(e.get('color'), sp.SKY)
    txt = str(e.get('text', '')).strip()
    w = n * (size + gap) + 40
    if txt:
        w += len(txt) * 36 + 50
    parts = []
    for k in range(n):
        parts.append(f'<div data-{eid}-sq="{k}" style="position:absolute;top:0;'
                     f'left:{k * (size + gap)}px;width:{size}px;height:{size}px;'
                     f'border-radius:10px;background:{c};border:3px solid {sp.INK};"></div>')
    cx = n * (size + gap)
    if e.get('ellipsis', True):
        parts.append(f'<div style="position:absolute;top:{-size * 0.28:.0f}px;left:{cx}px;'
                     f'font-size:{size}px;font-weight:900;color:{sp.INK};">…</div>')
        cx += size * 0.8
    if txt:
        parts.append(f'<div style="position:absolute;top:{size * 0.12:.0f}px;left:{cx + 14}px;'
                     f'font-size:{e.get("fs", 36)}px;font-weight:900;color:{sp.INK};">{txt}</div>')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="方块序列：{txt or n}" '
            f'style="top:{y}px;left:{x}px;width:{w:.0f}px;height:{size}px;">' + ''.join(parts) + '</div>')
    js = (f"gsap.set('#{eid} [data-{eid}-sq]',{{opacity:0,scale:.3,transformOrigin:'50% 50%'}});\n      "
          f"tl.to('#{eid} [data-{eid}-sq]',{{opacity:1,scale:1,duration:.3,stagger:.14,ease:'back.out(1.8)'}},{t:.2f});")
    return html, js


def render_barrow(f, sid, e, i, t):
    """粗块箭头（实心大箭头，流程指向）。"""
    eid = f"{sid}-barrow{i}"
    w_ = e.get('w', 260)
    h_ = e.get('h', 90)
    rot = e.get('rot', 0)
    c = col(e.get('color'), getattr(sp, 'ARROW', sp.INK))
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="块箭头" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{w_}px;height:{h_}px;background:{c};'
            f'clip-path:polygon(0 32%,60% 32%,60% 0,100% 50%,60% 100%,60% 68%,0 68%);'
            f'transform:rotate({rot}deg);"></div>')
    js = sp.draw_x(f'#{eid}', t, dur=.5) if abs(rot) < 10 else sp.pop(f'#{eid}', t, scale=.6)
    return html, js


def render_table(f, sid, e, i, t):
    """格子表格（数字/短文本行列，白底描边圆角格）。"""
    eid = f"{sid}-tbl{i}"
    rows = e.get('rows') or [['?']]
    w_ = e.get('w', 520)
    cell_h = e.get('cell_h', 76)
    fs = e.get('fs', 30)
    nrow = len(rows)
    ncol = max(len(r) for r in rows)
    cw = (w_ - (ncol - 1) * 10) / ncol
    parts = []
    for ri, row in enumerate(rows):
        for ci in range(ncol):
            val = row[ci] if ci < len(row) else ''
            parts.append(f'<div data-{eid}-c="{ri}-{ci}" style="position:absolute;'
                         f'top:{ri * (cell_h + 10)}px;left:{ci * (cw + 10):.0f}px;'
                         f'width:{cw:.0f}px;height:{cell_h}px;background:#FFFFFF;'
                         f'border:3px solid {sp.INK};border-radius:10px;display:flex;'
                         f'align-items:center;justify-content:center;font-size:{fs}px;'
                         f'font-weight:700;color:{sp.INK};">{val}</div>')
    h_ = nrow * cell_h + (nrow - 1) * 10
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="表格" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{w_}px;height:{h_}px;">' + ''.join(parts) + '</div>')
    js = sp.stagger_pop(f'#{eid} [data-{eid}-c]', t, step=.12)
    return html, js


def render_emoji(f, sid, e, i, t):
    """大号彩色 emoji（本地系统字体，确定性渲染）。"""
    eid = f"{sid}-emoji{i}"
    fs = e.get('fs', 140)
    ch = (e.get('text') or '✨')[:4]
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="表情：{ch}" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;font-size:{fs}px;line-height:1.1;'
            f"font-family:'Segoe UI Emoji','Apple Color Emoji','Noto Color Emoji',sans-serif;\">{ch}</div>")
    return html, sp.pop(f'#{eid}', t, scale=.5)


def render_icon(f, sid, e, i, t):
    """本地 lucide 线稿图标：inline 进合成物（无网络/无外链，lint 安全）。"""
    size = e.get('size', 120)
    rot = e.get('rot', 0)
    eid = f"{sid}-icon{i}"
    name = str(e.get('name', '')).strip()
    path = ICON_DIR / f"{name}.svg"
    if not path.exists():  # 兜底：未知图标 → 便签盒子 + 名字（不阻塞管线）
        note_html = sp.note(f, eid, e['x'], e['y'], name[:10] or '?', bg=sp.BUTTER, rot=-1.5, fs=36)
        return note_html, sp.rise(f'#{eid}', t)
    svg = path.read_text(encoding='utf-8')
    svg = re.sub(r'<!--.*?-->', '', svg, flags=re.S)          # 许可注释不入帧
    svg = re.sub(r'\sclass="[^"]*"', '', svg)                  # 去 lucide class
    svg = re.sub(r'<svg\b', '<svg width="100%" height="100%"', svg, count=1)
    stroke = e.get('color') or sp.INK
    sw = 2 if size >= 140 else 1.5                             # 24 viewBox：放大后描边视觉补偿
    svg = re.sub(r'stroke-width="[\d.]+"', f'stroke-width="{sw}"', svg, count=1)
    rot_css = f'transform:rotate({rot}deg);' if abs(rot) > 0.01 else ''
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="图标：{name}" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{size}px;height:{size}px;'
            f'color:{stroke};{rot_css}">{svg}</div>')
    return html, sp.pop(f'#{eid}', t, scale=.55)


def _fmt_num(v):
    return f"{v:.4g}"


def render_chart_bar(f, sid, e, i, t):
    """手绘柱状图：SVG + 微旋转抖动（确定性），柱 stagger 生长。"""
    eid = f"{sid}-cbar{i}"
    x, y, w, h = e['x'], e['y'], e.get('w', 560), e.get('h', 360)
    values = e.get('values') or [1, 1]
    labels = e.get('labels') or [''] * len(values)
    n = len(values)
    vmax = max(values) or 1.0
    bw = min(96, (w - 60) / n - 26)
    gap = (w - 40 - n * bw) / max(n - 1, 1) if n > 1 else 0
    pad_b, label_fs, pad_t = 56, 26, 48   # pad_t：数值标签头部空间（check 门禁测出的越界）
    area_h = h - pad_b - pad_t
    parts = []
    for j, v in enumerate(values):
        bx = 20 + j * (bw + gap)
        bh = max(14, area_h * (v / vmax))
        jitter = ((j % 3) - 1) * 0.8                          # 手绘感抖动收窄，防柱角越顶
        fill = CHART_FILLS[j % len(CHART_FILLS)]
        parts.append(
            f'<g data-bar="{j}" style="transform:rotate({jitter:.1f}deg);transform-origin:{bx + bw/2:.0f}px {h:.0f}px;">'
            f'<rect x="{bx:.0f}" y="{pad_t + area_h - bh:.0f}" width="{bw:.0f}" height="{bh:.0f}" rx="6" '
            f'fill="{fill}" stroke="{sp.INK}" stroke-width="4"/></g>')
        if labels[j]:
            parts.append(f'<text x="{bx + bw/2:.0f}" y="{h - 18}" text-anchor="middle" '
                         f'font-size="{label_fs}" fill="{sp.INK}" font-family="{sp.FONT}">{labels[j]}</text>')
        vy = max(pad_t + area_h - bh - 10, label_fs + 4)      # 数值标签夹在 SVG 内
        parts.append(f'<text x="{bx + bw/2:.0f}" y="{vy:.0f}" text-anchor="middle" '
                     f'font-size="{label_fs + 2}" font-weight="700" fill="{sp.INK}" '
                     f'font-family="{sp.FONT}">{_fmt_num(v)}</text>')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="柱状图" '
            f'style="top:{y}px;left:{x}px;width:{w}px;height:{h}px;">'
            f'<svg viewBox="0 0 {w} {h}" width="{w}" height="{h}">{"".join(parts)}</svg></div>')
    return html, sp.stagger_grow(f'#{eid} [data-bar]', t, step=.22, origin='center bottom')


def render_chart_line(f, sid, e, i, t):
    """手绘折线图：描边生长（dashoffset）+ 逐点 pop。"""
    eid = f"{sid}-cline{i}"
    x, y, w, h = e['x'], e['y'], e.get('w', 560), e.get('h', 360)
    values = e.get('values') or [1, 2, 1]
    n = len(values)
    vmax, vmin = max(values), min(values)
    span = (vmax - vmin) or 1.0
    pad_b, pad_t = 56, 26
    area_h = h - pad_b - pad_t
    pts = []
    for j, v in enumerate(values):
        px = 28 + (w - 56) * (j / max(n - 1, 1))
        py = pad_t + area_h * (1 - (v - vmin) / span)
        pts.append((px, py))
    poly = ' '.join(f'{px:.0f},{py:.0f}' for px, py in pts)
    dots = ''.join(
        f'<circle data-pt="{j}" cx="{px:.0f}" cy="{py:.0f}" r="13" fill="{sp.CORAL}" stroke="{sp.INK}" stroke-width="4"/>'
        for j, (px, py) in enumerate(pts))
    labels = e.get('labels') or []
    lbl = ''.join(
        f'<text x="{px:.0f}" y="{h - 18}" text-anchor="middle" font-size="26" fill="{sp.INK}" '
        f'font-family="{sp.FONT}">{labels[j]}</text>'
        for j, (px, py) in enumerate(pts) if j < len(labels) and labels[j])
    length = (w - 56) * 1.15 + area_h  # 折线长度近似（dash 动画用，宁多勿少）
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="折线图" '
            f'style="top:{y}px;left:{x}px;width:{w}px;height:{h}px;">'
            f'<svg viewBox="0 0 {w} {h}" width="{w}" height="{h}">'
            f'<polyline data-line points="{poly}" fill="none" stroke="{sp.INK}" stroke-width="6" '
            f'stroke-linecap="round" stroke-linejoin="round" stroke-dasharray="{length:.0f}" '
            f'stroke-dashoffset="{length:.0f}"/>{dots}{lbl}</svg></div>')
    js = (f"tl.to('#{eid} [data-line]',{{strokeDashoffset:0,duration:1.1,ease:'power2.out'}},{t:.2f});\n      "
          + sp.stagger_pop(f'#{eid} [data-pt]', t + .5, step=.18))
    return html, js


def render_chart_pie(f, sid, e, i, t):
    """手绘饼图：扇形逐个 pop；标签沿扇形外侧放置。"""
    eid = f"{sid}-cpie{i}"
    x, y, w, h = e['x'], e['y'], e.get('w', 360), e.get('h', 360)
    values = e.get('values') or [1, 1]
    labels = e.get('labels') or [''] * len(values)
    total = sum(values) or 1.0
    r = min(w, h) / 2 - 8
    cx, cy = w / 2, h / 2
    parts, angle = [], -90.0
    for j, v in enumerate(values):
        sweep = 360 * v / total
        a0, a1 = angle, angle + sweep
        large = 1 if sweep > 180 else 0
        x0, y0 = cx + r * math.cos(math.radians(a0)), cy + r * math.sin(math.radians(a0))
        x1, y1 = cx + r * math.cos(math.radians(a1)), cy + r * math.sin(math.radians(a1))
        fill = CHART_FILLS[j % len(CHART_FILLS)]
        parts.append(f'<path data-wedge="{j}" d="M{cx:.0f},{cy:.0f} L{x0:.0f},{y0:.0f} '
                     f'A{r:.0f},{r:.0f} 0 {large} 1 {x1:.0f},{y1:.0f} Z" '
                     f'fill="{fill}" stroke="{sp.INK}" stroke-width="4" stroke-linejoin="round"/>')
        mid = math.radians(a0 + sweep / 2)
        lx, ly = cx + (r + 44) * math.cos(mid), cy + (r + 44) * math.sin(mid)
        anchor = 'middle' if abs(math.cos(mid)) < .35 else ('start' if math.cos(mid) > 0 else 'end')
        parts.append(f'<text data-wlbl="{j}" x="{lx:.0f}" y="{ly:.0f}" text-anchor="{anchor}" '
                     f'dominant-baseline="middle" font-size="27" fill="{sp.INK}" '
                     f'font-family="{sp.FONT}">{labels[j]} {_fmt_num(v)}</text>')
        angle = a1
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="饼图" '
            f'style="top:{y}px;left:{x}px;width:{w}px;height:{h}px;">'
            f'<svg viewBox="0 0 {w} {h}" width="{w}" height="{h}">{"".join(parts)}</svg></div>')
    js = sp.stagger_pop(f'#{eid} [data-wedge]', t, step=.25) + '\n      ' + \
        sp.fade(f'#{eid} [data-wlbl]', t + .5)
    return html, js


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    sb = json.load(open(proj / 'storyboards' / 'storyboard.json', encoding='utf-8'))
    init_engine(proj)  # 按项目风格方向选引擎（手绘/扁平）
    meta = sp.load(proj)
    # 画幅随项目（project.json aspect，缺省 16:9 与既有项目一致）
    W, H = (1080, 1920) if _aspect(proj) == '9:16' else (1920, 1080)
    for seg in sb['segments']:
        sid = seg['id']
        spec_path = proj / 'llm' / f'comp-{sid}.spec.json'
        if not spec_path.exists():
            print(f'{sid}: 缺 spec，跳过')
            return 1
        spec = json.load(open(spec_path, encoding='utf-8'))
        words = meta['words'][sid]
        htmls, jss = [], []
        for i, e in enumerate(spec.get('elements', [])):
            if not e.get('kind'):
                print(f"{sid}: 元素{i + 1} 缺 kind，跳过")
                continue
            try:
                h, j = render_element(PREFIX, sid, e, i + 1, words, W)
            except ValueError as ex:  # 未知 kind：跳过不炸整段
                print(f'{sid}: {ex}，跳过')
                continue
            htmls.append(h)
            jss.append(j)
        full = sp.wrap(proj, PREFIX, sid, '\n    '.join(htmls), '\n      '.join(jss), W, H)
        payload = {'html': full, 'elements': extract_elements(full)}
        out = proj / 'llm' / f'comp-{sid}.json'
        json.dump(payload, open(out, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
        print(f'{sid}: {len(htmls)} 元素 ✓')
    return 0


def _aspect(proj) -> str:
    try:
        return json.load(open(proj / 'project.json', encoding='utf-8')).get('aspect', '16:9')
    except Exception:
        return '16:9'


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

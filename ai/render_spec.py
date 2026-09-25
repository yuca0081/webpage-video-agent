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

AI_DIR = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(AI_DIR / 'engines'))
import stylepack as sp  # noqa: E402  默认引擎（手绘叙事）；main 按项目风格方向分发

import colorutil as cu  # noqa: E402  WCAG 对比度工具

COLORS = {}
CHART_FILLS = []
PREFIX = 's'


def init_engine(proj):
    """按项目 style_samples.json 的 direction 选风格引擎，重建色表。

    direction 含「扁平」→ stylepack_flat（厚描边·色块）；含「手绘」→ stylepack；
    其余按 ai/registry/styles.json 注册表关键词匹配 → stylepack_generic（token 驱动，
    13 个注册风格：深空渐变/数学线框/地缘档案/高能说明书/发布会深色/轻快多彩/
    渐变玻璃/财经图表/公益数据/清洁医疗/自然环保/深色等距科技/黑板粉笔）；
    未命中回退手绘叙事。
    """
    global sp, COLORS, CHART_FILLS
    d = ''
    try:
        with open(proj / 'style' / 'style_samples.json', encoding='utf-8') as fh:
            d = (json.load(fh) or {}).get('direction', '')
    except Exception:
        d = ''
    generic = None
    if '扁平' in d:
        import stylepack_flat
        sp = stylepack_flat
    elif '手绘' in d or not d:
        import stylepack
        sp = stylepack
    else:
        import stylepack_generic
        generic = stylepack_generic.match(d)
        if generic:
            stylepack_generic.configure(generic['tokens'])
            sp = stylepack_generic
        else:
            import stylepack
            sp = stylepack
    COLORS = {'butter': sp.BUTTER, 'mint': sp.MINT, 'sky': sp.SKY, 'coral': sp.CORAL,
              'peach': sp.PEACH, 'pink': sp.PINK, 'turq': sp.TURQ, 'ink': sp.INK,
              'white': '#FFFFFF', 'navy': getattr(sp, 'NAVY', sp.INK),
              'green': getattr(sp, 'GREEN', sp.TURQ)}
    CHART_FILLS = [sp.MINT, sp.SKY, sp.BUTTER, sp.CORAL, sp.PEACH, sp.PINK]


def th(key, default):
    """引擎主题值（暗底文字/描边/表面色/图片框等）；手绘与扁平引擎无 THEME → 走缺省。"""
    return (getattr(sp, 'THEME', {}) or {}).get(key, default)


def _on_paper(c):
    """文字/装饰色落在页面纸底上：token 引擎按纸底自动加深/提亮过对比门禁。"""
    paper = th('paper', None)
    if not paper:
        return c
    return cu.ensure_readable(c, paper)


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
        color = col(e.get('color'), th('txt', sp.INK))
        html = sp.label(f, eid, e['x'], e['y'], text, color=color, fs=fs)
        return html, sp.fade(f'#{eid}', t)
    if kind == 'big':
        fs = e.get('fs', 110)
        color = _on_paper(col(e.get('color'), getattr(sp, 'BIG', '#B45309')))
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
                f'background:{fill};border:3px solid {th("line", sp.INK)};border-radius:50%;"></div>')
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
    if kind == 'chart_donut':
        return render_chart_donut(f, sid, e, i, t)
    if kind == 'quote':
        return render_quote(f, sid, e, i, t)
    if kind == 'checklist':
        return render_checklist(f, sid, e, i, t)
    if kind == 'stat':
        return render_stat(f, sid, e, i, t)
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

ICON_DIR = AI_DIR / 'assets' / 'icons'


def render_image(f, sid, e, i, t):
    """真实照片：制作期已下载到本地（assets/img/）。相框质感随风格 THEME：
    polaroid（白框手绘感）/ clean（细描边圆角）/ glass（半透明玻璃）。
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
        if getattr(sp, 'THEME', None):  # token 引擎：风格自带照片倾斜角
            rot = getattr(sp, 'IMG_ROT', 0) * (1 if i % 2 else -1)
        else:
            rot = 0 if getattr(sp, 'FLAT', False) else (-1.8 if i % 2 else 1.8)
    frame = th('img_frame', 'polaroid')
    if frame == 'glass':
        box = (f'background:rgba(255,255,255,.10);border:1px solid {th("line", sp.INK)};'
               f'border-radius:{th("radius", 18)}px;padding:0;'
               f'box-shadow:{th("img_shadow", "0 20px 50px rgba(0,0,0,.4)")};backdrop-filter:blur(6px);')
    elif frame == 'clean':
        box = (f'background:{th("surface", "#FFF")};border:1px solid {th("line", sp.INK)};'
               f'border-radius:{th("radius", 12)}px;padding:0;'
               f'box-shadow:{th("img_shadow", "0 12px 28px rgba(0,0,0,.16)")};')
    else:  # polaroid：拍立得白框（手绘/纸面传统）
        box = (f'background:#FFF;border:3px solid {sp.INK};padding:12px 12px 18px;'
               f'box-shadow:{th("img_shadow", f"7px 7px 0 {sp.INK}")};')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="图片：{q}" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{w}px;height:{h}px;{box}'
            f'transform:rotate({rot}deg);">'
            f'<img src="{src}" style="width:100%;height:100%;object-fit:cover;display:block;'
            f'border-radius:{th("radius", 8) if frame != "polaroid" else 0}px;" /></div>')
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
                     f'color:{th("txt", sp.INK)};white-space:nowrap;">{txt}</div>')
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
                     f'border-radius:10px;background:{c};border:3px solid {th("line", sp.INK)};"></div>')
    cx = n * (size + gap)
    if e.get('ellipsis', True):
        parts.append(f'<div style="position:absolute;top:{-size * 0.28:.0f}px;left:{cx}px;'
                     f'font-size:{size}px;font-weight:900;color:{th("txt", sp.INK)};">…</div>')
        cx += size * 0.8
    if txt:
        parts.append(f'<div style="position:absolute;top:{size * 0.12:.0f}px;left:{cx + 14}px;'
                     f'font-size:{e.get("fs", 36)}px;font-weight:900;color:{th("txt", sp.INK)};">{txt}</div>')
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
                         f'width:{cw:.0f}px;height:{cell_h}px;background:{th("surface", "#FFFFFF")};'
                         f'border:3px solid {th("line", sp.INK)};border-radius:10px;display:flex;'
                         f'align-items:center;justify-content:center;font-size:{fs}px;'
                         f'font-weight:700;color:{th("surface_txt", sp.INK)};">{val}</div>')
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
    stroke = e.get('color') or th('txt', sp.INK)  # 图标是视觉锚点：用正文色而非半透明描边色
    sw = 2 if size >= 140 else 1.5                             # 24 viewBox：放大后描边视觉补偿
    svg = re.sub(r'stroke-width="[\d.]+"', f'stroke-width="{sw}"', svg, count=1)
    rot_css = f'transform:rotate({rot}deg);' if abs(rot) > 0.01 else ''
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="图标：{name}" '
            f'style="top:{e["y"]}px;left:{e["x"]}px;width:{size}px;height:{size}px;'
            f'color:{stroke};{rot_css}">{svg}</div>')
    return html, sp.pop(f'#{eid}', t, scale=.55)


def render_quote(f, sid, e, i, t):
    """金句/引用：行内大引号 + 重磅大字 + 署名（风格主色装饰）。"""
    eid = f"{sid}-quote{i}"
    x, y = e['x'], e['y']
    w = e.get('w', 1240)
    fs = e.get('fs', 64)
    text = str(e.get('text', '')).strip()
    name = str(e.get('name', '')).strip()
    c = _on_paper(col(e.get('color'), getattr(sp, 'PRIMARY', getattr(sp, 'BIG', sp.INK))))
    body = (f'<div data-{eid}-txt style="font-size:{fs}px;'
            f'font-weight:800;line-height:1.5;color:{th("txt", sp.INK)};white-space:pre-wrap;">'
            f'<span data-{eid}-mark style="font-size:{fs * 1.45:.0f}px;line-height:0;'
            f'font-weight:900;color:{c};vertical-align:-0.12em;margin-right:.08em;">“</span>{text}</div>')
    who = ''
    if name:
        who = (f'<div data-{eid}-who style="margin-top:{fs * 0.4:.0f}px;text-align:right;'
               f'font-size:{max(26, fs * 0.42):.0f}px;font-weight:700;color:{c};">—— {name}</div>')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="金句：{text[:10]}" '
            f'style="top:{y}px;left:{x}px;width:{w}px;">{body}{who}</div>')
    js = (f"gsap.set('#{eid} [data-{eid}-mark]',{{opacity:0,scale:2.4,transformOrigin:'0% 60%',display:'inline-block'}});\n      "
          f"tl.to('#{eid} [data-{eid}-mark]',{{opacity:1,scale:1,duration:.5,ease:'power3.out'}},{t:.2f});\n      "
          + sp.fade(f'#{eid} [data-{eid}-who]', t + .7))
    return html, js


def render_checklist(f, sid, e, i, t):
    """要点清单：对勾圆徽 + 粗体条目，逐条落位（卖点/论据/清单）。"""
    eid = f"{sid}-cl{i}"
    x, y = e['x'], e['y']
    gap = e.get('gap', 96)
    nodes = e.get('nodes') or ['要点']
    c = col(e.get('color'), getattr(sp, 'PRIMARY', sp.MINT))
    fs = e.get('fs', 40)
    r = 26
    width = 70 + max((len(str(n)) for n in nodes), default=4) * fs * 1.1
    parts = []
    for k, txt in enumerate(nodes):
        cy = r + k * gap
        cc, tc = cu.pair(c)
        parts.append(f'<div data-{eid}-chk="{k}" style="position:absolute;top:{cy - r}px;left:0;'
                     f'width:{2 * r}px;height:{2 * r}px;border-radius:50%;background:{cc};'
                     f'display:flex;align-items:center;justify-content:center;'
                     f'font-size:{r + 4}px;font-weight:900;color:{tc};">✓</div>')
        parts.append(f'<div data-{eid}-lbl="{k}" style="position:absolute;top:{cy - fs * 0.68}px;'
                     f'left:{2 * r + 28}px;font-size:{fs}px;font-weight:800;'
                     f'color:{th("txt", sp.INK)};white-space:nowrap;">{txt}</div>')
    h = (len(nodes) - 1) * gap + 2 * r
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="清单：{nodes[0][:8]}" '
            f'style="top:{y}px;left:{x}px;width:{width:.0f}px;height:{h}px;">' + ''.join(parts) + '</div>')
    js = (f"gsap.set('#{eid} [data-{eid}-chk]',{{opacity:0,scale:.2,transformOrigin:'50% 50%'}});\n      "
          f"tl.to('#{eid} [data-{eid}-chk]',{{opacity:1,scale:1,duration:.35,stagger:.3,ease:'back.out(2)'}},{t:.2f});\n      "
          f"gsap.set('#{eid} [data-{eid}-lbl]',{{opacity:0,x:-18}});\n      "
          f"tl.to('#{eid} [data-{eid}-lbl]',{{opacity:1,x:0,duration:.35,stagger:.3,ease:'power2.out'}},{t + .1:.2f});")
    return html, js


def render_stat(f, sid, e, i, t):
    """指标卡：大数字 + 标签（KPI/参数，发布会/财经气质）。"""
    eid = f"{sid}-stat{i}"
    x, y = e['x'], e['y']
    w = e.get('w', 420)
    fs = e.get('fs', 96)
    text = str(e.get('text', '')).strip()
    title = str(e.get('title', '')).strip()
    lab_fs = max(28, int(fs * 0.32))
    accent = col(e.get('bg'), getattr(sp, 'PRIMARY', getattr(sp, 'BIG', sp.INK)))
    num_c = col(e.get('color'), accent)
    surface = th('surface', '#FFFFFF')
    if not (isinstance(surface, str) and surface.startswith('#') and len(surface) == 7):
        surface = th('paper', None)  # 玻璃等 rgba 表面：按纸底（合成后的近似）保障
    if isinstance(surface, str) and surface.startswith('#') and len(surface) == 7:
        num_c = cu.ensure_readable(num_c, surface)  # 彩字自动加深/提亮过门禁
    if getattr(sp, 'THEME', None):
        border = f'border:1px solid {th("line", sp.INK)};'
    elif getattr(sp, 'FLAT', False):
        border = f'border:4px solid {accent};'
    else:
        border = f'border:3px solid {sp.INK};'
    inner = (f'<div data-{eid}-num style="font-size:{fs}px;font-weight:900;line-height:1.12;'
             f'color:{num_c};letter-spacing:1px;">{text}</div>')
    if title:
        inner += (f'<div data-{eid}-lab style="margin-top:{int(fs * 0.06)}px;font-size:{lab_fs}px;'
                  f'font-weight:700;color:{th("surface_txt", sp.INK)};opacity:.85;">{title}</div>')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="指标：{text[:10]}" '
            f'style="top:{y}px;left:{x}px;width:{w}px;padding:{int(fs * 0.22)}px {int(fs * 0.3)}px;'
            f'background:{th("surface", "#FFFFFF")};{border}'
            f'border-radius:{th("radius", 16)}px;box-shadow:{th("img_shadow", "0 12px 30px rgba(0,0,0,.14)")};">'
            f'{inner}</div>')
    return html, sp.pop(f'#{eid}', t, scale=.86)


def render_chart_donut(f, sid, e, i, t):
    """环形图：扇环逐段描画 + 右侧图例；中心可放标题/合计。"""
    eid = f"{sid}-donut{i}"
    x, y = e['x'], e['y']
    w, h = e.get('w', 460), e.get('h', 360)
    values = e.get('values') or [1, 1]
    labels = e.get('labels') or [''] * len(values)
    total = sum(values) or 1.0
    title = str(e.get('title', '')).strip()
    dia = min(h, w - (200 if labels else 0))          # 右侧留图例列
    dia = max(dia, 160)
    sw = max(34, int(dia * 0.16))                     # 环厚
    r = (dia - sw) / 2
    cx = cy = dia / 2
    C = 2 * math.pi * r
    parts = []
    acc = 0.0
    for j, v in enumerate(values):
        frac = v / total
        fill = CHART_FILLS[j % len(CHART_FILLS)]
        rot = -90 + 360 * acc
        parts.append(f'<circle data-seg="{j}" cx="{cx:.0f}" cy="{cy:.0f}" r="{r:.0f}" fill="none" '
                     f'stroke="{fill}" stroke-width="{sw}"'
                     f' transform="rotate({rot:.1f} {cx:.0f} {cy:.0f})" '
                     f'stroke-dasharray="0 {C:.1f}"/>')
        acc += frac
    if title:
        parts.append(f'<text x="{cx:.0f}" y="{cy + 12:.0f}" text-anchor="middle" '
                     f'font-size="{dia * 0.13:.0f}" font-weight="800" fill="{th("txt", sp.INK)}" '
                     f'font-family="{sp.FONT}">{title}</text>')
    legend = []
    lx = dia + 36
    for j, v in enumerate(values):
        fill = CHART_FILLS[j % len(CHART_FILLS)]
        legend.append(f'<div data-lg="{j}" style="position:absolute;top:{j * 64 + 8}px;left:{lx}px;'
                      f'display:flex;align-items:center;font-size:30px;font-weight:700;'
                      f'color:{th("txt", sp.INK)};white-space:nowrap;">'
                      f'<span style="width:26px;height:26px;border-radius:8px;background:{fill};'
                      f'display:inline-block;margin-right:14px;"></span>'
                      f'{labels[j] or "项" + str(j + 1)}&nbsp;&nbsp;{_fmt_num(v / total * 100)}%</div>')
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="环形图" '
            f'style="top:{y}px;left:{x}px;width:{w}px;height:{h}px;">'
            f'<svg width="{dia}" height="{dia}">{"".join(parts)}</svg>' + ''.join(legend) + '</div>')
    # 逐段顺序描画（GSAP attr tween 只吃字面值，最终 dasharray 在 Python 侧算好）
    seg_js = ''
    acc_t = t
    for j, v in enumerate(values):
        final = f'{v / total * C:.1f} {C:.1f}'
        seg_js += (f"tl.to('#{eid} [data-seg=\"{j}\"]',{{attr:{{'stroke-dasharray':'{final}'}},"
                   f"duration:.55,ease:'power2.inOut'}},{acc_t:.2f});\n      ")
        acc_t += 0.28
    js = seg_js + sp.stagger_pop(f'#{eid} [data-lg]', t + .3, step=.2)
    return html, js


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
            f'fill="{fill}" stroke="{th("line", sp.INK)}" stroke-width="4"/></g>')
        if labels[j]:
            parts.append(f'<text x="{bx + bw/2:.0f}" y="{h - 18}" text-anchor="middle" '
                         f'font-size="{label_fs}" fill="{th("txt", sp.INK)}" font-family="{sp.FONT}">{labels[j]}</text>')
        vy = max(pad_t + area_h - bh - 10, label_fs + 4)      # 数值标签夹在 SVG 内
        parts.append(f'<text x="{bx + bw/2:.0f}" y="{vy:.0f}" text-anchor="middle" '
                     f'font-size="{label_fs + 2}" font-weight="700" fill="{th("txt", sp.INK)}" '
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
        f'<circle data-pt="{j}" cx="{px:.0f}" cy="{py:.0f}" r="13" fill="{sp.CORAL}" stroke="{th("line", sp.INK)}" stroke-width="4"/>'
        for j, (px, py) in enumerate(pts))
    labels = e.get('labels') or []
    lbl = ''.join(
        f'<text x="{px:.0f}" y="{h - 18}" text-anchor="middle" font-size="26" fill="{th("txt", sp.INK)}" '
        f'font-family="{sp.FONT}">{labels[j]}</text>'
        for j, (px, py) in enumerate(pts) if j < len(labels) and labels[j])
    length = (w - 56) * 1.15 + area_h  # 折线长度近似（dash 动画用，宁多勿少）
    html = (f'<div class="{f}-el" id="{eid}" data-hf-name="折线图" '
            f'style="top:{y}px;left:{x}px;width:{w}px;height:{h}px;">'
            f'<svg viewBox="0 0 {w} {h}" width="{w}" height="{h}">'
            f'<polyline data-line points="{poly}" fill="none" stroke="{th("line", sp.INK)}" stroke-width="6" '
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
                     f'fill="{fill}" stroke="{th("line", sp.INK)}" stroke-width="4" stroke-linejoin="round"/>')
        mid = math.radians(a0 + sweep / 2)
        lx, ly = cx + (r + 44) * math.cos(mid), cy + (r + 44) * math.sin(mid)
        anchor = 'middle' if abs(math.cos(mid)) < .35 else ('start' if math.cos(mid) > 0 else 'end')
        parts.append(f'<text data-wlbl="{j}" x="{lx:.0f}" y="{ly:.0f}" text-anchor="{anchor}" '
                     f'dominant-baseline="middle" font-size="27" fill="{th("txt", sp.INK)}" '
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

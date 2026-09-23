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
import stylepack as sp  # noqa: E402

COLORS = {'butter': sp.BUTTER, 'mint': sp.MINT, 'sky': sp.SKY, 'coral': sp.CORAL,
          'peach': sp.PEACH, 'pink': sp.PINK, 'turq': sp.TURQ, 'ink': sp.INK, 'white': '#FFFFFF'}
PREFIX = 's'


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
        color = col(e.get('color'), '#B45309')
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
    if kind == 'arrow':
        x1, y1, x2, y2 = e['x1'], e['y1'], e['x2'], e['y2']
        ln, ang = math.hypot(x2 - x1, y2 - y1), math.degrees(math.atan2(y2 - y1, x2 - x1))
        hw, hw2 = e.get('w', 10) / 2, 34
        c = col(e.get('color'), sp.INK)
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


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    sb = json.load(open(proj / 'storyboards' / 'storyboard.json', encoding='utf-8'))
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
            h, j = render_element(PREFIX, sid, e, i + 1, words, W)
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

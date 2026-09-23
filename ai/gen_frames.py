# -*- coding: utf-8 -*-
"""通用帧产物生成器：按项目名调用 _shared/defs.py 的构建器，产出 llm/comp-segNN.json。
用法：python ai/gen_frames.py <项目目录>
"""
import json
import pathlib
import re
import sys

SHARED = pathlib.Path(__file__).resolve().parent.parent / 'data' / 'projects' / '_shared'
sys.path.insert(0, str(SHARED))
import defs  # noqa: E402
import stylepack as sp  # noqa: E402

NAME2KEY = {'猫为什么四脚着地': 'cat-land', '冰箱搬热的原理': 'fridge', '疫苗训练免疫': 'vaccine',
            '星星为什么眨眼': 'star-twinkle', '密码破译简史': 'cipher', '蜂巢六边形': 'beehive',
            'GPS 定位原理': 'gps', '面包为什么发酵': 'bread', '极光的成因': 'aurora', '运动后肌肉酸痛': 'muscle'}


def extract_elements(html):
    els = []
    for m in re.finditer(r'<\w+\b[^>]*?id="([^"]+)"[^>]*?data-hf-name="([^"]+)"[^>]*>', html):
        els.append({'id': m.group(1), 'name': m.group(2)})
    if not els:
        els = [{'id': f'e{i}', 'name': n} for i, n in enumerate(re.findall(r'data-hf-name="([^"]+)"', html))]
    return els


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    name = json.load(open(proj / 'project.json', encoding='utf-8')).get('name', '')
    key = NAME2KEY.get(name)
    if not key:
        print(f'未登记的项目名: {name}')
        return 1
    segs = defs.BUILDERS[key](proj)
    for sid, (html, js, _) in segs.items():
        m = re.search(r'class="([a-z])-\S', html)
        prefix = m.group(1) if m else 'x'
        full = sp.wrap(proj, prefix, sid, html, js)
        els = extract_elements(full)
        payload = {'html': full, 'elements': els}
        out = proj / 'llm' / f'comp-{sid}.json'
        json.dump(payload, open(out, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    print(f'{key}: {len(segs)} 段产物 ✓')
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

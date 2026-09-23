# -*- coding: utf-8 -*-
"""组装 hyperframes 工程：index.html + assets/gsap.min.js + package.json + meta.json。
用法：python ai/assemble.py <项目目录>
"""
import json
import pathlib
import shutil
import subprocess
import sys

# gsap 资产固定来源：_shared/assets（首次部署时放一份）；旧项目路径仅作迁移期兜底。
_SHARED = pathlib.Path(__file__).resolve().parent.parent / 'data' / 'projects' / '_shared'
GSAP_CANDIDATES = [
    _SHARED / 'assets' / 'gsap.min.js',
    pathlib.Path(r"E:\workspace\develop\webpage-video-agent\data\projects\p20260922-135417\assets\gsap.min.js"),
]
GSAP_SRC = next((p for p in GSAP_CANDIDATES if p.exists()), GSAP_CANDIDATES[0])


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    meta = json.load(open(proj / 'audio_meta.json', encoding='utf-8'))
    pj = json.load(open(proj / 'project.json', encoding='utf-8'))
    name = pj.get('name', proj.name)
    # 画幅随项目（缺省 16:9 与既有项目一致）
    W, H = (1080, 1920) if pj.get('aspect') == '9:16' else (1920, 1080)

    durs = {v['id']: v['duration_s'] for v in meta['voices']}
    scene = {k: round(v + 0.35, 3) for k, v in durs.items()}
    ids = sorted(scene.keys())
    total = round(sum(scene.values()), 3)
    starts, acc = {}, 0.0
    for sid in ids:
        starts[sid] = round(acc, 3)
        acc += scene[sid]

    scenes = []
    for i, sid in enumerate(ids):
        scenes.append(
            f'      <div id="el-{sid}" class="scene" data-composition-id="{sid}"\n'
            f'        data-composition-src="compositions/frames/{sid}.html"\n'
            f'        data-start="{starts[sid]}" data-duration="{scene[sid]}" data-track-index="{i}"></div>\n'
            f'      <audio id="el-{sid}-voice" src="audio/{sid}.wav"\n'
            f'        data-start="{starts[sid]}" data-duration="{durs[sid]}" data-track-index="10" data-volume="1"></audio>')

    html = f"""<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width={W}, height={H}" />
    <script src="assets/gsap.min.js"></script>
    <style>
      * {{ margin: 0; padding: 0; box-sizing: border-box; }}
      html, body {{ width: {W}px; height: {H}px; overflow: hidden; background: #000; }}
      #root {{ position: relative; width: {W}px; height: {H}px; overflow: hidden; background: #FDF6E3; }}
      .scene {{ position: absolute; inset: 0; width: 100%; height: 100%; }}
    </style>
  </head>
  <body>
    <div id="root" data-composition-id="main" data-start="0" data-duration="{total}" data-width="{W}" data-height="{H}">
{chr(10).join(scenes)}
    </div>
    <script>
      window.__timelines = window.__timelines || {{}};
      window.__timelines["main"] = gsap.timeline({{ paused: true }});
      // v1 段间硬切（转场为独立验证项）
      (function () {{ var tl = window.__timelines["main"];
        tl.to({{}}, {{ duration: {total} }}, 0);
      }})();
    </script>
  </body>
</html>
"""
    (proj / 'index.html').write_text(html, encoding='utf-8')
    (proj / 'assets').mkdir(exist_ok=True)
    if not (proj / 'assets' / 'gsap.min.js').exists():
        shutil.copy(GSAP_SRC, proj / 'assets' / 'gsap.min.js')
    if not (proj / 'package.json').exists():
        (proj / 'package.json').write_text(json.dumps({
            'name': proj.name.replace(' ', '-'), 'private': True, 'type': 'module',
        }, ensure_ascii=False, indent=2), encoding='utf-8')
    if not (proj / 'meta.json').exists():
        (proj / 'meta.json').write_text(json.dumps(
            {'id': proj.name, 'name': name}, ensure_ascii=False), encoding='utf-8')
    print(f'index.html ✓ 总时长 {total}s，{len(ids)} 段')
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

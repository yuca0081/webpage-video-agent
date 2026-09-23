# -*- coding: utf-8 -*-
"""单项目建片链：生成产物 → m0 compositions → 组装 → check → render。
用法：python ai/film_build.py <项目目录>
（gen 文件由会话 AI 预先写在 <项目目录>/llm/_gen_frames.py）
"""
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
FFBIN = r"C:\Users\86151\AppData\Local\Microsoft\WinGet\Packages\Gyan.FFmpeg_Microsoft.Winget.Source_8wekyb3d8bbwe\ffmpeg-9.0.2-full_build\bin"


def sh(cmd, **kw):
    import os
    import shutil
    print('>', ' '.join(str(c) for c in cmd), flush=True)
    resolved = []
    for c in cmd:
        s = str(c)
        if s in ('npx', 'python'):
            resolved.append(shutil.which(s) or s)
        else:
            resolved.append(s)
    kw.setdefault('cwd', ROOT)
    r = subprocess.run(resolved, capture_output=True, text=True,
                       env={**os.environ, 'PATH': FFBIN + r';' + os.environ['PATH']}, **kw)
    if r.returncode != 0:
        print((r.stdout or '')[-800:], (r.stderr or '')[-400:], flush=True)
    return r


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    pid = proj.name

    # 1. 会话产物 → 产物 JSON（优先项目内 bespoke 生成器，否则通用 defs）
    gen = proj / 'llm' / '_gen_frames.py'
    r = sh(['python', gen]) if gen.exists() else sh(['python', ROOT / 'ai' / 'gen_frames.py', proj])
    if r.returncode != 0:
        return 1
    # 2. 管线落盘合成物（先清旧帧强制重写）
    for f in (proj / 'compositions' / 'frames').glob('*.html'):
        f.unlink()
    r = sh([ROOT / 'bin' / 'm0.exe', 'run', pid, 'compositions'])
    if r.returncode != 0:
        return 1
    # 3. 组装
    r = sh(['python', ROOT / 'ai' / 'assemble.py', proj])
    if r.returncode != 0:
        return 1
    # 4. check（清缓存标记）
    ok = proj / '.hyperframes-ok'
    if ok.exists():
        ok.unlink()
    r = sh(['npx', '--offline', '--yes', 'hyperframes@0.8.55', 'check'], cwd=proj)
    if r.returncode != 0 or 'Check passed' not in (r.stdout or ''):
        print('CHECK FAILED', flush=True)
        print((r.stdout or '')[-2000:], flush=True)
        return 1
    # 5. render（已有成片则跳过）
    if (proj / 'renders' / 'main.mp4').exists():
        print('render 已有，跳过', flush=True)
        return 0
    r = sh(['npx', '--offline', '--yes', 'hyperframes@0.8.55', 'render', '-o', 'renders/main.mp4'], cwd=proj, timeout=None)
    if 'rendered in' not in (r.stdout or ''):
        print('RENDER FAILED', flush=True)
        print((r.stdout or '')[-800:], flush=True)
        return 1
    print(f'✅ {pid} 建片完成', flush=True)
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

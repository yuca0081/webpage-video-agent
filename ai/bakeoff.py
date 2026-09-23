# -*- coding: utf-8 -*-
"""M0 语料 bake-off 驱动：10 篇文章批量过自动化阶段，产出通过率/耗时/成本报告。

每项目流程：
  m0 new → m0 run storyboard（API 模式）→ 风格复用（首片确认的 StylePack）→ ai/tts_align.py
报告：data/bakeoff/report.json
"""
import json
import pathlib
import shutil
import subprocess
import time

ROOT = pathlib.Path(__file__).resolve().parent.parent
BIN = ROOT / 'bin' / 'm0.exe'
SAMPLES = ROOT / 'data' / 'samples'
BAKEOFF = ROOT / 'data' / 'bakeoff'
STYLE_SRC = ROOT / 'data' / 'projects' / 'p20260922-135417' / 'llm' / 'style_samples.json'
NAMES = {
    'cat-land': '猫为什么四脚着地', 'fridge': '冰箱搬热的原理', 'vaccine': '疫苗训练免疫',
    'star-twinkle': '星星为什么眨眼', 'cipher': '密码破译简史', 'beehive': '蜂巢六边形',
    'gps': 'GPS 定位原理', 'bread': '面包为什么发酵', 'aurora': '极光的成因', 'muscle': '运动后肌肉酸痛',
}


def run(cmd, timeout=600):
    t0 = time.time()
    r = subprocess.run([str(c) for c in cmd], capture_output=True, text=True,
                       cwd=ROOT, timeout=timeout)
    return r, time.time() - t0


def latest_project():
    dirs = sorted((ROOT / 'data' / 'projects').iterdir())
    return dirs[-1].name


def find_existing(title):
    """按 project.json 的 name 复用已建项目（批量可重入）。"""
    root = ROOT / 'data' / 'projects'
    if not root.exists():
        return None
    for d in sorted(root.iterdir()):
        pj = d / 'project.json'
        if pj.exists():
            try:
                if json.load(open(pj, encoding='utf-8')).get('name') == title:
                    return d.name
            except Exception:
                pass
    return None


def main():
    BAKEOFF.mkdir(parents=True, exist_ok=True)
    report = {}
    for key, title in NAMES.items():
        entry = {'title': title}
        print(f'\n===== {key}（{title}）=====', flush=True)
        # 1. 建项目（可重入：同名复用）
        pid = find_existing(title)
        if pid:
            entry['reused'] = True
            print('复用已有项目', pid, flush=True)
        else:
            r, dt_new = run([BIN, 'new', SAMPLES / f'{key}.txt', title])
            if '项目已建' not in r.stdout:
                entry['error'] = f'new 失败: {r.stderr[-200:]}'
                report[key] = entry
                continue
            pid = latest_project()
        entry['project_id'] = pid
        pdir = ROOT / 'data' / 'projects' / pid

        # 2. API 分镜
        r, dt_sb = run([BIN, 'run', pid, 'storyboard'], timeout=180)
        manifest = (pdir / 'manifest.jsonl').read_text(encoding='utf-8')
        entry['storyboard_seconds'] = round(dt_sb, 1)
        if 'storyboard.api' in manifest and 'storyboard.saved' in manifest:
            entry['storyboard'] = 'pass_first_try'
            usage = [l for l in manifest.splitlines() if 'llm.usage' in l]
            if usage:
                u = json.loads(usage[-1])['detail']
                entry['tokens'] = u
        elif 'storyboard.api.rejected' in manifest:
            entry['storyboard'] = 'schema_rejected'
        else:
            entry['storyboard'] = 'api_or_other_fail'
            entry['storyboard_note'] = r.stdout[-200:]

        # 3. 风格复用（首片确认的 StylePack）
        if STYLE_SRC.exists():
            shutil.copy(STYLE_SRC, pdir / 'llm' / 'style_samples.json')
            r, dt_style = run([BIN, 'run', pid, 'style_samples'], timeout=60)
            entry['style_reuse'] = 'ok' if 'style.confirmed' in (
                pdir / 'manifest.jsonl').read_text(encoding='utf-8') else 'fail'

        # 4. TTS + 字级对齐
        r, dt_tts = run(['python', ROOT / 'ai' / 'tts_align.py', str(pdir)], timeout=1200)
        ok = (pdir / 'audio_meta.json').exists()
        entry['tts_align'] = 'ok' if ok else 'fail'
        entry['tts_seconds'] = round(dt_tts, 1)
        if ok:
            meta = json.load(open(pdir / 'audio_meta.json', encoding='utf-8'))
            entry['segments'] = len(meta['voices'])
            entry['duration_s'] = round(sum(v['duration_s'] for v in meta['voices']), 1)

        report[key] = entry
        print(json.dumps(entry, ensure_ascii=False), flush=True)
        (BAKEOFF / 'report.json').write_text(
            json.dumps(report, ensure_ascii=False, indent=1), encoding='utf-8')

    # 汇总
    sb_ok = sum(1 for e in report.values() if e.get('storyboard') == 'pass_first_try')
    tts_ok = sum(1 for e in report.values() if e.get('tts_align') == 'ok')
    style_ok = sum(1 for e in report.values() if e.get('style_reuse') == 'ok')
    summary = {
        'n': len(report),
        'storyboard_first_try_pass': f'{sb_ok}/{len(report)}',
        'style_reuse_ok': f'{style_ok}/{len(report)}',
        'tts_align_ok': f'{tts_ok}/{len(report)}',
        'avg_tts_seconds': round(sum(e.get('tts_seconds', 0) for e in report.values()) / max(len(report), 1), 1),
    }
    (BAKEOFF / 'report.json').write_text(
        json.dumps({'summary': summary, 'projects': report}, ensure_ascii=False, indent=1),
        encoding='utf-8')
    print('\n===== 汇总 =====')
    print(json.dumps(summary, ensure_ascii=False, indent=1))


if __name__ == '__main__':
    main()

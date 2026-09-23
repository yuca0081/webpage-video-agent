# -*- coding: utf-8 -*-
"""视觉 QA 门禁（帧述 M0 工具）：逐段检查成片画面的排列与美观。

双模式（VISION_MODE 环境变量，可插拔）：
  session（默认，测试阶段）：只做第 1 步抽帧到 reports/visual_qa_frames/，
      评审由会话中的 AI 用自己的读图能力完成（按同一 rubric 写 reports/visual_qa.json）。
  qwen-vl：抽帧后自动调 qwen-vl-max（dashscope）评审，全自动，商业化阶段启用。

用法：
    python ai/visual_qa.py <项目目录> [renders/main.mp4]

评审 rubric（两种模式共用）：排列正确性 / 布局平衡 / 对齐间距 / 可读性 / 美观艺术性。
报告：reports/visual_qa.json = {segNN: {pass, score, issues[]}}；全段通过才算门禁通过。
依赖：ffmpeg；qwen-vl 模式另需 pip install openai + DASHSCOPE_API_KEY
"""
import base64
import json
import pathlib
import subprocess
import sys

FFBIN = r"C:\Users\86151\AppData\Local\Microsoft\WinGet\Packages\Gyan.FFmpeg_Microsoft.Winget.Source_8wekyb3d8bbwe\ffmpeg-9.0.2-full_build\bin"

RUBRIC = """你是视频画面评审。这是一段科普视频某一分镜「落定后」的画面（1920x1080，底部为字幕带，属于画面一部分）。
按以下维度严格检查，宁可错杀不可放过：
1. 排列正确性：元素是否重叠遮挡（文字压文字/文字压图形主体）、是否越界出画、是否贴边过紧
2. 布局平衡：视觉重心是否失衡、是否有大面积空白区（>25%画面的空区）、元素是否挤在一角
3. 对齐与间距：同类元素是否对齐、间距是否一致、大小层级是否清晰
4. 可读性：文字是否过小（1920 宽下正文应 ≥28px）、颜色对比是否足够
5. 美观与艺术性：配色是否协调、画面是否完整成"作品"、构图是否有设计感
只输出 JSON：
{"pass": true/false, "score": 1-10, "issues": [{"severity": "high|medium|low", "element": "哪个元素", "problem": "什么问题", "suggestion": "怎么改"}]}
pass 标准：无 high 问题且 score >= 7。"""


def extract_frame(mp4: pathlib.Path, t: float, out: pathlib.Path):
    subprocess.run([rf"{FFBIN}\ffmpeg.exe", '-y', '-v', 'error', '-ss', str(t),
                    '-i', str(mp4), '-frames:v', '1', str(out)], check=True)


def b64(p: pathlib.Path) -> str:
    return base64.b64encode(p.read_bytes()).decode()


def review(client, frame: pathlib.Path, seg_key: str, narration: str) -> dict:
    url = f"data:image/png;base64,{b64(frame)}"
    resp = client.chat.completions.create(
        model='qwen-vl-max',
        messages=[{'role': 'user', 'content': [
            {'type': 'image_url', 'image_url': {'url': url}},
            {'type': 'text', 'text': f"分镜名：{seg_key}\n本段旁白：{narration}\n\n{RUBRIC}"},
        ]}],
        temperature=0.1)
    txt = resp.choices[0].message.content.strip()
    if txt.startswith('```'):
        txt = txt.split('```')[1]
        if txt.startswith('json'):
            txt = txt[4:]
    return json.loads(txt)


def main(proj_dir: str, mp4: str | None = None) -> int:
    proj = pathlib.Path(proj_dir)
    video = (proj / mp4) if mp4 else proj / 'renders' / 'main.mp4'  # 相对路径按项目目录解析
    meta = json.load(open(proj / 'audio_meta.json', encoding='utf-8'))
    sb = {s['id']: s for s in json.load(open(proj / 'storyboards' / 'storyboard.json', encoding='utf-8'))['segments']}

    outdir = proj / 'reports' / 'visual_qa_frames'
    outdir.mkdir(parents=True, exist_ok=True)

    # 段终点时刻（场景时长 = 音频 + 0.35s）
    t, ends = 0.0, {}
    for v in meta['voices']:
        t += v['duration_s'] + 0.35
        ends[v['id']] = round(t - 0.3, 2)  # 段落定状态

    for v in meta['voices']:
        sid = v['id']
        frame = outdir / f'{sid}.png'
        extract_frame(video, ends[sid], frame)
        print(f"{sid} 帧 ✓ → {frame}")

    if os.environ.get('VISION_MODE', 'session') == 'session':
        print('VISION_MODE=session：抽帧完成。评审由会话 AI 完成（读图后写 reports/visual_qa.json）。')
        return 0

    import openai
    client = openai.OpenAI(
        api_key=os.environ.get('DASHSCOPE_API_KEY'),
        base_url='https://dashscope.aliyuncs.com/compatible-mode/v1')

    report, all_pass = {}, True
    for v in meta['voices']:
        sid = v['id']
        frame = outdir / f'{sid}.png'
        verdict = review(client, frame, sb[sid]['key'], sb[sid]['narration'])
        verdict['frame'] = str(frame.relative_to(proj))
        report[sid] = verdict
        status = 'PASS' if verdict.get('pass') else 'FAIL'
        if not verdict.get('pass'):
            all_pass = False
        print(f"{sid} [{status}] score={verdict.get('score')}")
        for i in verdict.get('issues', []):
            print(f"   [{i.get('severity')}] {i.get('element')}: {i.get('problem')} → {i.get('suggestion')}")
        (outdir / f'{sid}.verdict.json').write_text(
            json.dumps(verdict, ensure_ascii=False, indent=1), encoding='utf-8')

    (proj / 'reports' / 'visual_qa.json').write_text(
        json.dumps(report, ensure_ascii=False, indent=1), encoding='utf-8')
    print('门禁：', 'PASS' if all_pass else 'FAIL', '→', proj / 'reports' / 'visual_qa.json')
    return 0 if all_pass else 1


import os

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(2)
    sys.exit(main(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else None))

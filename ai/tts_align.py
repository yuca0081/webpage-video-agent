# -*- coding: utf-8 -*-
"""TTS + 字级时间戳对齐（帧述 M0 工具）。

用法：
    python ai/tts_align.py <项目目录>

行为（幂等，缺什么补什么）：
  1. 对 storyboard.json 每段：写 audio/segNN.txt（旁白）
  2. hyperframes tts（Kokoro zf_xiaobei 离线）→ audio/segNN.wav
  3. faster-whisper medium + initial_prompt=旁白 → 字级时间戳 → audio_meta.json

⚠️ hyperframes tts 的 --text-file 必须真实存在，否则路径字符串会被当文本朗读。
依赖：pip install kokoro-onnx soundfile faster-whisper；winget install Gyan.FFmpeg
"""
import json
import pathlib
import shutil
import subprocess
import sys

VOICE, LANG, PINNED = 'zf_xiaobei', 'zh', 'hyperframes@0.8.55'


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    sb = json.load(open(proj / 'storyboards' / 'storyboard.json', encoding='utf-8'))
    npx = shutil.which('npx') or shutil.which('npx.cmd')
    (proj / 'audio').mkdir(exist_ok=True)

    # 1+2. 文本落盘 + TTS（先写全 txt，再出 wav，避免路径朗读坑）
    for seg in sb['segments']:
        (proj / 'audio' / f"{seg['id']}.txt").write_text(seg['narration'], encoding='utf-8')
    for seg in sb['segments']:
        wav = proj / 'audio' / f"{seg['id']}.wav"
        if wav.exists():
            print(seg['id'], 'wav 已有')
            continue
        r = subprocess.run(
            [npx, '--offline', '--yes', PINNED, 'tts', '--text-file', str(proj / 'audio' / f"{seg['id']}.txt"),
             '-v', VOICE, '-l', LANG, '-o', str(wav), '--json'],
            capture_output=True, text=True, timeout=300)
        ok = any(l.startswith('{"ok":true') for l in r.stdout.splitlines())
        print(seg['id'], 'wav ✓' if ok else f"FAIL: {(r.stderr or r.stdout)[-150:]}")
        if not ok:
            return 1

    # 3. 字级时间戳
    from faster_whisper import WhisperModel
    model = WhisperModel('medium', device='cpu', compute_type='int8')
    voices = []
    for seg in sb['segments']:
        sid = seg['id']
        segments, _ = model.transcribe(
            str(proj / 'audio' / f'{sid}.wav'), language='zh',
            word_timestamps=True, condition_on_previous_text=False,
            initial_prompt='以下是普通话的句子。' + seg['narration'])
        words = [{'text': w.word.strip(), 'start': round(w.start, 3), 'end': round(w.end, 3)}
                 for s in segments for w in s.words if w.word.strip()]
        voices.append({'id': sid, 'path': f'audio/{sid}.wav',
                       'duration_s': round(words[-1]['end'], 3) if words else 0, 'words': words})
        print(sid, '对齐 ✓', len(words), '词')
    meta = {'tts_provider': 'kokoro', 'voice_id': VOICE,
            'asr_aligner': 'faster-whisper/medium+initial_prompt', 'voices': voices}
    json.dump(meta, open(proj / 'audio_meta.json', 'w', encoding='utf-8'),
              ensure_ascii=False, indent=2)
    print('audio_meta.json ✓')
    return 0


if __name__ == '__main__':
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(2)
    sys.exit(main(sys.argv[1]))

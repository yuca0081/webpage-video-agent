# -*- coding: utf-8 -*-
"""制作期图片获取（帧述，2026-09-23；2026-09-26 加 AI 生图源）。

扫 llm/comp-*.spec.json 里的 image 元素 → 取图下载到 assets/img/ →
spec 回写本地 src + 来源页；credits.txt 记录出处（自用 1.0 版权留档）。

渲染层只引用本地相对路径（assets/img/imgNN.jpg）——
合成物渲染期零网络，确定性/lint 规则不破。

图源链（按元素 source 字段分流，2026-09-26）：
  source=gen ：AI 生图（智谱 CogView）→ 必应 → Commons
  其他/空    ：必应 → Commons → AI 生图兜底
写实名词（人物/产品/地标/新闻实物）spec 里必须用 search——生图会编造事实。

环境变量：
  IMG_GEN       0 关闭生图源（默认开）
  IMG_GEN_MODEL 生图模型（默认 cogview-3-flash：免费档，右下角带"AI生成"水印；
                cogview-4 无水印但需按量余额，错误码 1113=余额不足）
  IMG_GEN_KEY   生图 API key（缺省用 LLM_API_KEY——智谱同 key 体系，实测套餐 key 可调免费档）

失败兜底：src 置空 → render_spec 画便签占位，管线不死。

用法：python ai/fetch_images.py <项目目录>
"""
import json
import os
import pathlib
import re
import sys
import urllib.parse
import urllib.request

UA = {
    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)',
    'Referer': 'https://cn.bing.com/',
}
MAGIC = {
    b'\xff\xd8\xff': '.jpg',
    b'\x89PNG': '.png',
    b'GIF8': '.gif',
}
GEN_URL = 'https://open.bigmodel.cn/api/paas/v4/images/generations'
GEN_SIZES = {'16:9': '1344x768', '9:16': '768x1344'}


# ── 图源：搜图 ───────────────────────────────────────────────

def search_bing(query, n=10):
    """必应图片异步接口 → [(原图url, 来源页url)]。"""
    q = urllib.parse.quote(query)
    url = f'https://cn.bing.com/images/async?q={q}&first=1&count={n}&mmasync=1'
    try:
        req = urllib.request.Request(url, headers=UA)
        html = urllib.request.urlopen(req, timeout=15).read().decode('utf-8', 'ignore')
    except Exception as e:
        print(f'  bing 搜索失败: {e}')
        return []
    out = []
    for m in re.findall(r'm="({&quot;.*?})"', html)[:n]:
        try:
            d = json.loads(m.replace('&quot;', '"').replace('&#39;', "'").replace('&amp;', '&'))
            if d.get('murl', '').startswith('http'):
                out.append((d['murl'], d.get('purl', '')))
        except json.JSONDecodeError:
            continue
    return out


def search_commons(query, n=8):
    """Wikimedia Commons（海外可达时）。"""
    q = urllib.parse.quote(query)
    url = ('https://commons.wikimedia.org/w/api.php?action=query&format=json'
           f'&generator=search&gsrsearch=filetype:bitmap%20{q}&gsrlimit={n}'
           '&gsrnamespace=6&prop=imageinfo&iiprop=url|size|mime&iiurlwidth=1280')
    try:
        req = urllib.request.Request(url, headers={'User-Agent': 'zhenshu-dev/1.0'})
        data = json.load(urllib.request.urlopen(req, timeout=15))
    except Exception as e:
        print(f'  commons 搜索失败: {e}')
        return []
    out = []
    for p in ((data.get('query') or {}).get('pages') or {}).values():
        ii = (p.get('imageinfo') or [{}])[0]
        if ii.get('mime') in ('image/jpeg', 'image/png') and ii.get('width', 0) >= 480:
            out.append((ii.get('thumburl') or ii.get('url', ''), p.get('title', '')))
    return out


def download(url):
    """下载并校验图片字节。返回 bytes / None。"""
    try:
        r = urllib.request.urlopen(urllib.request.Request(url, headers=UA), timeout=20)
        b = r.read()
    except Exception:
        return None
    if len(b) < 30_000:            # 太小多为占位图/损坏
        return None
    if not any(b.startswith(m) for m in MAGIC):
        return None
    return b


# ── 图源：AI 生图（2026-09-26，智谱 CogView）────────────────

def gen_key():
    return os.environ.get('IMG_GEN_KEY') or os.environ.get('LLM_API_KEY') or ''


def gen_enabled():
    return os.environ.get('IMG_GEN', '1') != '0' and bool(gen_key())


def gen_size(proj: pathlib.Path) -> str:
    """生图尺寸随项目画幅（16:9 横 / 9:16 竖）。"""
    try:
        meta = json.load(open(proj / 'project.json', encoding='utf-8'))
        return GEN_SIZES.get(meta.get('aspect') or '16:9', '1344x768')
    except Exception:
        return '1344x768'


def generate_image(prompt, size):
    """智谱生图 → (bytes, ext) / None。prompt 用中文画面描述。"""
    body = json.dumps({'model': os.environ.get('IMG_GEN_MODEL') or 'cogview-3-flash',
                       'prompt': prompt, 'size': size}).encode('utf-8')
    req = urllib.request.Request(GEN_URL, data=body, method='POST', headers={
        'Authorization': f'Bearer {gen_key()}',
        'Content-Type': 'application/json',
    })
    try:
        resp = json.load(urllib.request.urlopen(req, timeout=90))
        url = resp['data'][0]['url']
    except Exception as e:
        print(f'  生图失败: {e}')
        return None
    data = download(url)
    if not data:
        return None
    ext = next((ext for magic, ext in MAGIC.items() if data.startswith(magic)), '.png')
    return data, ext


# ── 主流程 ───────────────────────────────────────────────────

def fill_image_element(e, proj, size):
    """按 source 分流取图 → (bytes, ext, 出处) / None。"""
    gen_first = e.get('source') == 'gen'
    chain = ['gen', 'bing', 'commons'] if gen_first else ['bing', 'commons', 'gen']
    for how in chain:
        if how == 'gen':
            if not gen_enabled():
                continue
            r = generate_image(e['query'], size)
            if r:
                return r[0], r[1], f'gen:{os.environ.get("IMG_GEN_MODEL") or "cogview-3-flash"}'
        else:
            hits = search_bing(e['query']) if how == 'bing' else search_commons(e['query'])
            for murl, page in hits:
                data = download(murl)
                if data:
                    return data, None, (murl, page)
    return None


def main(proj_dir: str) -> int:
    proj = pathlib.Path(proj_dir)
    spec_files = sorted((proj / 'llm').glob('comp-*.spec.json'))
    if not spec_files:
        print('无 spec，跳过')
        return 0
    img_dir = proj / 'assets' / 'img'
    img_dir.mkdir(parents=True, exist_ok=True)
    seq = len(list(img_dir.iterdir()))
    credits = proj / 'credits.txt'
    size = gen_size(proj)

    got = miss = 0
    for sf in spec_files:
        spec = json.load(open(sf, encoding='utf-8'))
        changed = False
        for e in spec.get('elements', []):
            if e.get('kind') != 'image':
                continue
            query = str(e.get('query', '')).strip()
            if not query:
                continue
            if e.get('src') and (proj / e['src']).exists():
                continue  # 已本地化（幂等）
            r = fill_image_element(e, proj, size)
            if r:
                data, ext, origin = r
                if ext is None:  # 搜图：按 magic 定扩展名
                    ext = next(ext for magic, ext in MAGIC.items() if data.startswith(magic))
                seq += 1
                rel = f'assets/img/img{seq:02d}{ext}'
                (proj / rel).write_bytes(data)
                if isinstance(origin, tuple):  # 搜图：(原图url, 来源页)
                    e['src'], e['page'] = rel, origin[1]
                    with open(credits, 'a', encoding='utf-8') as f:
                        f.write(f'{query}\t{origin[0]}\t{origin[1]}\n')
                else:  # 生图：origin = "gen:<model>"
                    e['src'], e['page'] = rel, origin
                    with open(credits, 'a', encoding='utf-8') as f:
                        f.write(f'{query}\t{origin}\tAI 生成（免费档带"AI生成"水印）\n')
                tag = 'AI生图' if not isinstance(origin, tuple) else '搜图'
                print(f"  {sf.stem} 「{query}」 ✓ {len(data)//1024}KB ({tag})")
                got += 1
                changed = True
            else:
                e['src'] = ''      # 渲染层兜底便签
                print(f"  {sf.stem} 「{query}」 ✗ 无可用图")
                miss += 1
                changed = True
        if changed:
            json.dump(spec, open(sf, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    print(f'图片就绪：{got} 张新取 / {miss} 张无源兜底')
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

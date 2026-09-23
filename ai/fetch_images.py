# -*- coding: utf-8 -*-
"""制作期图片获取（帧述，2026-09-23）。

扫 llm/comp-*.spec.json 里的 image 元素 → 搜图下载到 assets/img/ →
spec 回写本地 src + 来源页；credits.txt 记录出处（自用 1.0 版权留档）。

渲染层只引用本地相对路径（assets/img/imgNN.jpg）——
合成物渲染期零网络，确定性/lint 规则不破。

图源链（按连通性排序，国内实测 2026-09-23）：
  1. 必应图片异步接口（无 key，国内可达）
  2. Wikimedia Commons（海外部署时可达，API 无 key）

失败兜底：src 置空 → render_spec 画便签占位，管线不死。

用法：python ai/fetch_images.py <项目目录>
"""
import json
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


# ── 图源 ─────────────────────────────────────────────────────

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


# ── 主流程 ───────────────────────────────────────────────────

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
            data = None
            for murl, page in search_bing(query) + search_commons(query):
                data = download(murl)
                if data:
                    ext = next(ext for magic, ext in MAGIC.items() if data.startswith(magic))
                    seq += 1
                    rel = f'assets/img/img{seq:02d}{ext}'
                    (proj / rel).write_bytes(data)
                    e['src'], e['page'] = rel, page
                    with open(credits, 'a', encoding='utf-8') as f:
                        f.write(f'{query}\t{murl}\t{page}\n')
                    print(f"  {sf.stem} 「{query}」 ✓ {len(data)//1024}KB")
                    break
            if not data:
                e['src'] = ''      # 渲染层兜底便签
                print(f"  {sf.stem} 「{query}」 ✗ 无可用图")
                miss += 1
                changed = True
                continue
            got += 1
            changed = True
        if changed:
            json.dump(spec, open(sf, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
    print(f'图片就绪：{got} 张新下 / {miss} 张无源兜底')
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1]))

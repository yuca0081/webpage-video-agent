# -*- coding: utf-8 -*-
"""10 篇 bake-off 项目的帧语义设计（会话 AI 产出）。
每项目 = {sid: (body_html, body_js, elements)}；构图原型来自首片 v3 布局系统。"""
import math
import sys
import pathlib

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import stylepack as sp
from stylepack import (INK, PAPER, CORAL, BUTTER, MINT, SKY, TURQ, PEACH, PINK,
                       pop, fade, rise, draw_x, chars_reveal, stagger_pop, stagger_grow,
                       note, note_c, label, big, beam, disc, radial_arrows, converge_beams,
                       polyline, title_chars, cue)

CX, CY = 960, 460  # 内容中轴


def _catland(P):
    seg = {}
    # seg01 标题帧
    sid = 'seg01'
    html = (sp.note(P, 'a-q', 1560, 160, '？', SKY, 6, 64) +
            f'<div class="a-title a-el" id="a-title" data-hf-name="主标题" style="top:330px;font-size:120px;">{sp.title_chars("猫从高处落下")}</div>' +
            f'<div class="a-title a-el" id="a-title2" data-hf-name="副标题" style="top:520px;font-size:80px;color:#6B6B6B;">{sp.title_chars("总能四脚着地？")}</div>' +
            sp.big(P, 'a-cat', 870, 660, '🐈', 150) +
            sp.note(P, 'a-why', 1210, 700, '背朝下也能翻身', MINT, -2, 36))
    js = (pop('#a-cat', .4) + chars_reveal('#a-title', 1.2, .2) + chars_reveal('#a-title2', 2.6, .18, 30) +
          pop('#a-q', 1.0) + rise('#a-why', cue(P, sid, '翻', 5.0)))
    seg[sid] = (html, js, ['主标题', '副标题', '问号便签', '猫', '翻身便签'])
    # seg02 空中翻转
    sid = 'seg02'
    html = (sp.disc(P, 'b-cat', CX, CY, 150, PEACH) +
            f'<div class="b-el" id="b-face" data-hf-name="猫本体" data-layout-allow-occlusion="1" style="top:{CY-95}px;left:{CX-95}px;width:190px;height:190px;font-size:150px;text-align:center;line-height:190px;">🐈</div>' +
            sp.label(P, 'b-l1', 300, 200, '背朝下', fs=44) + sp.label(P, 'b-l2', 1400, 200, '四脚朝下', fs=44) +
            sp.beam(P, 'b-arrow', 760, 720, 400, 20, SKY, 0) +
            sp.label(P, 'b-ms', 830, 770, '几十毫秒内翻转', '#6B6B6B', 34))
    js = (fade('#b-l1', .3) + pop('#b-cat', .8) + pop('#b-face', .8) +
          f"gsap.set('#b-face',{{rotation:180,transformOrigin:'50% 50%'}});\n      tl.to('#b-face',{{rotation:0,duration:1.1,ease:'power2.inOut'}},{cue(P,sid,'翻',4.0):.2f});" +
          fade('#b-l2', cue(P, sid, '四', 6.0)) + draw_x('#b-arrow', cue(P, sid, '翻', 4.0)) + fade('#b-ms', cue(P, sid, '毫秒', 8.0)))
    seg[sid] = (html, js, ['猫', '背朝下', '四脚朝下', '翻转箭头', '毫秒标注'])
    # seg03 前庭系统（放射）
    sid = 'seg03'
    arrs, arrsel = sp.radial_arrows(P, CX, CY, 8, 210, 140, SKY)
    html = (f'<div class="c-el" id="c-ear" data-hf-name="耳朵标注" style="top:180px;left:0;right:0;text-align:center;font-size:110px;">👂</div>' +
            arrs + sp.disc(P, 'c-core', CX, CY, 110, MINT) +
            sp.label(P, 'c-lab', CX-70, CY-30, '前庭', fs=44) +
            sp.note_c(P, 'c-note', 760, '瞬间感知空间姿态', BUTTER, -1.5, 38))
    js = (pop('#c-ear', .3, .5) + pop('#c-core', cue(P, sid, '灵敏', 2.5)) + fade('#c-lab', cue(P, sid, '灵敏', 2.5)) +
          stagger_pop(arrsel, cue(P, sid, '感知', 5.0), .12) + rise('#c-note', cue(P, sid, '姿态', 7.5)))
    seg[sid] = (html, js, ['耳朵', '前庭圆盘', '放射信号×8', '姿态便签'])
    # seg04 脊柱扭转（三步流程）
    sid = 'seg04'
    cards = ''.join(sp.note(P, f'd-s{i}', 240+i*540, 400, t, [SKY, BUTTER, MINT][i], [2,-2,1.5][i], 36)
                    for i, t in enumerate(['前半身 先转', '后半身 反向转', '顺势 跟上']))
    links = ''.join(sp.beam(P, f'd-l{i}', 500+i*540, 470, 200, 12, '#FFFFFF', 0) for i in range(2))
    html = (sp.label(P, 'd-spine', 760, 240, '脊柱像弹簧分段扭转', fs=46) + cards + links +
            f'<div class="d-el" id="d-cat" data-hf-name="猫剪影" data-layout-allow-occlusion="1" style="top:640px;left:870px;font-size:120px;">🐈</div>')
    js = (fade('#d-spine', .3) + stagger_pop('[id^="d-s"]', cue(P, sid, '前半', 2.0), .9) +
          draw_x('#d-l0', cue(P, sid, '后半', 5.5)) + draw_x('#d-l1', cue(P, sid, '跟', 8.5)) +
          pop('#d-cat', cue(P, sid, '守恒', 11.0)))
    seg[sid] = (html, js, ['脊柱标注', '三步卡×3', '连接×2', '猫'])
    # seg05 无锁骨与缓冲（左右对比）
    sid = 'seg05'
    html = (f'<div class="e-el" id="e-cat" data-hf-name="猫居中" data-layout-allow-occlusion="1" style="top:380px;left:850px;font-size:170px;">🐈</div>' +
            sp.note(P, 'e-left', 260, 300, '没有锁骨\n肩胛骨悬挂', SKY, 2, 36) +
            sp.note(P, 'e-right', 1240, 300, '肉垫 + 皮毛\n缓冲落地', MINT, -2, 36) +
            sp.label(P, 'e-cap', 760, 740, '柔软的身体 = 天然气垫', fs=40))
    js = (pop('#e-cat', .4) + rise('#e-left', cue(P, sid, '锁骨', 2.5)) + rise('#e-right', cue(P, sid, '缓冲', 6.5)) +
          fade('#e-cap', cue(P, sid, '冲击', 11.0)))
    seg[sid] = (html, js, ['猫', '左便签', '右便签', '气垫标注'])
    # seg06 高空降落伞
    sid = 'seg06'
    pl = sp.polyline(P, 'f-curve', '160,320 560,240 1000,300 1440,180 1760,240', 1920, 420, 560)
    html = (f'<div class="f-el" id="f-cat" data-hf-name="降落伞猫" data-layout-allow-occlusion="1" style="top:200px;left:860px;font-size:130px;transform:rotate(8deg);">🐈</div>' +
            sp.big(P, 'f-floor', 200, 200, '5 层楼 ↑', 72) +
            sp.note(P, 'f-safe', 1290, 330, '展开四肢\n如降落伞', BUTTER, 2, 34) + pl +
            sp.label(P, 'f-lab', 1180, 1000-260, '高度越高 · 存活率反而回升', '#6B6B6B', 34))
    js = (pop('#f-cat', .4) + fade('#f-floor', cue(P, sid, '层', 2.0)) + rise('#f-safe', cue(P, sid, '降落伞', 5.0)) +
          fade('#f-curve', cue(P, sid, '存活', 9.0), .8) + fade('#f-lab', cue(P, sid, '回升', 12.0)))
    seg[sid] = (html, js, ['降落伞猫', '楼层大字', '伞便签', '存活率曲线', '结论标注'])
    # seg07 收尾
    sid = 'seg07'
    pl = sp.polyline(P, 'g-path', '160,220 620,180 960,260 1400,160 1760,220', 1920, 420, 480, nodes=[(620, 180), (1400, 160)])
    html = (f'<div class="g-el" id="g-cat" data-hf-name="落地猫" data-layout-allow-occlusion="1" style="top:240px;left:850px;font-size:160px;">🐈</div>' + pl +
            f'<div class="g-title g-el" id="g-slog" data-hf-name="收束标语" style="top:700px;font-size:74px;">{sp.title_chars("进化打磨的物理大师")}</div>')
    js = (pop('#g-cat', .4) + fade('#g-path', cue(P, sid, '落', 1.5), .8) +
          chars_reveal('#g-slog', cue(P, sid, '大师', 4.0), .16))
    seg[sid] = (html, js, ['落地猫', '路径', '收束标语'])
    return seg


def _fridge(P):
    seg = {}
    sid = 'seg01'
    html = (f'<div class="a-el" id="a-box" data-hf-name="冰箱" style="top:300px;left:770px;width:380px;height:340px;border:4px solid {INK};border-radius:14px;background:#fff;box-shadow:6px 6px 0 {INK};"><div style="position:absolute;top:20px;left:20px;right:110px;bottom:20px;background:{SKY};border-radius:8px;opacity:.7;"></div></div>' +
            sp.label(P, 'a-cold', 500, 380, '箱内 冷', '#1E6B8B', 40) + sp.label(P, 'a-hot', 1330, 380, '箱外 热', '#B91C1C', 40) +
            sp.beam(P, 'a-out', 560, 470, 220, 14, CORAL, 0) + sp.beam(P, 'a-in', 1160, 470, 220, 14, '#38BDF8', 180) +
            sp.note_c(P, 'a-note', 760, '冰箱不制造冷，只是搬运热', MINT, -1.5, 40))
    js = (pop('#a-box', .4) + fade('#a-cold', cue(P, sid, '箱', 2.0)) + fade('#a-hot', cue(P, sid, '热', 3.0)) +
          draw_x('#a-out', cue(P, sid, '搬', 6.0)) + draw_x('#a-in', cue(P, sid, '搬', 6.0)) + rise('#a-note', cue(P, sid, '制冷剂', 10.0)))
    seg[sid] = (html, js, ['冰箱', '冷标注', '热标注', '外移热流', '回流冷流', '搬运便签'])
    sid = 'seg02'
    html = (sp.beam(P, 'b-pipe', 260, CY, 1400, 40, SKY, 0) +
            f'<div class="b-el" id="b-bub" data-hf-name="沸腾气泡" data-layout-allow-occlusion="1" style="top:{CY-90}px;left:880px;font-size:90px;">🫧</div>' +
            sp.label(P, 'b-absorb', 760, 250, '沸腾汽化 · 吸走热量', fs=44) +
            sp.note(P, 'b-food', 240, 620, '箱内食物的热', BUTTER, -2, 34) +
            sp.label(P, 'b-gas', 1350, 620, '液体 → 气体', '#6B6B6B', 34))
    js = (draw_x('#b-pipe', .4) + fade('#b-absorb', cue(P, sid, '沸', 2.0)) + pop('#b-bub', cue(P, sid, '沸腾', 3.5)) +
          rise('#b-food', cue(P, sid, '吸', 6.0)) + fade('#b-gas', cue(P, sid, '气体', 10.0)))
    seg[sid] = (html, js, ['管道', '沸腾标注', '气泡', '食物便签', '相变标注'])
    sid = 'seg03'
    html = (sp.disc(P, 'c-comp', 500, CY, 110, PEACH) +
            sp.label(P, 'c-clab', 380, CY-20, '压缩机', fs=44) +
            sp.beam(P, 'c-hot', 650, CY-11, 700, 22, CORAL, 0) +
            sp.note(P, 'c-heat', 1370, 240, '热量排到室内', BUTTER, 2, 36) +
            sp.label(P, 'c-lig', 1350, 560, '气体 → 液体', '#6B6B6B', 34))
    js = (pop('#c-comp', .4) + fade('#c-lab', .8) + draw_x('#c-hot', cue(P, sid, '压缩', 3.0)) +
          rise('#c-heat', cue(P, sid, '散热', 7.0)) + fade('#c-lig', cue(P, sid, '液体', 12.0)))
    seg[sid] = (html, js, ['压缩机', '标注', '高温热流', '散热便签', '液化标注'])
    sid = 'seg04'
    import math as _m
    arcs = []
    for i, (dx, dy) in enumerate([(0, -190), (300, 0), (0, 190), (-300, 0)]):
        ang = [0, 90, 180, 270][i]
        arcs.append(f'<div class="a-el" data-cy="{i}" data-hf-name="循环箭头{i+1}" style="top:{CY+dy-9}px;left:{CX+dx-110}px;width:220px;height:18px;background:{TURQ};border:3px solid {INK};border-radius:10px;transform:rotate({ang}deg);"></div>')
    html = (''.join(arcs) + sp.disc(P, 'd-core', CX, CY, 90, MINT) +
            sp.label(P, 'd-lab', CX-64, CY+120, '循环', fs=44) +
            sp.label(P, 'd-in', CX-500, CY-22, '箱内吸热', '#1E6B8B', 36) +
            sp.label(P, 'd-out', CX+260, CY-22, '箱外放热', '#B91C1C', 36))
    js = (pop('#d-core', .4) + fade('#d-lab', .8) + stagger_pop('[data-cy]', cue(P, sid, '循环', 2.0), .5) +
          fade('#d-in', cue(P, sid, '吸', 6.0)) + fade('#d-out', cue(P, sid, '散', 8.0)))
    seg[sid] = (html, js, ['循环圆盘', '循环箭头×4', '吸热标注', '放热标注'])
    sid = 'seg05'
    html = (f'<div class="e-el" id="e-fridge" data-hf-name="冰箱发热" data-layout-allow-occlusion="1" style="top:300px;left:800px;font-size:230px;">🧊</div>' +
            f'<div class="e-el" id="e-hand" data-hf-name="手摸侧面" data-layout-allow-occlusion="1" style="top:430px;left:1180px;font-size:130px;">✋</div>' +
            f'<div class="e-title e-el" id="e-slog" data-hf-name="点题标语" style="top:680px;font-size:80px;">{sp.title_chars("热量搬运机器")}</div>')
    js = (pop('#e-fridge', .4) + fade('#e-hand', cue(P, sid, '摸', 2.5)) +
          chars_reveal('#e-slog', cue(P, sid, '搬运', 6.0), .16))
    seg[sid] = (html, js, ['冰箱', '手', '点题标语'])
    return seg


def _vaccine(P):
    seg = {}
    sid = 'seg01'
    dots = ''.join(f'<div class="a-el" data-s="{i}" data-hf-name="免疫细胞{i+1}" style="top:{400+(i%3)*70}px;left:{700+(i//3)*110}px;width:44px;height:44px;background:{TURQ};border:3px solid {INK};border-radius:50%;"></div>' for i in range(6))
    html = (f'<div class="a-el" id="a-body" data-hf-name="人体轮廓" style="top:250px;left:600px;width:360px;height:520px;border:4px solid {INK};border-radius:170px 170px 60px 60px;background:#fff;box-shadow:6px 6px 0 {INK};"></div>' +
            dots + sp.note(P, 'a-vac', 1210, 300, '💉 疫苗', PEACH, 2, 40) +
            sp.label(P, 'a-mem', 1240, 520, '记忆部队', fs=40))
    js = (pop('#a-body', .4) + stagger_pop('[data-s]', cue(P, sid, '细胞', 3.0), .3) +
          rise('#a-vac', cue(P, sid, '疫苗', 6.5)) + fade('#a-mem', cue(P, sid, '训练', 8.5)))
    seg[sid] = (html, js, ['人体轮廓', '免疫细胞×6', '疫苗便签', '记忆标注'])
    sid = 'seg02'
    html = (f'<div class="b-el" id="b-virus" data-hf-name="病原体" style="top:{CY-80}px;left:330px;width:160px;height:160px;background:{PINK};border:4px solid {INK};border-radius:50%;box-shadow:5px 5px 0 {INK};"></div>' +
            sp.label(P, 'b-key', 640, CY-24, '抗体 = 钥匙', fs=40) +
            sp.beam(P, 'b-fit', 980, CY-11, 260, 22, MINT, 0) +
            sp.disc(P, 'b-lock', 1420, CY, 100, BUTTER) +
            sp.note_c(P, 'b-note', 700, '抗原对上就锁死', SKY, -1.5, 36))
    js = (pop('#b-virus', .4) + fade('#b-key', cue(P, sid, '抗体', 4.0)) + draw_x('#b-fit', cue(P, sid, '锁', 7.0)) +
          pop('#b-lock', cue(P, sid, '病原', 9.0)) + rise('#b-note', cue(P, sid, '抗原', 12.0)))
    seg[sid] = (html, js, ['病原体', '抗体标注', '契合梁', '锁盘', '便签'])
    sid = 'seg03'
    bars = ''.join(f'<div class="c-el" data-r="{i}" data-hf-name="病原增长{i+1}" style="top:{560-i*60}px;left:{560+i*160}px;width:100px;height:{120+i*60}px;background:{PINK};border:3px solid {INK};border-radius:8px;"></div>' for i in range(4))
    html = (sp.label(P, 'c-slow', 560, 200, '首次应答：慢', fs=46) + bars +
            sp.label(P, 'c-t', 560, 740, '———— 时间 ————', '#6B6B6B', 34) +
            sp.note(P, 'c-weak', 1330, 300, '人已病重', PEACH, 2, 38))
    js = (fade('#c-slow', .3) + stagger_grow('[data-r]', 1.5, .5) + fade('#c-t', 2.0) + rise('#c-weak', cue(P, sid, '病重', 5.5)))
    seg[sid] = (html, js, ['慢标注', '增长柱×4', '时间轴', '病重便签'])
    sid = 'seg04'
    cards = ''.join(sp.note(P, f'd-c{i}', 240+i*540, 350, t, [SKY, BUTTER, MINT][i], [2,-2,1.5][i], 34)
                    for i, t in enumerate(['灭活病原体', '抗原片段', 'mRNA 指令']))
    html = (f'<div class="d-el" id="d-syr" data-hf-name="注射器" data-layout-allow-occlusion="1" style="top:180px;left:850px;font-size:110px;">💉</div>' + cards +
            sp.label(P, 'd-l1', 500, 470, '安全演练', fs=40) +
            sp.note_c(P, 'd-note', 640, '免疫细胞：记住这个敌人', MINT, -1.5, 38))
    js = (pop('#d-syr', .4) + stagger_pop('[id^="d-c"]', cue(P, sid, '病原', 3.0), .8) +
          fade('#d-l1', cue(P, sid, '演练', 8.0)) + rise('#d-note', cue(P, sid, '记', 11.0)))
    seg[sid] = (html, js, ['注射器', '三卡片', '演练标注', '记忆便签'])
    sid = 'seg05'
    arrs, arrsel = sp.radial_arrows(P, CX, CY, 10, 190, 170, MINT)
    html = (sp.disc(P, 'e-mem', CX, CY, 110, TURQ) + sp.label(P, 'e-mlab', CX-90, CY-22, '记忆 B', fs=42) +
            arrs + sp.note(P, 'e-stop', 1350, 200, '抗体大军拦截', BUTTER, 2, 36) +
            sp.label(P, 'e-fast', 220, 200, '几天内拉起', fs=38))
    js = (pop('#e-mem', .4) + fade('#e-mlab', .9) + stagger_pop(arrsel, cue(P, sid, '激活', 3.5), .15) +
          rise('#e-stop', cue(P, sid, '拦', 8.0)) + fade('#e-fast', cue(P, sid, '几', 2.0)))
    seg[sid] = (html, js, ['记忆B盘', '抗体放射×10', '拦截便签', '快速标注'])
    sid = 'seg06'
    html = (f'<div class="f-el" id="f-syr" data-hf-name="针管" data-layout-allow-occlusion="1" style="top:280px;left:700px;font-size:150px;">💉</div>' +
            f'<div class="f-el" id="f-exam" data-hf-name="试卷" data-layout-allow-occlusion="1" style="top:280px;left:1050px;font-size:150px;">📝</div>' +
            f'<div class="f-title f-el" id="f-slog" data-hf-name="收束标语" style="top:660px;font-size:78px;">{sp.title_chars("一针疫苗 = 一次模拟考")}</div>')
    js = (pop('#f-syr', .4) + pop('#f-exam', 1.2) + chars_reveal('#f-slog', cue(P, sid, '模拟', 3.5), .14))
    seg[sid] = (html, js, ['针管', '试卷', '收束标语'])
    return seg


def _startwinkle(P):
    seg = {}
    sid = 'seg01'
    stars = ''.join(f'<div class="a-el" data-st="{i}" data-hf-name="星{i+1}" style="top:{180+(i*97)%420}px;left:{240+(i*211)%1440}px;font-size:{30+(i%3)*14}px;color:{INK};">✦</div>' for i in range(9))
    html = (f'<div class="a-el" id="a-night" data-hf-name="夜幕" style="inset:0;background:linear-gradient(#22304A 0%,#3A4A6B 70%,{PAPER} 100%);"></div>' +
            stars + f'<div class="a-title a-el" id="a-title" data-hf-name="标题" style="top:640px;font-size:96px;color:#fff;">{sp.title_chars("星星在眨眼？")}</div>')
    js = (fade('#a-night', .2, .5) + f"gsap.set('[data-st]',{{opacity:.4}});\n      tl.to('[data-st]',{{opacity:1,duration:.3,stagger:.15,yoyo:false,ease:'none'}},.6);" +
          chars_reveal('#a-title', cue(P, sid, '眨', 4.0), .16))
    seg[sid] = (html, js, ['夜幕', '星×9', '标题'])
    sid = 'seg02'
    layers = ''.join(f'<div class="b-el" data-lay="{i}" data-hf-name="大气层{i+1}" style="top:{300+i*130}px;left:520px;width:1200px;height:110px;background:rgba(56,189,248,{.12+i*.08});border:3px solid {INK};border-radius:10px;"></div>' for i in range(3))
    html = (f'<div class="b-el" id="b-star" data-hf-name="恒星" style="top:130px;left:905px;font-size:90px;">✦</div>' + layers +
            sp.beam(P, 'b-ray', 1050, 220, 620, 12, '#FFF', 12) +
            sp.label(P, 'b-atm', 250, 420, '大气层', fs=44))
    js = (pop('#b-star', .4) + stagger_grow('[data-lay]', cue(P, sid, '大气', 2.5), .5, 'center center') +
          draw_x('#b-ray', cue(P, sid, '穿', 7.0), .8) + fade('#b-atm', cue(P, sid, '大气', 2.5)))
    seg[sid] = (html, js, ['恒星', '大气层×3', '穿层光', '大气标注'])
    sid = 'seg03'
    bands = ''.join(f'<div class="c-el" data-b="{i}" data-hf-name="气团{i+1}" style="top:{280+i*150}px;left:{300+(i%2)*140}px;width:{700+(i%3)*200}px;height:110px;background:{[SKY,BUTTER][i%2]};border:3px solid {INK};border-radius:55px;opacity:.85;transform:rotate({(i%2)*4-2}deg);"></div>' for i in range(4))
    html = (bands + sp.label(P, 'c-hot', 430, 300, '热', '#B91C1C', 40) + sp.label(P, 'c-cold', 1230, 450, '冷', '#1E6B8B', 40) +
            sp.note_c(P, 'c-note', 740, '密度与折射率起伏', MINT, -1, 36))
    js = (stagger_pop('[data-b]', cue(P, sid, '气团', 2.0), .5) + fade('#c-hot', cue(P, sid, '热', 3.5)) +
          fade('#c-cold', cue(P, sid, '密', 5.5)) + rise('#c-note', cue(P, sid, '折射', 9.0)))
    seg[sid] = (html, js, ['气团×4', '热标注', '冷标注', '折射便签'])
    sid = 'seg04'
    pl = sp.polyline(P, 'd-bend', '960,140 900,300 1010,470 940,650', 1920, 800, 140, nodes=[])
    html = (f'<div class="d-el" id="d-star" data-hf-name="恒星" style="top:80px;left:905px;font-size:90px;">✦</div>' + pl +
            f'<div class="d-el" id="d-eye" data-hf-name="观察者" style="top:820px;left:890px;font-size:100px;">👁</div>' +
            sp.note(P, 'd-flick', 1300, 300, '忽明忽暗', BUTTER, 2, 38))
    js = (pop('#d-star', .3) + fade('#d-bend', cue(P, sid, '弯', 2.5), 1.0) + pop('#d-eye', cue(P, sid, '眼', 6.0)) +
          rise('#d-flick', cue(P, sid, '闪', 9.0)))
    seg[sid] = (html, js, ['恒星', '弯折光路', '观察者', '闪烁便签'])
    sid = 'seg05'
    html = (sp.label(P, 'e-star', 380, 260, '恒星 = 光点', fs=44) +
            f'<div class="e-el" id="e-sp" data-hf-name="闪烁光点" style="top:330px;left:450px;font-size:100px;">✦</div>' +
            sp.label(P, 'e-plan', 1240, 260, '行星 = 小圆面', fs=44) +
            sp.disc(P, 'e-disc', 1330, 420, 90, TURQ) +
            sp.note_c(P, 'e-note', 700, '稳定 vs 眨眼', MINT, -1, 38))
    js = (fade('#e-star', .3) + pop('#e-sp', .8) + fade('#e-plan', cue(P, sid, '行星', 4.0)) +
          pop('#e-disc', cue(P, sid, '行星', 4.0)) + rise('#e-note', cue(P, sid, '抵消', 10.0)))
    seg[sid] = (html, js, ['恒星标注', '光点', '行星标注', '圆面', '对比便签'])
    sid = 'seg06'
    html = (f'<div class="f-el" id="f-space" data-hf-name="太空黑幕" style="inset:0;background:#22304A;"></div>' +
            f'<div class="f-el" id="f-star" data-hf-name="稳定恒星" style="top:300px;left:890px;font-size:130px;">✦</div>' +
            f'<div class="f-title f-el" id="f-slog" data-hf-name="收束标语" style="top:640px;font-size:76px;color:#fff;">{sp.title_chars("没有大气，星光笔直")}</div>')
    js = (fade('#f-space', .2, .5) + pop('#f-star', .8) + chars_reveal('#f-slog', cue(P, sid, '笔直', 4.0), .15))
    seg[sid] = (html, js, ['太空黑幕', '恒星', '收束标语'])
    return seg


def _cipher(P):
    seg = {}
    sid = 'seg01'
    strip = ''.join(f'<div class="a-el" data-ch="{i}" data-hf-name="字母{i+1}" style="top:400px;left:{300+i*100}px;width:80px;height:80px;border:3px solid {INK};border-radius:10px;background:#fff;text-align:center;line-height:74px;font-size:40px;font-weight:700;">{c}</div>' for i, c in enumerate('ABCD'))
    strip2 = ''.join(f'<div class="a-el" data-sh="{i}" data-hf-name="移位字母{i+1}" style="top:560px;left:{300+i*100}px;width:80px;height:80px;border:3px solid {INK};border-radius:10px;background:{MINT};text-align:center;line-height:74px;font-size:40px;font-weight:700;">{c}</div>' for i, c in enumerate('DEFG'))
    html = (sp.label(P, 'a-k', 620, 240, '凯撒密码：整体移 3 位', fs=44) + strip +
            f'<div class="a-el" id="a-arr" data-hf-name="下移箭头" style="top:492px;left:620px;font-size:44px;">⬇</div>' + strip2 +
            sp.note(P, 'a-25', 1360, 340, '25 次就试穿', CORAL, 2, 38))
    js = (fade('#a-k', .3) + stagger_pop('[data-ch]', 1.2, .15) + fade('#a-arr', 2.6) +
          stagger_pop('[data-sh]', cue(P, sid, '位移', 3.5), .15) + rise('#a-25', cue(P, sid, '尝试', 9.0)))
    seg[sid] = (html, js, ['标题', '明文字母×4', '箭头', '密文字母×4', '25次便签'])
    sid = 'seg02'
    bars = ''.join(f'<div class="c-el" data-hb="{i}" data-hf-name="频柱{i+1}" style="top:{640-([90,150,240,120,80,110][i])}px;left:{400+i*180}px;width:120px;height:{[90,150,240,120,80,110][i]}px;background:{[SKY,BUTTER,CORAL,MINT,PEACH,PINK][i]};border:3px solid {INK};border-radius:8px;"></div>' for i in range(6))
    html = (sp.label(P, 'c-t', 640, 180, '统计符号频率', fs=44) + bars +
            sp.label(P, 'c-e', 940, 330, '最高 → e', fs=44) +
            sp.note(P, 'c-broken', 1330, 500, '密码锁碎裂', PEACH, -2, 38))
    js = (fade('#c-t', .3) + stagger_grow('[data-hb]', 1.2, .22) + fade('#c-e', cue(P, sid, '频率', 5.0)) +
          rise('#c-broken', cue(P, sid, '瓦解', 10.0)))
    seg[sid] = (html, js, ['标题', '频柱×6', 'e标注', '碎裂便签'])
    sid = 'seg03'
    rotors = ''.join(sp.disc(P, f'd-r{i}', 640+i*240, 400, 80, [SKY, BUTTER, MINT][i]) for i in range(3))
    html = (rotors + sp.label(P, 'd-t', 520, 200, '恩尼格玛 · 每日换设置', fs=44) +
            sp.note(P, 'd-weather', 1290, 300, '「天气」必现', BUTTER, 2, 38) +
            sp.note_c(P, 'd-note', 640, '用猜测的明文反推设置', MINT, -1.5, 36))
    js = (stagger_pop('[id^="d-r"]', 1.0, .4) + fade('#d-t', .3) + rise('#d-weather', cue(P, sid, '天气', 5.0)) +
          rise('#d-note', cue(P, sid, '反推', 10.0)))
    seg[sid] = (html, js, ['转子×3', '标题', '天气便签', '反推便签'])
    sid = 'seg04'
    html = (sp.disc(P, 'e-p1', 560, 380, 80, SKY) + sp.label(P, 'e-p1l', 500, 490, '大素数 p', fs=36) +
            f'<div class="e-el" id="e-mul" data-hf-name="乘号" style="top:330px;left:760px;font-size:90px;">×</div>' +
            sp.disc(P, 'e-p2', 960, 380, 80, SKY) + sp.label(P, 'e-p2l', 900, 490, '大素数 q', fs=36) +
            f'<div class="e-el" id="e-lock" data-hf-name="公开的锁" data-layout-allow-occlusion="1" style="top:300px;left:1180px;font-size:130px;">🔒</div>' +
            sp.note(P, 'e-hard', 300, 620, '反推 = 宇宙年龄', CORAL, -2, 38))
    js = (pop('#e-p1', .3) + fade('#e-mul', .8) + pop('#e-p2', 1.1) + pop('#e-lock', cue(P, sid, '锁', 3.5)) +
          fade('#e-p1l', .8) + fade('#e-p2l', 1.4) + rise('#e-hard', cue(P, sid, '宇宙', 9.0)))
    seg[sid] = (html, js, ['素数p', '乘号', '素数q', '公开的锁', '宇宙年龄便签'])
    sid = 'seg05'
    html = (f'<div class="e-el" id="e-lock2" data-hf-name="锁" data-layout-allow-occlusion="1" style="top:300px;left:700px;font-size:140px;">🔒</div>' +
            f'<div class="e-el" id="e-key" data-hf-name="钥匙" data-layout-allow-occlusion="1" style="top:300px;left:1080px;font-size:140px;">🗝</div>' +
            f'<div class="e-title e-el" id="e-slog" data-hf-name="收束标语" style="top:660px;font-size:76px;">{sp.title_chars("换锁与开锁的军备竞赛")}</div>')
    js = (pop('#e-lock2', .4) + pop('#e-key', 1.2) + chars_reveal('#e-slog', cue(P, sid, '竞赛', 3.5), .14))
    seg[sid] = (html, js, ['锁', '钥匙', '收束标语'])
    return seg


def _beehive(P):
    seg = {}
    sid = 'seg01'
    import math as _m
    hexes = []
    for r in range(3):
        for c in range(6):
            x, y = 260+c*250+(r % 2)*125, 200+r*215
            pts = ','.join(f'{x+60*_m.cos(_m.radians(60*k-30)):.0f},{y+60*_m.sin(_m.radians(60*k-30)):.0f}' for k in range(6))
            hexes.append(f'<polygon points="{pts}" fill="{[BUTTER,MINT,SKY][(r+c)%3]}" stroke="{INK}" stroke-width="4" data-hex="{r*6+c}" data-hf-name="蜂房{r*6+c+1}"/>')
    html = (f'<svg class="a-el" id="a-hex" data-hf-name="蜂巢网格" viewBox="0 0 1920 900" style="position:absolute;top:0;left:0;width:1920px;height:900px;">{"".join(hexes)}</svg>' +
            f'<div class="a-el" id="a-q" data-hf-name="问号" style="top:330px;left:850px;font-size:200px;font-weight:700;">？</div>')
    js = (f"gsap.set('#a-hex polygon',{{opacity:0}});\n      tl.to('#a-hex polygon',{{opacity:1,duration:.3,stagger:.04,ease:'none'}},.4);" +
          pop('#a-q', cue(P, sid, '数学', 5.0)))
    seg[sid] = (html, js, ['蜂巢网格×18', '问号'])
    sid = 'seg02'
    html = (sp.disc(P, 'b-circle', 400, 400, 100, SKY) + sp.label(P, 'b-cl', 320, 540, '圆 · 留缝', fs=36) +
            sp.note(P, 'b-tri', 800, 300, '三角 · 方\n能铺满', BUTTER, 2, 34) +
            sp.note(P, 'b-hex', 1290, 300, '六边形\n面积最大', MINT, -2, 36) +
            sp.label(P, 'b-key', 1150, 560, '同样周长 → 最多空间', fs=40))
    js = (pop('#b-circle', .4) + fade('#b-cl', .8) + rise('#b-tri', cue(P, sid, '三角', 4.0)) +
          rise('#b-hex', cue(P, sid, '六边', 7.5)) + fade('#b-key', cue(P, sid, '面积', 11.0)))
    seg[sid] = (html, js, ['圆形', '圆标注', '三角方便签', '六边方便签', '关键标注'])
    sid = 'seg03'
    import math as _m
    x, y = 380, 330
    pts = ','.join(f'{x+110*_m.cos(_m.radians(60*k-30)):.0f},{y+110*_m.sin(_m.radians(60*k-30)):.0f}' for k in range(6))
    html = (f'<svg class="b-el" id="c-hex" data-hf-name="大蜂房" viewBox="300 190 380 300" style="position:absolute;top:190px;left:300px;width:380px;height:300px;"><polygon points="{pts}" fill="{BUTTER}" stroke="{INK}" stroke-width="6"/></svg>' +
            sp.big(P, 'c-8', 900, 300, '8kg 蜂蜜', 64) + sp.label(P, 'c-eq', 900, 420, '⬇ 只换 1kg 蜂蜡', fs=40) +
            sp.note_c(P, 'c-note', 700, '每一分蜂蜡都要省', MINT, -1, 38))
    js = (pop('#c-hex', .4) + fade('#c-8', cue(P, sid, '蜂蜜', 3.0)) + fade('#c-eq', cue(P, sid, '蜂蜡', 5.5)) +
          rise('#c-note', cue(P, sid, '省', 9.0)))
    seg[sid] = (html, js, ['大蜂房', '蜂蜜大字', '换算标注', '省蜡便签'])
    sid = 'seg04'
    circles = ''.join(f'<div class="e-el" data-bub="{i}" data-hf-name="泡泡{i+1}" style="top:{280+(i//4)*120}px;left:{560+(i%4)*200}px;width:100px;height:100px;background:{[SKY,MINT,BUTTER,PEACH][i%4]};border:3px solid {INK};border-radius:50%;opacity:.85;"></div>' for i in range(8))
    html = (circles + sp.label(P, 'e-t1', 560, 200, '圆管受热 → 挤成六边形', fs=42) +
            sp.label(P, 'e-t2', 760, 640, '和肥皂泡一个道理', '#6B6B6B', 36))
    js = (stagger_pop('[data-bub]', 1.0, .2) + fade('#e-t1', cue(P, sid, '张力', 4.0)) + fade('#e-t2', cue(P, sid, '肥皂', 10.0)))
    seg[sid] = (html, js, ['泡泡×8', '张力标注', '肥皂泡标注'])
    sid = 'seg05'
    html = (f'<div class="f-el" id="f-hive" data-hf-name="蜂巢" data-layout-allow-occlusion="1" style="top:270px;left:810px;font-size:220px;">🍯</div>' +
            sp.note(P, 'f-math', 420, 320, '数学最优解', SKY, -2, 38) +
            sp.note(P, 'f-phys', 1300, 320, '物理自动解', MINT, 2, 38) +
            f'<div class="f-title f-el" id="f-slog" data-hf-name="收束标语" style="top:680px;font-size:72px;">{sp.title_chars("蜜蜂不用学几何")}</div>')
    js = (pop('#f-hive', .4) + rise('#f-math', cue(P, sid, '数学', 2.5)) + rise('#f-phys', cue(P, sid, '物理', 5.0)) +
          chars_reveal('#f-slog', cue(P, sid, '账', 8.5), .14))
    seg[sid] = (html, js, ['蜂巢', '数学便签', '物理便签', '收束标语'])
    return seg


def _gps(P):
    seg = {}
    sid = 'seg01'
    sats = ''.join(f'<div class="a-el" data-sat="{i}" data-hf-name="卫星{i+1}" data-layout-allow-occlusion="1" style="top:{180+(i%2)*90}px;left:{520+(i//2)*330}px;font-size:76px;">🛰</div>' for i in range(4))
    html = (f'<div class="a-el" id="a-phone" data-hf-name="手机" data-layout-allow-occlusion="1" style="top:520px;left:855px;font-size:150px;">📱</div>' +
            sats + sp.label(P, 'a-n', 800, 430, '2 万公里高空 · 30+ 颗', '#6B6B6B', 34))
    js = (pop('#a-phone', .4) + stagger_pop('[data-sat]', cue(P, sid, '卫星', 3.0), .3) + fade('#a-n', cue(P, sid, '卫星', 3.0)))
    seg[sid] = (html, js, ['手机', '卫星×4', '规模标注'])
    sid = 'seg02'
    html = (f'<div class="b-el" id="b-sat" data-hf-name="卫星原子钟" data-layout-allow-occlusion="1" style="top:150px;left:830px;font-size:120px;">🛰</div>' +
            sp.disc(P, 'b-clock', CX, 190, 46, BUTTER) +
            sp.beam(P, 'b-sig', 940, 300, 20, 380, SKY, 0) +
            f'<div class="b-el" id="b-phone" data-hf-name="手机" data-layout-allow-occlusion="1" style="top:700px;left:855px;font-size:110px;">📱</div>' +
            sp.note(P, 'b-dt', 1230, 420, '时间差 → 距离', MINT, 2, 36))
    js = (pop('#b-sat', .3) + pop('#b-clock', cue(P, sid, '原子钟', 1.8)) + fade('#b-sig', cue(P, sid, '广播', 4.0)) +
          pop('#b-phone', cue(P, sid, '手机', 6.0)) + rise('#b-dt', cue(P, sid, '距离', 10.0)))
    seg[sid] = (html, js, ['卫星', '原子钟', '信号', '手机', '测距便签'])
    sid = 'seg03'
    rings = ''.join(sp.disc(P, f'c-r{i}', CX+[ -260, 260, -260, 260][i], CY+[-160, -160, 160, 160][i], 150, 'rgba(168,216,240,.5)').replace('box-shadow:6px 6px 0 #2D2D2D', 'opacity:.8') for i in range(4))
    html = (rings + sp.disc(P, 'c-you', CX, CY, 60, CORAL) + sp.label(P, 'c-youl', CX-30, CY+80, '你', fs=40) +
            sp.note_c(P, 'c-note', 760, '四球相交 · 位置唯一', MINT, -1, 38))
    js = (stagger_pop('[id^="c-r"]', 1.0, .4) + pop('#c-you', cue(P, sid, '唯一', 7.0)) + fade('#c-youl', cue(P, sid, '唯一', 7.0)) +
          rise('#c-note', cue(P, sid, '四', 10.0)))
    seg[sid] = (html, js, ['球面×4', '你', '你的标注', '定位便签'])
    sid = 'seg04'
    html = (f'<div class="d-el" id="d-clock" data-hf-name="时钟" data-layout-allow-occlusion="1" style="top:250px;left:830px;font-size:150px;">⏰</div>' +
            sp.note(P, 'd-us', 1300, 260, '卫星每天慢 38 微秒', BUTTER, 2, 34) +
            sp.big(P, 'd-km', 640, 520, '不校正 → 每天漂 10 公里', 56, '#B91C1C') +
            sp.label(P, 'd-rel', 780, 680, '相对论修正', fs=38))
    js = (pop('#d-clock', .4) + rise('#d-us', cue(P, sid, '微秒', 3.5)) + fade('#d-km', cue(P, sid, '公里', 7.0)) +
          fade('#d-rel', cue(P, sid, '相对论', 11.0)))
    seg[sid] = (html, js, ['时钟', '微秒便签', '漂移大字', '相对论标注'])
    sid = 'seg05'
    html = (f'<div class="e-el" id="e-ein" data-hf-name="爱因斯坦" data-layout-allow-occlusion="1" style="top:250px;left:760px;font-size:170px;">🧔</div>' +
            f'<div class="e-el" id="e-sat" data-hf-name="卫星" data-layout-allow-occlusion="1" style="top:250px;left:1080px;font-size:130px;">🛰</div>' +
            f'<div class="e-title e-el" id="e-slog" data-hf-name="收束标语" style="top:640px;font-size:76px;">{sp.title_chars("你在和爱因斯坦合作")}</div>')
    js = (pop('#e-ein', .4) + pop('#e-sat', 1.2) + chars_reveal('#e-slog', cue(P, sid, '合作', 3.5), .14))
    seg[sid] = (html, js, ['爱因斯坦', '卫星', '收束标语'])
    return seg


def _bread(P):
    seg = {}
    sid = 'seg01'
    html = (f'<div class="a-el" id="a-bowl" data-hf-name="面盆" style="top:430px;left:660px;width:600px;height:200px;border:4px solid {INK};border-radius:20px 20px 120px 120px;background:#fff;box-shadow:6px 6px 0 {INK};"></div>' +
            f'<div class="a-el" id="a-dough" data-hf-name="面团" style="top:440px;left:760px;width:400px;height:130px;background:{PEACH};border:3px solid {INK};border-radius:60px;"></div>' +
            sp.label(P, 'a-t', 790, 260, '先「醒」一两个小时', fs=44))
    js = (pop('#a-bowl', .4) + fade('#a-t', cue(P, sid, '醒', 2.0)) +
          f"gsap.set('#a-dough',{{scaleY:.6,transformOrigin:'center bottom'}});\n      tl.to('#a-dough',{{scaleY:1,duration:1.6,ease:'power1.inOut'}},{cue(P,sid,'发',4.0):.2f});")
    seg[sid] = (html, js, ['面盆', '面团', '醒面标注'])
    sid = 'seg02'
    html = (sp.disc(P, 'b-yeast', 500, 400, 90, BUTTER) + sp.label(P, 'b-yl', 420, 520, '酵母菌', fs=40) +
            f'<div class="b-el" id="b-sugar" data-hf-name="糖" data-layout-allow-occlusion="1" style="top:370px;left:790px;font-size:80px;">🍬</div>' +
            f'<div class="b-el" id="b-alc" data-hf-name="酒精" data-layout-allow-occlusion="1" style="top:280px;left:1130px;font-size:80px;">🍺</div>' +
            f'<div class="b-el" id="b-co2" data-hf-name="二氧化碳" data-layout-allow-occlusion="1" style="top:500px;left:1180px;font-size:80px;">🫧</div>' +
            sp.label(P, 'b-arrow', 930, 400, '→', fs=80))
    js = (pop('#b-yeast', .4) + fade('#b-yl', .8) + pop('#b-sugar', cue(P, sid, '糖', 2.5)) +
          pop('#b-alc', cue(P, sid, '酒精', 5.0)) + pop('#b-co2', cue(P, sid, '二氧化碳', 6.5)))
    seg[sid] = (html, js, ['酵母盘', '酵母标注', '糖', '酒精', '二氧化碳'])
    sid = 'seg03'
    import math as _m
    net = ''.join(f'<div class="c-el" data-n="{i}" data-hf-name="面筋网{i+1}" style="top:{260+(i//3)*150}px;left:{520+(i%3)*320}px;width:260px;height:110px;border:5px solid {TURQ};border-radius:50%;transform:rotate({(i*37)%50-25}deg);"></div>' for i in range(6))
    html = (net + sp.label(P, 'c-t', 700, 170, '面筋 = 弹性骨架', fs=44) +
            f'<div class="c-el" id="c-bub" data-hf-name="被锁住的气泡" data-layout-allow-occlusion="1" style="top:430px;left:890px;font-size:90px;">🫧</div>')
    js = (stagger_pop('[data-n]', 1.0, .25) + fade('#c-t', cue(P, sid, '面筋', 2.5)) + pop('#c-bub', cue(P, sid, '气泡', 7.0)))
    seg[sid] = (html, js, ['面筋网×6', '骨架标注', '气泡'])
    sid = 'seg04'
    html = (f'<div class="d-el" id="d-oven" data-hf-name="烤箱" data-layout-allow-occlusion="1" style="top:200px;left:790px;font-size:150px;">🔥</div>' +
            f'<div class="d-el" id="d-loaf" data-hf-name="膨胀定型的面包" style="top:450px;left:700px;width:500px;height:220px;background:#D9A066;border:4px solid {INK};border-radius:40px;box-shadow:6px 6px 0 {INK};"></div>' +
            sp.label(P, 'd-t', 700, 740, '六十度 · 酵母退场 · 组织定型', '#6B6B6B', 36))
    js = (pop('#d-oven', .4) + fade('#d-loaf', cue(P, sid, '烤箱', 2.0)) + fade('#d-t', cue(P, sid, '定型', 9.0)))
    seg[sid] = (html, js, ['烤箱', '面包', '定型标注'])
    sid = 'seg05'
    holes = ''.join(f'<div class="e-el" data-h="{i}" data-hf-name="孔洞{i+1}" style="top:{300+(i*53)%260}px;left:{560+(i*167)%800}px;width:{30+(i%4)*18}px;height:{30+(i%3)*16}px;background:{PAPER};border:3px solid {INK};border-radius:50%;"></div>' for i in range(10))
    html = (f'<div class="e-el" id="e-loaf" data-hf-name="掰开的面包" style="top:250px;left:460px;width:1000px;height:420px;background:#D9A066;border:4px solid {INK};border-radius:30px;box-shadow:6px 6px 0 {INK};"></div>' + holes +
            f'<div class="e-title e-el" id="e-slog" data-hf-name="收束标语" style="top:740px;font-size:66px;">{sp.title_chars("几百万个小气球")}</div>')
    js = (fade('#e-loaf', .3) + stagger_pop('[data-h]', 1.2, .12) + chars_reveal('#e-slog', cue(P, sid, '气球', 5.5), .14))
    seg[sid] = (html, js, ['面包', '孔洞×10', '收束标语'])
    return seg


def _aurora(P):
    seg = {}
    sid = 'seg01'
    curtains = ''.join(f'<div class="a-el" data-cu="{i}" data-hf-name="光帘{i+1}" style="top:120px;left:{300+i*260}px;width:140px;height:480px;background:linear-gradient({["#38BDF8,#7ECDC0"][0]},transparent);border-radius:70px;opacity:.75;transform:rotate({(i%3)*4-4}deg);"></div>' for i in range(5))
    html = (f'<div class="a-el" id="a-night" data-hf-name="夜空" style="inset:0;background:linear-gradient(#16223A 0%,#2A3A58 60%,{PAPER} 100%);"></div>' +
            curtains.replace('#38BDF8,#7ECDC0', '#4ADE80,#A78BFA') +
            f'<div class="a-title a-el" id="a-title" data-hf-name="标题" style="top:660px;font-size:92px;color:#fff;">{sp.title_chars("天空中的光之帘幕")}</div>')
    js = (fade('#a-night', .2, .5) + f"gsap.set('[data-cu]',{{opacity:0,y:-40}});\n      tl.to('[data-cu]',{{opacity:.75,y:0,duration:.8,stagger:.25,ease:'power2.out'}},.5);" +
          chars_reveal('#a-title', cue(P, sid, '摇', 5.0), .15))
    seg[sid] = (html, js, ['夜空', '光帘×5', '标题'])
    sid = 'seg02'
    parts = ''.join(f'<div class="b-el" data-p="{i}" data-hf-name="粒子{i+1}" style="top:{300+(i*61)%300}px;left:{700+(i*97)%700}px;width:22px;height:22px;background:{BUTTER};border:2px solid {INK};border-radius:50%;"></div>' for i in range(10))
    html = (f'<div class="b-el" id="b-sun" data-hf-name="太阳" data-layout-allow-occlusion="1" style="top:180px;left:280px;font-size:170px;">☀️</div>' + parts +
            f'<div class="b-el" id="b-earth" data-hf-name="地球" data-layout-allow-occlusion="1" style="top:330px;left:1450px;font-size:150px;">🌍</div>' +
            sp.label(P, 'b-wind', 700, 200, '太阳风 →', fs=42))
    js = (pop('#b-sun', .3) + fade('#b-wind', cue(P, sid, '太阳风', 2.0)) +
          f"gsap.set('[data-p]',{{opacity:0,scale:.3}});\n      tl.to('[data-p]',{{opacity:1,scale:1,duration:.3,stagger:.15,ease:'back.out(2)'}},{cue(P,sid,'粒子',3.5):.2f});" +
          pop('#b-earth', cue(P, sid, '地球', 8.0)))
    seg[sid] = (html, js, ['太阳', '太阳风标注', '粒子×10', '地球'])
    sid = 'seg03'
    html = (f'<div class="c-el" id="c-earth" data-hf-name="地球磁场" style="top:{CY-170}px;left:{CX-170}px;width:340px;height:340px;background:radial-gradient(#7ECDC0,#38BDF8);border:4px solid {INK};border-radius:50%;box-shadow:6px 6px 0 {INK};"></div>' +
            ''.join(f'<div class="c-el" data-f="{i}" data-hf-name="磁力线{i+1}" style="top:{CY-260+(i%2)*40}px;left:{CX-330+i*40}px;width:{(660-i*80)}px;height:{520-(i%2)*60}px;border:4px solid {CORAL};border-radius:50%;opacity:.6;"></div>' for i in range(4)) +
            sp.note(P, 'c-pole', 1380, 180, '滑向两极', SKY, 2, 38) +
            sp.label(P, 'c-shield', 300, 180, '磁场 = 盾牌', fs=42))
    js = (pop('#c-earth', .3) + f"gsap.set('[data-f]',{{opacity:0,scale:.7,transformOrigin:'50% 50%'}});\n      tl.to('[data-f]',{{opacity:.6,scale:1,duration:.6,stagger:.3,ease:'power2.out'}},{cue(P,sid,'磁场',2.5):.2f});" +
          fade('#c-shield', cue(P, sid, '盾', 2.5)) + rise('#c-pole', cue(P, sid, '极', 9.0)))
    seg[sid] = (html, js, ['地球', '磁力线×4', '盾牌标注', '两极便签'])
    sid = 'seg04'
    layers = ''.join(f'<div class="d-el" data-gl="{i}" data-hf-name="{n}光层" style="top:{200+i*150}px;left:{340+(i%2)*120}px;width:{900+(i%2)*300}px;height:100px;background:{c};border:3px solid {INK};border-radius:50px;opacity:0;"></div>'
                     for i, (n, c) in enumerate([('绿', '#4ADE80'), ('红', '#F87171'), ('紫粉', '#C084FC')]))
    html = (layers + sp.label(P, 'd-oxy', 1360, 240, '氧 → 绿/红', fs=38) +
            sp.label(P, 'd-nit', 1360, 490, '氮 → 紫粉', fs=38) +
            sp.note_c(P, 'd-note', 700, '被撞的原子会发光', MINT, -1, 38))
    js = (f"tl.to('[data-gl]',{{opacity:.85,duration:.6,stagger:.5,ease:'power2.out'}},{cue(P,sid,'绿',2.0):.2f});" +
          fade('#d-oxy', cue(P, sid, '氧', 4.5)) + fade('#d-nit', cue(P, sid, '氮', 7.0)) + rise('#d-note', cue(P, sid, '光', 10.0)))
    seg[sid] = (html, js, ['光层×3', '氧标注', '氮标注', '发光便签'])
    sid = 'seg05'
    html = (f'<div class="e-el" id="e-nn" data-hf-name="霓虹灯" style="top:280px;left:770px;font-size:180px;">霓</div>' +
            f'<div class="e-title e-el" id="e-slog" data-hf-name="收束标语" style="top:600px;font-size:78px;">{sp.title_chars("太阳供电 · 磁场导流 · 大气发光")}</div>')
    js = (pop('#e-nn', .4) + chars_reveal('#e-slog', cue(P, sid, '供电', 3.5), .12))
    seg[sid] = (html, js, ['霓虹', '收束标语'])
    return seg


def _muscle(P):
    seg = {}
    sid = 'seg01'
    html = (f'<div class="a-el" id="a-man" data-hf-name="酸痛的人" data-layout-allow-occlusion="1" style="top:280px;left:820px;font-size:200px;">😓</div>' +
            f'<div class="a-el" id="a-stairs" data-hf-name="楼梯" data-layout-allow-occlusion="1" style="top:560px;left:760px;font-size:120px;">🪜</div>' +
            sp.note_c(P, 'a-note', 720, '第二天酸到下不了楼', BUTTER, -1.5, 38))
    js = (pop('#a-man', .4) + fade('#a-stairs', 1.0) + rise('#a-note', cue(P, sid, '酸痛', 3.5)))
    seg[sid] = (html, js, ['酸痛的人', '楼梯', '酸痛便签'])
    sid = 'seg02'
    html = (f'<div class="b-el" id="b-myth" data-hf-name="乳酸堆积大字" data-layout-allow-occlusion="1" data-layout-allow-overlap="1" style="top:380px;left:0;right:0;text-align:center;font-size:110px;font-weight:700;">乳酸堆积</div>' +
            f'<div class="b-el" id="b-x" data-hf-name="红叉" data-layout-allow-overlap="1" style="top:300px;left:50%;margin-left:-160px;font-size:300px;color:{CORAL};opacity:.85;">✕</div>')
    js = (fade('#b-myth', .3) + f"gsap.set('#b-x',{{opacity:0,scale:2.2,transformOrigin:'50% 50%'}});\n      tl.to('#b-x',{{opacity:.85,scale:1,duration:.35,ease:'power3.in'}},{cue(P,sid,'乳酸',2.5):.2f});")
    seg[sid] = (html, js, ['误区大字', '红叉'])
    sid = 'seg03'
    dots = ''.join(f'<div class="c-el" data-l="{i}" data-hf-name="乳酸分子{i+1}" style="top:{330+(i*67)%240}px;left:{520+(i*137)%880}px;width:26px;height:26px;background:{PINK};border:2px solid {INK};border-radius:50%;"></div>' for i in range(10))
    html = (dots + sp.label(P, 'c-t', 700, 200, '运动后 1–2 小时内', fs=44) +
            sp.note_c(P, 'c-note', 700, '乳酸已被代谢干净', MINT, -1, 38))
    js = (stagger_pop('[data-l]', .8, .1) + fade('#c-t', .3) + f"tl.to('[data-l]',{{opacity:.12,duration:1.2,stagger:.06,ease:'none'}},{cue(P,sid,'代谢',4.5):.2f});" + rise('#c-note', cue(P, sid, '干净', 7.0)))
    seg[sid] = (html, js, ['乳酸×10', '时间标注', '代谢便签'])
    sid = 'seg04'
    cracks = ''.join(sp.beam(P, f'd-cr{i}', 640+i*200, 400+(i%2)*60, 90, 8, CORAL, 20) for i in range(3))
    html = (sp.beam(P, 'd-fiber', 500, 430, 900, 46, PEACH, 0) + cracks +
            sp.label(P, 'd-t', 700, 240, '肌纤维的细微损伤', fs=44) +
            sp.note(P, 'd-real', 1330, 560, '真正的元凶', BUTTER, 2, 36))
    js = (draw_x('#d-fiber', .5, .7) + fade('#d-t', .3) + stagger_pop('[id^="d-cr"]', cue(P, sid, '损伤', 4.0), .3) +
          rise('#d-real', cue(P, sid, '延迟', 8.0)))
    seg[sid] = (html, js, ['肌纤维', '裂痕×3', '损伤标注', '元凶便签'])
    sid = 'seg05'
    html = (f'<div class="e-el" id="e-run" data-hf-name="下坡跑" data-layout-allow-occlusion="1" style="top:300px;left:480px;font-size:130px;">🏃</div>' +
            f'<div class="e-el" id="e-dumb" data-hf-name="缓慢放哑铃" data-layout-allow-occlusion="1" style="top:300px;left:1250px;font-size:130px;">🏋</div>' +
            sp.label(P, 'e-l', 400, 500, '下坡跑', fs=40) + sp.label(P, 'e-r', 1200, 500, '缓慢放哑铃', fs=40) +
            sp.note_c(P, 'e-note', 680, '拉长状态下发力 · 损伤最大', SKY, -1, 38))
    js = (pop('#e-run', .4) + pop('#e-dumb', 1.2) + fade('#e-l', .8) + fade('#e-r', 1.5) + rise('#e-note', cue(P, sid, '离心', 5.0)))
    seg[sid] = (html, js, ['下坡跑', '哑铃', '标注×2', '离心便签'])
    sid = 'seg06'
    arrs, arrsel = sp.radial_arrows(P, CX, CY, 8, 220, 150, MINT, start_deg=22)
    html = (sp.disc(P, 'f-imm', CX, CY, 110, MINT) + sp.label(P, 'f-il', CX-96, CY-22, '免疫细胞', fs=40) +
            arrs + sp.note(P, 'f-pain', 1330, 210, '痛觉神经被激活', PEACH, 2, 36))
    js = (pop('#f-imm', .4) + fade('#f-il', .9) + stagger_pop(arrsel, cue(P, sid, '炎症', 3.0), .15) +
          rise('#f-pain', cue(P, sid, '痛', 8.5)))
    seg[sid] = (html, js, ['免疫盘', '免疫标注', '聚集箭头×8', '痛觉便签'])
    sid = 'seg07'
    html = (sp.beam(P, 'f-thin', 380, 460, 460, 40, PEACH, 0) + sp.label(P, 'g-l1', 480, 555, '修复前', fs=36) +
            f'<div class="g-el" id="g-arrow" data-hf-name="升级箭头" style="top:430px;left:920px;font-size:90px;">→</div>' +
            sp.beam(P, 'g-thick', 1090, 440, 520, 76, MINT, 0) + sp.label(P, 'g-l2', 1250, 570, '修复后更粗', fs=36))
    js = (fade('#f-thin', .3) + fade('#g-l1', .6) + fade('#g-arrow', cue(P, sid, '修复', 4.0)) +
          f"gsap.set('#g-thick',{{scaleX:0,transformOrigin:'left center'}});\n      tl.to('#g-thick',{{scaleX:1,duration:.6,ease:'power2.out'}},{cue(P,sid,'粗壮',5.5):.2f});" + fade('#g-l2', 7.0))
    seg[sid] = (html, js, ['细纤维', '修复前标注', '升级箭头', '粗纤维', '修复后标注'])
    sid = 'seg08'
    html = (f'<div class="h-el" id="h-up" data-hf-name="升级箭头" style="top:260px;left:880px;font-size:170px;color:#16A34A;">⬆</div>' +
            f'<div class="h-title h-el" id="h-slog" data-hf-name="升级标语" style="top:560px;font-size:96px;">{sp.title_chars("酸痛 = 升级信号")}</div>')
    js = (pop('#h-up', .4) + chars_reveal('#h-slog', cue(P, sid, '信号', 3.0), .16))
    seg[sid] = (html, js, ['升级箭头', '升级标语'])
    sid = 'seg09'
    icons = ''.join(f'<div class="i-el" data-i="{i}" data-hf-name="应对{i+1}" data-layout-allow-occlusion="1" style="top:330px;left:{380+i*400}px;font-size:130px;text-align:center;width:220px;">{ic}<div style="font-size:36px;font-weight:700;margin-top:6px;">{t}</div></div>' for i, (ic, t) in enumerate([('🚶', '轻度活动'), ('😴', '睡眠'), ('🥚', '蛋白质')]))
    html = (icons + f'<div class="i-title i-el" id="i-slog" data-hf-name="收束标语" style="top:680px;font-size:72px;">{sp.title_chars("给修复供料，让升级加速")}</div>')
    js = (stagger_pop('[data-i]', 1.0, .5) + chars_reveal('#i-slog', cue(P, sid, '加速', 6.0), .13))
    seg[sid] = (html, js, ['应对×3', '收束标语'])
    return seg


BUILDERS = {
    'cat-land': _catland, 'fridge': _fridge, 'vaccine': _vaccine, 'star-twinkle': _startwinkle,
    'cipher': _cipher, 'beehive': _beehive, 'gps': _gps, 'bread': _bread, 'aurora': _aurora, 'muscle': _muscle,
}

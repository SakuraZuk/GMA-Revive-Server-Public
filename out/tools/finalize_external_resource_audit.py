# -*- coding: utf-8 -*-
"""补充散装清单完整性、脚本差异、音库结构和线上只读证据。"""
import collections
import hashlib
import json
import struct
import sys
from pathlib import Path

from audit_external_resources import ROOT, SOURCE, OUT, digest, save, scan
from npk_unpack import parse_archive, decode_entry
from npk_script_decode import text_value
from mem_marshal_extract import Loader
from rotor_compat import Rotor
import zlib


def decode_code(raw):
    binding = json.loads((ROOT / 'out/npk_decoded/script_loader_bindings.json').read_text(encoding='utf-8'))
    if raw[:1] != b'c':
        inflated = zlib.decompress(Rotor(binding['rotor_key'].encode('ascii')).decrypt(raw))
        raw = bytes(b ^ 233 if i < 99 else b for i, b in enumerate(inflated))[1:-1]
    loader = Loader(raw)
    code = loader.r_object()
    if loader.p != len(raw) or not isinstance(code, dict) or code.get('type') != 'code':
        raise ValueError('脚本对象未完整消费')
    return code


def main():
    sys.stdout.reconfigure(encoding='utf-8')
    report = json.loads((OUT / 'report.json').read_text(encoding='utf-8'))
    package_res = SOURCE.parent / 'res'
    loose = []
    for folder in [package_res, SOURCE / 'res']:
        for path in sorted(folder.rglob('*')):
            if not path.is_file():
                continue
            md5, sha = digest(path)
            loose.append({'文件': str(path.relative_to(ROOT)), '大小': path.stat().st_size, 'MD5': md5, 'SHA256': sha})
    save(OUT / 'all-loose-files.json', loose)
    lists = []
    for folder in [package_res, SOURCE / 'res']:
        path = folder / 'res_discrete.lst'
        errors, rows = [], []
        for line in path.read_text(encoding='utf-8').splitlines():
            fields = line.split()
            if len(fields) < 3:
                errors.append({'清单行': line, '错误': '不足三个字段'})
                continue
            name, size, md5 = fields[:3]
            target = folder / name
            if not target.is_file():
                errors.append({'文件': name, '错误': '缺少文件'})
                continue
            actual_md5, _ = digest(target)
            if int(size) != target.stat().st_size or actual_md5 != md5:
                errors.append({'文件': name, '错误': '大小或MD5不一致', '实际MD5': actual_md5, '清单MD5': md5})
            rows.append(name)
        lists.append({'清单': str(path.relative_to(ROOT)), '条目数': len(path.read_text(encoding='utf-8').splitlines()),
                      '存在数': len(rows), '错误数': len(errors), '错误': errors})
    report['散装资源全目录'] = {'数量': len(loose), '总大小': sum(p['大小'] for p in loose),
        '扩展名统计': dict(collections.Counter(Path(p['文件']).suffix for p in loose)), '清单核验': lists}
    # 散装音频只验RIFF边界；不能用文件个数宣称全部语音。
    wem_errors = []
    for path in sorted(package_res.rglob('*.wem')):
        raw = path.read_bytes()
        if len(raw) < 12 or raw[:4] != b'RIFF' or raw[8:12] != b'WAVE' or struct.unpack_from('<I', raw, 4)[0] + 8 != len(raw):
            wem_errors.append(str(path.relative_to(ROOT)))
    report['散装WEM'] = {'数量': len(list(package_res.rglob('*.wem'))), 'RIFF结构错误': wem_errors,
                       '目录数量': dict(collections.Counter(str(p.parent.relative_to(package_res)) for p in package_res.rglob('*.wem')))}
    # 将外部完整脚本与APK基础、现行官方覆盖层逐条比较。
    base_path = ROOT / 'work/apk/assets/script.npk'
    summary, base = scan(base_path)
    external = json.loads((OUT / 'script-entries.json').read_text(encoding='utf-8'))
    patched = json.loads((OUT / 'script-baseline-entries.json').read_text(encoding='utf-8'))
    changed = [k for k in external.keys() & base.keys() if external[k]['内容SHA256'] != base[k]['内容SHA256']]
    entries, raw = parse_archive(SOURCE / 'script.npk')
    code_rows = []
    route_rows = []
    for entry in entries:
        if str(entry['hash']) not in changed:
            continue
        code = decode_code(decode_entry(entry, raw))
        row = {'ID': str(entry['hash']), '模块': text_value(code['filename']), '与官方覆盖层一致': str(entry['hash']) in patched and external[str(entry['hash'])]['内容SHA256'] == patched[str(entry['hash'])]['内容SHA256']}
        routes = []
        def walk_routes(current):
            for value in current['consts']:
                if isinstance(value, dict) and value.get('type') == 'code':
                    walk_routes(value)
                elif isinstance(value, (str, bytes)):
                    try:
                        value = text_value(value)
                    except UnicodeDecodeError:
                        continue
                    if any(word in value for word in ['http://', 'https://', '127.0.0.1', 'localhost']):
                        routes.append(value)
        if not row['与官方覆盖层一致']:
            walk_routes(code)
            route_rows.append({'模块': row['模块'], '连接常量': routes})
        if row['模块'].replace('\\', '/').endswith('patch_logic/version.py'):
            row['常量'] = [text_value(v) for v in code['consts'] if isinstance(v, (str, bytes, int))]
        if row['模块'].replace('\\', '/').endswith('patch_logic/patch_utils.py'):
            for child in code['consts']:
                if isinstance(child, dict) and child.get('type') == 'code' and text_value(child['name']) == 'verify_patch':
                    row['资源校验方法'] = {'字节码': child['bytecode'].hex(), '引用名': [text_value(v) for v in child['names']],
                        '常量': [text_value(v) for v in child['consts'] if isinstance(v, (str, bytes, int))]}
        code_rows.append(row)
    save(OUT / 'script-route-constants.json', route_rows)
    report['脚本核验'] = {'与APK相比变更数': len(changed), '变更模块': code_rows, '基线SHA256': summary['SHA256'],
        '说明': '完整合并包与现行65KB覆盖包不是同一发布文件，不能直接替换现行清单'}
    # 保存线上真实清单、资源哈希和当前运行状态，不记录任何口令。
    from deploy_hotfix_bridge import connect, run, audit
    client = connect()
    try:
        remote = '/opt/hs-res/patch_pub.android_1.0.128a588'
        state = audit(client)
        state['资源目录'] = run(client, 'readlink -f /opt/hs-res')
        state['远端包MD5'] = run(client, 'md5sum ' + remote + '/*.npk')
        state['远端清单SHA256'] = run(client, 'sha256sum /opt/hs-hotfix/data/patch_list_pub_android.txt /opt/hs-hotfix/data/hotfix.json')
        with client.open_sftp() as sftp:
            body = sftp.open('/opt/hs-hotfix/data/patch_list_pub_android.txt', 'rb').read()
            (OUT / 'remote-patch-list.json').write_bytes(body)
        state['远端清单与本地一致'] = body == (ROOT / 'deploy/data/patch_list_pub_android.txt').read_bytes()
        save(OUT / 'remote-audit.json', state)
        report['线上核验'] = {'清单与本地一致': state['远端清单与本地一致'], '资源目录': state['资源目录'], '证据': str((OUT / 'remote-audit.json').relative_to(ROOT))}
    finally:
        client.close()
    report['汇总'] = {'NPK数量': len(report['包']), '总大小': sum(p['大小'] for p in report['包']),
        '解压条目总数': sum(p['解压通过'] for p in report['包']), '解压错误总数': sum(p['错误数'] for p in report['包']),
        '与现行清单一致包数': sum(p['现行清单一致'] for p in report['包'])}
    voice_rows = [p for p in loose if p['文件'].endswith('.wem')]
    report['散装WEM']['不同内容数量'] = len({p['SHA256'] for p in voice_rows})
    report['发布判定'] = {
        '状态': '整目录不满足直接发布条件，本轮未修改生产分发',
        '已在线资源': ['char1', 'char2', 'char3', 'char4', 'effect', 'scenewd1', 'uiicon', 'wwise', 'wwisech', 'wwisejp'],
        '待专项接入': ['char5', 'char6增量170项', 'ui增量109项', 'reslow四包', '散装视频和音频'],
        '阻断项': ['脚本含7个非官方基础模块改动，旧内网更新地址见script-route-constants.json',
                   'res.npk比现行基础包少2项', '场景包已有条目内容发生变化，需逐项核对',
                   'Documents散装着色器缓存679项与所附MD5清单不一致'],
        '现有客户端限制': '客户端与清单同为1.0.128时no_patch=True，单纯增加清单文件不能证明已安装玩家收到新资源',
        '验收边界': '未操作MuMu、未清除玩家存档、未宣称全部语音或全场景资源齐全'}
    save(OUT / 'report.json', report)
    brief = {k: report[k] for k in ['汇总', '散装WEM', '脚本核验', '线上核验']}
    brief['散装清单'] = [{k:v for k,v in item.items() if k != '错误'} for item in lists]
    print(json.dumps(brief, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()

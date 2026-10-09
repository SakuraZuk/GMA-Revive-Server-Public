# -*- coding: utf-8 -*-
"""只读核验外部资源：原生索引、全量解压、散装文件和现行热更差异。"""
import collections
import hashlib
import json
import mmap
import struct
import sys
import zlib
from pathlib import Path

import lz4.block

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'com.netease.hsqsl/files/netease/h62/Documents'
OUT = ROOT / 'out/external-resource-audit-20261009'


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def digest(path):
    md5, sha = hashlib.md5(), hashlib.sha256()
    with path.open('rb') as fp:
        while block := fp.read(4 * 1024 * 1024):
            md5.update(block)
            sha.update(block)
    return md5.hexdigest(), sha.hexdigest()


def scan(path):
    md5, sha = digest(path)
    rows, errors, spans = {}, [], []
    codecs, types = collections.Counter(), collections.Counter()
    with path.open('rb') as fp, mmap.mmap(fp.fileno(), 0, access=mmap.ACCESS_READ) as data:
        if data[:4] != b'NXPK' or len(data) < 24:
            raise ValueError('不是完整NXPK：' + str(path))
        count = struct.unpack_from('<I', data, 4)[0]
        index = struct.unpack_from('<I', data, 20)[0]
        end = index + count * 28
        if index < 24 or end > len(data):
            raise ValueError('索引越界：' + str(path))
        for i in range(count):
            key, offset, stored, decoded, hash64, codec = struct.unpack_from('<IIIIQI', data, index + i * 28)
            if codec >= 62:
                offset ^= ((-100 - decoded) & 0xffffffff) ^ 0x7a090d89
                codec -= 62
            fid = str(key)
            try:
                if fid in rows or offset < 24 or offset + stored > len(data) or (offset < end and offset + stored > index):
                    raise ValueError('重复条目或存储越界')
                if decoded > 256 * 1024 * 1024:
                    raise ValueError('解压长度超出核验上限')
                raw = data[offset:offset + stored]
                content = (lz4.block.decompress(raw, uncompressed_size=decoded) if codec == 2 else
                           zlib.decompress(raw) if codec == 1 else raw if codec == 0 else None)
                if content is None or len(content) != decoded:
                    raise ValueError('未知编码或解压长度错误')
                prefix = content[:12]
                kind = ('DDS' if prefix.startswith(b'DDS ') else 'KTX' if prefix.startswith(b'\xabKTX') else
                        'RIFF' if prefix.startswith(b'RIFF') else 'BKHD' if prefix.startswith(b'BKHD') else
                        'PNG' if prefix.startswith(b'\x89PNG') else '其他')
                types[kind] += 1
                codecs[str(codec)] += 1
                rows[fid] = {'存储MD5': hashlib.md5(raw).hexdigest(), '内容SHA256': hashlib.sha256(content).hexdigest(),
                             '解压长度': decoded, '类型': kind, '前缀': prefix.hex()}
                spans.append((offset, offset + stored))
            except Exception as exc:
                errors.append({'条目': fid, '错误': str(exc)})
        spans.sort()
        overlaps = sum(a[1] > b[0] for a, b in zip(spans, spans[1:]))
        summary = {'文件': str(path.relative_to(ROOT)), '大小': len(data), 'MD5': md5, 'SHA256': sha,
                   '条目数': count, '解压通过': len(rows), '错误数': len(errors), '重叠数': overlaps,
                   '头部': data[:24].hex(), '压缩类型': dict(codecs), '内容类型': dict(types), '错误': errors[:30]}
    return summary, rows


def main():
    sys.stdout.reconfigure(encoding='utf-8')
    OUT.mkdir(exist_ok=True)
    manifest = json.loads((ROOT / 'deploy/data/patch_list_manifest.json').read_text(encoding='utf-8'))['android']['npk']
    report = {'来源': str(SOURCE), '包': [], '散装资源': {}, '边界': '全量静态解压与版本差异；不代表所有资源齐全或Android验收'}
    for path in sorted(SOURCE.glob('*.npk')):
        summary, rows = scan(path)
        save(OUT / (path.stem + '-entries.json'), rows)
        name = path.stem
        baseline = ROOT / ('files/netease/h62/Documents/' + path.name)
        if name in {'res', 'char1', 'ui', 'wwise', 'scene', 'scenewd2'}:
            baseline = ROOT / ('out/repack/_apk_extract/' + path.name)
        if name in manifest:
            fields = manifest[name].split()
            summary['现行清单一致'] = int(fields[0]) == summary['大小'] and fields[1] == summary['MD5']
            summary['现行大小'] = int(fields[0])
            summary['现行MD5'] = fields[1]
        else:
            summary['现行清单一致'] = False
            summary['现行清单缺此包'] = True
        if baseline.exists() and not summary.get('现行清单一致'):
            old, oldrows = scan(baseline)
            save(OUT / (name + '-baseline-entries.json'), oldrows)
            shared = rows.keys() & oldrows.keys()
            summary['与基线差异'] = {'基线': old['文件'], '基线SHA256': old['SHA256'], '共有': len(shared),
                '内容相同': sum(rows[k]['内容SHA256'] == oldrows[k]['内容SHA256'] for k in shared),
                '内容变更': sum(rows[k]['内容SHA256'] != oldrows[k]['内容SHA256'] for k in shared),
                '新增': len(rows.keys() - oldrows.keys()), '缺少': len(oldrows.keys() - rows.keys()),
                '缺少ID': sorted(oldrows.keys() - rows.keys()), '新增ID': sorted(rows.keys() - oldrows.keys())}
        report['包'].append(summary)
        save(OUT / 'report.json', report)
        print(json.dumps(summary, ensure_ascii=False), flush=True)
    loose = []
    for path in sorted((SOURCE / 'res').rglob('*')):
        if path.is_file():
            md5, sha = digest(path)
            loose.append({'文件': str(path.relative_to(SOURCE)), '大小': path.stat().st_size, 'MD5': md5, 'SHA256': sha})
    save(OUT / 'loose-files.json', loose)
    report['散装资源'] = {'数量': len(loose), '总大小': sum(p['大小'] for p in loose),
                         '扩展名统计': dict(collections.Counter(Path(p['文件']).suffix for p in loose))}
    report['语音结论'] = '以wwise/wwisech/wwisejp全包校验为准，RIFF和BKHD计数只能证明存在音频与音库，不能证明全部剧情覆盖'
    save(OUT / 'report.json', report)
    print(json.dumps(report['散装资源'], ensure_ascii=False))


if __name__ == '__main__':
    main()

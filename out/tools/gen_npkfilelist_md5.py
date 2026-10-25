# -*- coding: utf-8 -*-
"""按清单整包哈希选择资源，生成完整逐文件MD5表；任何解析失败都中止。

NXPK索引为28字节；末字段>=62时，实际偏移须按已有原生资源审计规则还原。
只生成元数据，不修改NPK，不上传；客户端差量偏移兼容与发布另行验证。
必须显式指定输出，避免把未验证候选直接写进正式分发目录。
"""
import argparse
import hashlib
import json
import struct
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOCS = ROOT / 'files/netease/h62/Documents'
APK = ROOT / 'out/repack/hs-direct-agree.apk'
CACHE = ROOT / 'out/repack/_apk_extract'


def matching_source(name, size, expected):
    if not name or not all(c.isalnum() or c in '_-' for c in name):
        raise ValueError('资源名称无效')
    for path in (DOCS / (name + '.npk'), CACHE / (name + '.npk')):
        if path.is_file() and path.stat().st_size == size:
            data = path.read_bytes()
            if hashlib.md5(data).hexdigest() == expected:
                return data, str(path.relative_to(ROOT))
    if APK.is_file():
        with zipfile.ZipFile(APK) as archive:
            entry = 'assets/' + name + '.npk'
            if entry in archive.namelist() and archive.getinfo(entry).file_size == size:
                data = archive.read(entry)
                if hashlib.md5(data).hexdigest() == expected:
                    return data, 'APK/assets/' + name + '.npk'
    raise ValueError('没有与清单大小及MD5一致的资源：' + name)


def block_md5s(data):
    if len(data) < 24 or data[:4] != b'NXPK':
        raise ValueError('不是完整NXPK')
    count = struct.unpack_from('<I', data, 4)[0]
    index = struct.unpack_from('<I', data, 20)[0]
    index_end = index + count * 28
    if index < 24 or index_end > len(data):
        raise ValueError('索引越界')
    seen, spans, pairs = set(), [], []
    encoded, empty = 0, 0
    for i in range(count):
        fid, offset, size, decoded, h1, h2, codec = struct.unpack_from('<IIIIIII', data, index + i * 28)
        if fid in seen:
            raise ValueError('重复文件编号')
        seen.add(fid)
        if codec >= 62:
            offset ^= ((-100 - decoded) & 0xffffffff) ^ 0x7a090d89
            codec -= 62
            encoded += 1
        if codec not in (0, 1, 2):
            raise ValueError('未知编码类型')
        if not size:
            empty += 1
            continue
        end = offset + size
        if offset < 24 or end > len(data) or (offset < index_end and end > index):
            raise ValueError('存储块越界或与索引重叠')
        spans.append((offset, end))
        pairs.extend((fid, hashlib.md5(memoryview(data)[offset:end]).hexdigest()))
    spans.sort()
    if any(a[1] > b[0] for a, b in zip(spans, spans[1:])):
        raise ValueError('存储块互相重叠')
    return pairs, {'条目数': count, '校验条目数': len(pairs) // 2, '编码偏移条目': encoded, '空条目': empty}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, default=ROOT / 'deploy/data/patch_list_pub_android.txt')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text(encoding='utf-8'))['android']['npk']
    table, checks = {}, []
    if not manifest:
        raise ValueError('资源清单为空')
    for name, value in manifest.items():
        words = value.split()
        size, expected = int(words[0]), words[1]
        data, source = matching_source(name, size, expected)
        pairs, check = block_md5s(data)
        table[name] = pairs
        check.update({'资源': name, '源': source, '整包MD5': expected, '大小': size})
        checks.append(check)
        print(json.dumps(check, ensure_ascii=False), flush=True)
    body = json.dumps(table, separators=(',', ':')).encode('utf-8')
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(args.output.name + '.tmp')
    temporary.write_bytes(body)
    temporary.replace(args.output)
    report = {'资源数': len(table), '总校验条目': sum(len(v)//2 for v in table.values()),
              '字节': len(body), 'MD5': hashlib.md5(body).hexdigest(), '资源核验': checks,
              '边界': '全部资源匹配输入清单；仅生成候选，未发布，未证明Android差量下载兼容'}
    args.output.with_suffix('.report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print(json.dumps({k:v for k,v in report.items() if k != '资源核验'}, ensure_ascii=False))


if __name__ == '__main__':
    sys.stdout.reconfigure(encoding='utf-8')
    main()

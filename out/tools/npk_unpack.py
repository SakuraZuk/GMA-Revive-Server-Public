# -*- coding: utf-8 -*-
"""NXPK 严格解包器：原始存储块按标准 LZ4 解压，不执行取反。

索引 +8=存储长度、+12=解压长度，证据是 PC VA 0xbf69a3 调用及
Unicorn VA 0x1309310 实际返回值。输出统一使用 .bin；19 73 头的
脚本负载尚未证实为 marshal，禁止用 .marshal 后缀暗示已解密。
运行：python out/tools/npk_unpack.py 输入.npk 输出目录 [条目上限]
"""
import hashlib
import json
import struct
import sys
import zlib
from pathlib import Path


class DecodeError(ValueError):
    """损坏或尚未支持的数据必须显式失败。"""


def parse_archive(path):
    data = Path(path).read_bytes()
    if len(data) < 24 or data[:4] != b'NXPK':
        raise DecodeError('不是完整的 NXPK 文件')
    count = struct.unpack_from('<I', data, 4)[0]
    index = struct.unpack_from('<I', data, 20)[0]
    index_end = index + count * 28
    if index < 24 or index_end > len(data):
        raise DecodeError('索引区越界')
    entries = []
    seen = set()
    for i in range(count):
        key, offset, stored, decoded, hash64, codec = struct.unpack_from(
            '<IIIIQI', data, index + i * 28)
        end = offset + stored
        overlaps_index = offset < index_end and end > index
        if key in seen or offset < 24 or end > len(data) or overlaps_index:
            raise DecodeError('条目哈希重复或存储区越界：%08X' % key)
        seen.add(key)
        entries.append(dict(hash=key, offset=offset, stored_size=stored,
                            decoded_size=decoded, hash64=hash64, codec=codec,
                            after_index=offset >= index_end))
    return entries, data


def lz4_decompress(src, expected):
    """带严格读写边界的标准 LZ4 block 解码，支持重叠回溯复制。"""
    out = bytearray()
    i = 0

    def length(initial):
        nonlocal i
        value = initial
        if initial == 15:
            while True:
                if i >= len(src):
                    raise DecodeError('长度扩展被截断')
                b = src[i]
                i += 1
                value += b
                if b != 255:
                    break
        return value

    if expected < 0 or expected > 64 * 1024 * 1024:
        raise DecodeError('目标长度超过工具限制')
    while i < len(src):
        token = src[i]
        i += 1
        literal = length(token >> 4)
        if i + literal > len(src) or len(out) + literal > expected:
            raise DecodeError('字面量读取或输出越界')
        out.extend(src[i:i+literal])
        i += literal
        if i == len(src):
            break
        if i + 2 > len(src):
            raise DecodeError('回溯偏移被截断')
        offset = int.from_bytes(src[i:i+2], 'little')
        i += 2
        if offset == 0 or offset > len(out):
            raise DecodeError('非法回溯偏移')
        match = length(token & 15) + 4
        if len(out) + match > expected:
            raise DecodeError('回溯输出越界')
        for _ in range(match):
            out.append(out[-offset])
    if len(out) != expected:
        raise DecodeError('输出长度不匹配：%d != %d' % (len(out), expected))
    return bytes(out)


def decode_entry(entry, data):
    start = entry['offset']
    raw = data[start:start+entry['stored_size']]
    codec = entry['codec']
    if codec == 2:
        result = lz4_decompress(raw, entry['decoded_size'])
    elif codec == 1:
        result = zlib.decompress(raw)
        if len(result) != entry['decoded_size']:
            raise DecodeError('zlib 输出长度不匹配')
    else:
        raise DecodeError('尚未验证的压缩类型：%d' % codec)
    return result


def main(path, outdir, limit=None):
    entries, data = parse_archive(path)
    outdir = Path(outdir)
    outdir.mkdir(parents=True, exist_ok=True)
    selected = entries if limit is None else entries[:limit]
    rows = []
    for entry in selected:
        row = dict(entry)
        row['hash'] = '%08X' % entry['hash']
        try:
            result = decode_entry(entry, data)
            name = row['hash'] + '.bin'
            (outdir/name).write_bytes(result)
            row.update(status='已解压', file=name, sha256=hashlib.sha256(result).hexdigest(),
                       prefix=result[:16].hex(), payload='未识别')
        except (ValueError, zlib.error) as exc:
            row.update(status='失败', error=str(exc))
        rows.append(row)
    report = dict(source=str(Path(path).resolve()), archive_sha256=hashlib.sha256(data).hexdigest(),
                  total=len(entries), tested=len(rows),
                  blocks_after_index=sum(e['after_index'] for e in entries),
                  success=sum(r['status']=='已解压' for r in rows), entries=rows)
    (outdir/'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print('%s：总数=%d，本次=%d，成功=%d，失败=%d' % (
        path, len(entries), len(rows), report['success'], len(rows)-report['success']))
    for row in rows:
        if row['status'] == '失败':
            print('  %s：%s' % (row['hash'], row['error']))
    return report


if __name__ == '__main__':
    result = main(sys.argv[1], sys.argv[2], int(sys.argv[3]) if len(sys.argv)>3 else None)
    sys.exit(0 if result['success']==result['tested'] else 1)

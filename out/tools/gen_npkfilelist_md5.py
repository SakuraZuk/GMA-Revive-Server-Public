# -*- coding: utf-8 -*-
"""gen_npkfilelist_md5.py — 生成客户端热更所需的真实 per-file md5 表。

背景（2026-10-06 实机 KeyError 384809306 定位）：
  客户端 patch_logic/patcher.py npk_patcher._check_file 对"需要下载"的每个文件条目
  执行 self.npkfilelist_md5info[fileid]，该字典来自 patch_mgr._get_npkmd5_file 下载的
  <patchpath>/npk_filelist_md5.txt（JSON: {npk名: [fileid, md5, fileid, md5, ...]}，
  set_npkfilelist_md5_info 按偶数索引取 fileid、奇数索引取 md5）。
  空 {} 在"存在本地 npk 且需逐文件 diff"时会 KeyError；全量下载路径不查表。

NPK 文件结构（NXPK v2，实测）:
  头部 24 字节: 'NXPK' + uint32 count@4 + 8 字节保留 + uint32 ???@16 + uint32 index_offset@20
  数据区: [24, index_offset)
  索引:   [index_offset, index_offset + count*28)，条目 = struct '<IIIIIII'
          (fileid, offset, compress_size, uncompress_size, compress_hash, uncompress_hash, reserved)
  条目块 md5 = md5(文件[offset : offset+compress_size])（= 客户端下载并校验的字节）

数据源优先级：files/.../Documents/<name>.npk（官方/服务端同源），否则 APK assets。
产物: deploy/data/npk_filelist_md5.txt（供 deploy/gen_patchlist.py 使用与上传）。
"""
import hashlib
import json
import struct
import sys
import zipfile
from pathlib import Path

ROOT = Path(r'.')
DOCS = ROOT / 'files/netease/h62/Documents'
APK = ROOT / 'out/repack/hs-direct-agree.apk'
CACHE = ROOT / 'out/repack/_apk_extract'
OUT = ROOT / 'deploy/data/npk_filelist_md5.txt'

NPKS = ['char1', 'char2', 'char3', 'char4', 'char6', 'effect', 'res', 'scene',
        'scenewd1', 'scenewd2', 'ui', 'uiicon', 'wwise', 'wwisech', 'wwisejp', 'script']

# script 用官方 1.0.128 补丁包（与服务器 /patch_pub.../script.npk 一致）
FORCE_DOCS = {'script'}


def npk_path(name):
    p = DOCS / (name + '.npk')
    if name in FORCE_DOCS or p.exists():
        return p
    cached = CACHE / (name + '.npk')
    if not cached.exists():
        with zipfile.ZipFile(APK) as z:
            data = z.read('assets/%s.npk' % name)
        CACHE.mkdir(parents=True, exist_ok=True)
        cached.write_bytes(data)
    return cached


def parse_npk(path):
    data = path.read_bytes()
    if data[:4] != b'NXPK':
        raise ValueError('%s 非 NXPK 格式' % path)
    count = struct.unpack('<I', data[4:8])[0]
    index_offset = struct.unpack('<I', data[20:24])[0]
    if index_offset + count * 28 > len(data):
        raise ValueError('%s 索引越界' % path)
    entries = []
    for i in range(count):
        e = struct.unpack('<IIIIIII', data[index_offset + i * 28:index_offset + (i + 1) * 28])
        entries.append(e)
    return data, entries


def main():
    sys.stdout.reconfigure(encoding='utf-8')
    table = {}
    for name in NPKS:
        path = npk_path(name)
        data, entries = parse_npk(path)
        pairs = []
        skipped = 0
        bad = 0
        for fid, offset, csize, usize, chash, uhash, resv in entries:
            if csize == 0:
                skipped += 1
                continue
            if offset < 24 or offset + csize > len(data):
                bad += 1
                continue
            pairs.append(fid)
            pairs.append(hashlib.md5(data[offset:offset + csize]).hexdigest())
        if bad:
            # 索引条目布局与本解析不符（大 npk 索引为引擎专用布局）。
            # 这些 npk 与本地逐字节相同，客户端比对全部命中、不会触发下载，
            # 也就不会查 md5 表；仅当未来真的出现逐文件 diff 时才需要再逆向。
            print('%-9s SKIP  bad=%d/%d entries (与本地同源,不会触发下载)' % (name, bad, len(entries)))
            continue
        table[name] = pairs
        print('%-9s %-60s entries=%-6d pairs=%-6d skip0=%d' % (
            name, path.name, len(entries), len(pairs) // 2, skipped))
    body = json.dumps(table, separators=(',', ':')).encode('utf-8')
    OUT.write_bytes(body)
    print('written %s (%d bytes, md5=%s)' % (OUT, len(body), hashlib.md5(body).hexdigest()))


if __name__ == '__main__':
    main()

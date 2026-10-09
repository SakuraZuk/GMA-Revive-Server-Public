# -*- coding: utf-8 -*-
"""生成带 0hash 的补丁清单（客户端 patch_mgr.get_patchlist_data_dict 实测还原）。

0hash = md5( 去空白(json.dumps(无0hash清单, indent=2)) + str(len(处理后的bytes)) )
去空白 = 删 \x04 \x0a \x0c \x0d，再删 b'\n' b'\t' b' '

2026-10-05 实机证据修正：客户端 dump 时 **不排序 key**（按 py2 dict 迭代顺序，
对本清单即 JSON 到达顺序）。此前误用 sort_keys=True 导致实机
"patchlist data hash error"（服务端 71f295… vs 客户端 f33aca39…，后者与不排序
算法逐字节吻合）。win 真样本因键序恰好等于字典序而无法区分两种算法，勿再以其验证。
因此本生成器写文件与控制键序（android, 0time）必须保持一致——勿再加顶层键，
否则 py2 dict 迭代顺序会变。若必须加键，先实机验证。
"""
import hashlib
import json
import sys
from pathlib import Path


def compute_0hash(manifest: dict) -> str:
    # 不传 sort_keys：保持插入序（= 本文件写出行序），与实机客户端一致。
    dump = json.dumps(manifest, indent=2, ensure_ascii=True).encode('utf-8')
    for c in (b'\x04', b'\x0a', b'\x0c', b'\x0d'):
        dump = dump.replace(c, b'')
    for c in (b'\n', b'\t', b' '):
        dump = dump.replace(c, b'')
    return hashlib.md5(dump + str(len(dump)).encode('ascii')).hexdigest()


def main():
    src = Path(sys.argv[1]) if len(sys.argv) > 1 else Path('deploy/data/patch_list_manifest.json')
    dst = Path(sys.argv[2]) if len(sys.argv) > 2 else Path('deploy/data/patch_list_pub_android.txt')
    manifest = json.loads(src.read_text(encoding='utf-8'))
    manifest.pop('0hash', None)

    # 客户端 _get_npkmd5_file 强制要求 android.npkmd5="文件名 md5 大小"，缺失即
    # alert('Npk md5 info missing in patchlist') + game3d.exit()。
    # 2026-10-06 起改为真实 per-file md5 表（out/tools/gen_npkfilelist_md5.py 生成）：
    # 存在本地 npk 且需逐文件 diff 时（如 script.npk 的 12 个补丁模块），
    # 客户端 npk_patcher._check_file 会查 npkfilelist_md5info[fileid]，
    # 空表会 KeyError（实机 384809306 = 0x16EFB95A guis/shop/shop.py）。
    md5_file = Path('deploy/data/npk_filelist_md5.txt')
    if not md5_file.exists():
        md5_file.write_bytes(b'{}')
    data = md5_file.read_bytes()
    manifest['android']['npkmd5'] = '%s %s %d' % (
        md5_file.name, hashlib.md5(data).hexdigest(), len(data))

    manifest['0time'] = '2021-11-25-12.39.03'
    manifest['0hash'] = compute_0hash(dict(manifest))
    dst.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print('npkmd5 =', manifest['android']['npkmd5'])
    print('0hash =', manifest['0hash'], '->', dst)


if __name__ == '__main__':
    main()

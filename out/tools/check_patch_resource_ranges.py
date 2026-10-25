# -*- coding: utf-8 -*-
"""用正式资源的小块Range核验候选MD5；不写服务器，不输出分发地址。"""
import concurrent.futures
import hashlib
import json
import os
import re
import struct
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / 'out/evidence/qq-bugs-20261010'


def main():
    private = Path(os.environ['LOCALAPPDATA']) / 'hs-server/deployment-config/hotfix.json'
    script = next(iter(json.loads(private.read_text(encoding='utf-8'))['startup_scripts'].values()))
    origin = re.search(r"(?m)^\s*PATCH_URL\s*=\s*['\"]([^'\"]+)", script)[1]
    with urllib.request.urlopen(origin + '/pl/patch_list_pub_android.txt', timeout=15) as response:
        manifest = json.load(response)['android']
    report = json.loads((EVIDENCE / '完整逐文件校验候选.report.json').read_text(encoding='utf-8'))
    table = json.loads((EVIDENCE / '完整逐文件校验候选.json').read_text(encoding='utf-8'))
    target = urlsplit(manifest['0patchpath'].rstrip('/'))
    base = urlunsplit((target.scheme, target.hostname, target.path, '', ''))
    samples = []
    for row in report['资源核验']:
        name = row['资源']
        live = manifest['npk'][name].split()
        if int(live[0]) != row['大小'] or live[1] != row['整包MD5']:
            raise ValueError('正式清单已变化，不能混用候选')
        source = ROOT / row['源']
        with source.open('rb') as stream:
            header = stream.read(24)
            count = struct.unpack_from('<I', header, 4)[0]
            stream.seek(struct.unpack_from('<I', header, 20)[0])
            raw = stream.read(count * 28)
        entries = [struct.unpack_from('<IIIIIII', raw, i * 28) for i in range(count)]
        fid, encoded_offset, size, decoded, h1, h2, codec = min((v for v in entries if v[2]), key=lambda v: v[2])
        if size > 2 * 1024 * 1024:
            raise ValueError('最小条目超过只读采样上限')
        offset = encoded_offset ^ (((-100 - decoded) & 0xffffffff) ^ 0x7a090d89) if codec >= 62 else encoded_offset
        pairs = table[name]
        md5 = dict(zip(pairs[::2], pairs[1::2]))[fid]
        samples.append((name, offset, encoded_offset, size, md5))

    def verify(sample):
        name, offset, encoded, size, expected = sample
        result = {'资源': name, '采样字节': size}
        request = urllib.request.Request(base + '/' + name + '.npk', headers={'Range': 'bytes=%d-%d' % (offset, offset+size-1)})
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                data = response.read(size+1)
                result.update({'状态码': response.status, '长度一致': len(data) == size,
                               'MD5一致': hashlib.md5(data).hexdigest() == expected,
                               '范围一致': response.headers.get('Content-Range', '').startswith('bytes %d-%d/' % (offset, offset+size-1))})
        except Exception as error:
            result['错误类型'] = type(error).__name__
        if name == 'char1' and encoded != offset:
            wrong = urllib.request.Request(base + '/' + name + '.npk', headers={'Range': 'bytes=%d-%d' % (encoded, encoded+size-1)})
            try:
                with urllib.request.urlopen(wrong, timeout=15) as response:
                    result['原编码偏移状态码'] = response.status
            except urllib.error.HTTPError as error:
                result['原编码偏移状态码'] = error.code
        return result

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        results = list(pool.map(verify, samples))
    output = {'结果': results, '全部通过': all(r.get('状态码') == 206 and r.get('长度一致') and r.get('MD5一致') and r.get('范围一致') for r in results),
              '边界': '每包一个真实小块，证明Range与候选条目MD5；不是全条目网络下载或Android重组验收'}
    (EVIDENCE / '更新资源真实分段核验.json').write_text(json.dumps(output, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print(json.dumps(output, ensure_ascii=False, indent=2))
    if not output['全部通过']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()

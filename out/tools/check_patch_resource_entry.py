# -*- coding: utf-8 -*-
"""只读核验正式清单资源入口；报告隐藏分发地址，不下载大型资源正文。"""
import concurrent.futures
import hashlib
import json
import os
import re
import urllib.request
from datetime import datetime
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit


def main():
    private = Path(os.environ['LOCALAPPDATA']) / 'hs-server/deployment-config/hotfix.json'
    config = json.loads(private.read_text(encoding='utf-8'))
    script = next(iter(config['startup_scripts'].values()))
    origin = re.search(r"(?m)^\s*PATCH_URL\s*=\s*['\"]([^'\"]+)", script)[1]
    with urllib.request.urlopen(origin + '/pl/patch_list_pub_android.txt', timeout=15) as response:
        manifest = json.load(response)
    android = manifest['android']
    base = android['0patchpath'].rstrip('/')
    parsed = urlsplit(base)
    legacy = urlunsplit((parsed.scheme, parsed.hostname, parsed.path, '', ''))
    entries = []

    def inspect(item):
        name, value = item
        expected = int(value.split()[0])
        rows = []
        for kind, target in [('清单端口', base), ('原生80端口', legacy)]:
            row = {'资源': name, '入口': kind, '预期字节': expected}
            try:
                request = urllib.request.Request(target + '/' + name + '.npk', method='HEAD')
                with urllib.request.urlopen(request, timeout=15) as response:
                    row.update({'状态码': response.status, '长度一致': int(response.headers.get('Content-Length', -1)) == expected,
                                '支持分段': response.headers.get('Accept-Ranges') == 'bytes'})
            except Exception as error:
                row['错误类型'] = type(error).__name__
            rows.append(row)
        return rows

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        for result in pool.map(inspect, sorted(android['npk'].items())):
            entries.extend(result)
    filename, expected_md5, expected_size = android['npkmd5'].split()
    table_checks = []
    for kind, target in [('清单端口', base), ('原生80端口', legacy)]:
        row = {'入口': kind}
        try:
            with urllib.request.urlopen(target + '/' + filename, timeout=15) as response:
                data = response.read(16 * 1024 * 1024)
                table = json.loads(data)
                row.update({'状态码': response.status, '长度一致': len(data) == int(expected_size),
                            'MD5一致': hashlib.md5(data).hexdigest() == expected_md5,
                            '资源表数量': len(table)})
        except Exception as error:
            row['错误类型'] = type(error).__name__
        table_checks.append(row)
    report = {'检查时间': datetime.now().isoformat(), '资源入口': entries, '逐文件校验表': table_checks,
              '边界': '仅本机正式资源HEAD及校验表读取；不是vivo网络、完整资源哈希或Android启动验收'}
    target = Path(__file__).resolve().parents[2] / 'out/evidence/qq-bugs-20261010/vivo资源入口核验.json'
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()

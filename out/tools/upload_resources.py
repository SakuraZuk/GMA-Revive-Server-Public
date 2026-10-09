# -*- coding: utf-8 -*-
"""upload_resources.py — 上传资源 npk 到热更服 Nginx 目录(8083)。
文件:Documents 下大 npk + base.apk 内抽取的 scene/scenewd2 + 重打 script.npk。
"""
import io
import sys
import time
import zipfile
from pathlib import Path

import paramiko

ROOT = Path(r'.')
DOCS = ROOT / 'files/netease/h62/Documents'
REMOTE_DIR = '/opt/hs-res/patch_pub.android_1.0.128a588'
from local_ops_credentials import load_target
HOST, PORT, USER, PW = load_target('hotfix')

# (远端名, 本地路径)
UPLOADS = [
    ('char2.npk', DOCS / 'char2.npk'),
    ('char3.npk', DOCS / 'char3.npk'),
    ('char4.npk', DOCS / 'char4.npk'),
    ('char6.npk', DOCS / 'char6.npk'),
    ('effect.npk', DOCS / 'effect.npk'),
    ('scenewd1.npk', DOCS / 'scenewd1.npk'),
    ('uiicon.npk', DOCS / 'uiicon.npk'),
    ('wwisech.npk', DOCS / 'wwisech.npk'),
    ('wwisejp.npk', DOCS / 'wwisejp.npk'),
    ('script.npk', ROOT / 'out/repack/script.npk'),
]

APK_EXTRACT = ['scene.npk', 'scenewd2.npk']


def main():
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, port=PORT, username=USER, password=PW, timeout=30,
              allow_agent=False, look_for_keys=False)
    _, o, _ = c.exec_command('mkdir -p %s' % REMOTE_DIR)
    o.channel.recv_exit_status()
    sftp = c.open_sftp()

    # 从 APK 抽取 scene/scenewd2 到临时文件
    tmp = ROOT / 'out/repack/_apk_extract'
    tmp.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(ROOT / 'base.apk') as z:
        for name in APK_EXTRACT:
            dst = tmp / name
            if not dst.exists():
                data = z.read('assets/' + name)
                dst.write_bytes(data)
                print('extracted %s (%dB)' % (name, len(data)))
    for name in APK_EXTRACT:
        UPLOADS.append((name, tmp / name))

    total = 0
    t0 = time.time()
    for name, path in UPLOADS:
        size = path.stat().st_size
        total += size
        t1 = time.time()
        sftp.put(str(path), REMOTE_DIR + '/' + name)
        print('uploaded %-14s %10dB in %.1fs' % (name, size, time.time() - t1), flush=True)
    print('done: %d files, %.2f GB in %.1fs' % (
        len(UPLOADS), total / 1e9, time.time() - t0))
    _, o, _ = c.exec_command('ls -la %s | head -20; df -h / | tail -1' % REMOTE_DIR)
    print(o.read().decode('utf-8', 'replace'))
    c.close()


if __name__ == '__main__':
    main()

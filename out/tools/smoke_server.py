# -*- coding: utf-8 -*-
"""启动临时本机服务，验证业务推送、DNS、HTTPS、Range 和帧握手；最后清理进程。"""
import base64
import json
import os
from pathlib import Path
import socket
import ssl
import struct
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]


def port(kind=socket.SOCK_STREAM):
    with socket.socket(socket.AF_INET, kind) as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def request(base, path, data=None, method=None, headers=None):
    body = None if data is None else json.dumps(data, ensure_ascii=False).encode('utf-8')
    req = urllib.request.Request(base + path, data=body, method=method, headers=headers or {})
    with urllib.request.urlopen(req, timeout=3) as response:
        return response.status, response.read()


def recvall(sock, size):
    result = b''
    while len(result) < size:
        part = sock.recv(size-len(result))
        if not part:
            raise RuntimeError('帧响应提前断开')
        result += part
    return result


def main():
    processes = []
    checks = []
    with tempfile.TemporaryDirectory(prefix='hs-server-') as temporary:
        data_dir = Path(temporary)
        (data_dir / 'hotfix.json').write_text(json.dumps({
            'startup_scripts': {'1.0.125': '# -*- coding: utf-8 -*-\n# 幂等测试脚本\npass\n'},
            'runtime': {'index': 2, 'script': 'pass\n'}}, ensure_ascii=False), encoding='utf-8')
        (data_dir / 'accounts.dev.json').write_text(json.dumps({'local-test': {
            'password': 'local-test', 'avatars': [{'hostnum': 10001, 'avatar_info': {
                'nickname': '协议测试角色', 'level': 1, 'head_id': 0,
                'custom_head_image_url': '', 'head_box_id': 0}}]}}, ensure_ascii=False), encoding='utf-8')
        for name in ('patch_list_pub_android.txt', 'notice.xml'):
            (data_dir / name).write_bytes((ROOT / 'deploy/data' / name).read_bytes())
        (data_dir / 'resources').mkdir()
        (data_dir / 'resources/test.npk').write_bytes(b'0123456789')
        debug_port, game_port, hotfix_port, tls_port, sdk_port, login_port = [port() for _ in range(6)]
        dns_port = port(socket.SOCK_DGRAM)
        environment = os.environ.copy()
        environment.update({
            'HS_DATA_DIR': str(data_dir), 'HS_GAME_BIND': f'127.0.0.1:{game_port}',
            'HS_GAME_DEBUG_BIND': f'127.0.0.1:{debug_port}', 'HS_GAME_ADDRESS': f'127.0.0.1:{game_port}',
            'HS_HOTFIX_BIND': f'127.0.0.1:{hotfix_port}', 'HS_HOTFIX_TLS_BIND': f'127.0.0.1:{tls_port}',
            'HS_TLS_CERT': str(ROOT / 'work/mock/cert.pem'), 'HS_TLS_KEY': str(ROOT / 'work/mock/key.pem'),
            'HS_DNS_BIND': f'127.0.0.1:{dns_port}', 'HS_DNS_ANSWER': '192.0.2.20',
            'HS_SDK_BIND': f'127.0.0.1:{sdk_port}', 'HS_LOGIN_BIND': f'127.0.0.1:{login_port}'})
        environment['HS_HOTFIX_REQUIRED'] = ''
        log_handles = []
        try:
            for name in ('gameserver', 'hotfixserver', 'sdkserver', 'loginserver'):
                log = (data_dir / f'{name}.log').open('wb')
                log_handles.append(log)
                processes.append(subprocess.Popen([str(ROOT / f'out/bin/windows/{name}.exe')],
                    cwd=ROOT, env=environment, stdout=log, stderr=log,
                    creationflags=subprocess.CREATE_NO_WINDOW))
            debug, hotfix = f'http://127.0.0.1:{debug_port}', f'http://127.0.0.1:{hotfix_port}'
            for base in (debug, hotfix, f'http://127.0.0.1:{sdk_port}', f'http://127.0.0.1:{login_port}'):
                for attempt in range(60):
                    try:
                        request(base, '/healthz')
                        break
                    except OSError:
                        time.sleep(0.1)
                else:
                    raise RuntimeError('服务未启动')
            _, body = request(debug, '/debug/session', {})
            session_id = json.loads(body)['session_id']

            def rpc(method, args):
                _, result = request(debug, '/debug/rpc', {'session_id': session_id, 'method': method, 'args': args})
                return json.loads(result)['pushes']

            pushes = rpc('quick_login', [{'hostnum': 10001, 'account': 'local-test', 'password': 'local-test',
                'hotfix_index': 0, 'need_guide_ids': [], 'conn_type': 0}])
            assert [p['method'] for p in pushes] == ['login_result', 'on_get_all_avatars', 'on_hotfix_when_login']
            assert pushes[0]['args'] == [0, '', 0]
            assert pushes[1]['args'][0][0]['avatar_info']['nickname'] == '协议测试角色'
            _, body = request(debug, '/debug/become-player', {'session_id': session_id})
            player_pushes = json.loads(body)['pushes']
            assert [p['method'] for p in player_pushes] == ['sync_server_time']
            assert player_pushes[0]['args'][1] == -28800
            checks.append('登录成功、角色列表、玩家收尾及时间同步序列')
            assert rpc('query_hotfix', [0])[0]['args'] == ['pass\n', 2]
            assert rpc('query_hotfix', [2])[0]['args'] == ['', 2]
            assert rpc('query_hotfix', [3])[0]['args'] == ['', 3]
            checks.append('运行期热修更新、同索引和领先索引')
            _, body = request(hotfix, '/pl/patch_hotfix_data_pub?123')
            startup = json.loads(base64.b64decode(body))
            assert '1.0.125' in startup and startup['1.0.125'].endswith('pass\n')
            _, body = request(hotfix, '/pl/patch_list_pub_android.txt')
            assert json.loads(body)['android']['version'] == '1.0.128'
            status, body = request(hotfix, '/resources/test.npk', headers={'Range': 'bytes=2-5'})
            assert status == 206 and body == b'2345'
            status, body = request(hotfix, '/upload_patch_log', {})
            assert status == 200 and body == b''
            checks.append('启动期版本匹配、补丁清单、资源 Range 和日志兜底')
            # 使用历史本机自签证书，仅验证 TLS 服务确实启用；不证明 Android 已接入。
            with urllib.request.urlopen(f'https://127.0.0.1:{tls_port}/pl/patch_hotfix_data_pub',
                    context=ssl._create_unverified_context(), timeout=3) as response:
                assert json.loads(base64.b64decode(response.read())) == startup
            checks.append('本机 HTTPS 启动期响应')
            query = bytes.fromhex('123401000001000000000000')
            for label in 'h62.update.netease.com'.split('.'):
                query += bytes([len(label)]) + label.encode('ascii')
            query += bytes.fromhex('0000010001')
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
                sock.settimeout(3)
                sock.sendto(query, ('127.0.0.1', dns_port))
                response = sock.recv(4096)
                assert response[:2] == query[:2] and socket.inet_ntoa(response[-4:]) == '192.0.2.20'
            checks.append('本机 UDP 假 DNS 返回专用热更服地址')
            _, body = request(f'http://127.0.0.1:{sdk_port}', '/v1/route')
            assert json.loads(body)['address'] == f'127.0.0.1:{game_port}'
            _, body = request(f'http://127.0.0.1:{sdk_port}', '/v1/route?version=1.0.125')
            route = json.loads(body)
            assert route['mode'] == 'hotfix' and route['address'] == '192.0.2.20:443'
            _, body = request(f'http://127.0.0.1:{login_port}', '/v1/login', {})
            assert json.loads(body)['game'] == f'127.0.0.1:{game_port}'
            checks.append('SDK 版本路由及 POST 临时登录管理接口')
            with socket.create_connection(('127.0.0.1', game_port), timeout=3) as sock:
                sock.sendall(struct.pack('<IH', 2, 0))
                size = struct.unpack('<I', recvall(sock, 4))[0]
                frame = recvall(sock, size)
                assert frame[:3] == b'\x00\x00\x08'
            checks.append('MobileRPC 种子请求和响应帧')
            request(debug, '/debug/session', {'session_id': session_id}, method='DELETE')
        finally:
            for process in processes:
                if process.poll() is None:
                    process.terminate()
                process.wait(timeout=5)
            for log in log_handles:
                log.close()
        report = {'状态': '通过', '验证项目': checks, '范围': '本机编译产物联调；未做 Android 登录及远程部署',
                  '时间': time.strftime('%Y-%m-%d %H:%M:%S', time.gmtime(time.time()+8*3600))+' 亚洲上海'}
        (ROOT / 'out/server-smoke.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
        print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()

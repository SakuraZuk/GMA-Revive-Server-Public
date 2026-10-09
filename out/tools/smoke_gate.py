# -*- coding: utf-8 -*-
"""gate 传输适配冒烟（协议基线版）：模拟客户端握手与登录。

按后期实机证据验证密文回执、msgpack 参数 "_N" 字典、持久化角色与时间。
本工具是协议模拟验收，不能代替 Android 进入主城验收。

序列：seed_request → session_key(RSA-OAEP + 随机 padding + RC4 启用)
     → connect_server → create_entity(Account) 后回 set_send_rpc_salt
     → quick_login(client_info) → 登录推送序列断言 → query_hotfix。
"""
import base64
import json
import socket
import struct
import sys
import os
import time
from pathlib import Path
from datetime import datetime, timedelta, timezone
import msgpack

from Crypto.Cipher import ARC4, PKCS1_OAEP
from Crypto.Hash import SHA
from Crypto.PublicKey import RSA
from Crypto.Random import get_random_bytes

HOST = __import__('os').environ.get('SMOKE_HOST', '127.0.0.1')
PORT = int(__import__('os').environ.get('SMOKE_PORT', '9000'))

# ---- protobuf wire 最小实现 ----
def varint(v):
    out = b''
    while True:
        b = v & 0x7F
        v >>= 7
        out += bytes([b | (0x80 if v else 0)])
        if not v:
            return out

def field(num, wire, payload):
    tag = varint((num << 3) | wire)
    if wire == 2:
        return tag + varint(len(payload)) + payload
    return tag + payload  # wire==0: payload 已是 varint 字节

def parse_fields(data):
    out, i = [], 0
    while i < len(data):
        tag, i = decode_varint(data, i)
        num, wire = tag >> 3, tag & 7
        if wire == 0:
            v, i = decode_varint(data, i)
            out.append((num, v))
        elif wire == 2:
            l, i = decode_varint(data, i)
            out.append((num, data[i:i+l])); i += l
        else:
            raise ValueError('wire %d' % wire)
    return out

def decode_varint(data, i):
    v, shift = 0, 0
    while True:
        b = data[i]; i += 1
        v |= (b & 0x7F) << shift; shift += 7
        if not b & 0x80:
            return v, i

# ---- BSON 最小编码（与服务端 appendValue 对齐）----
def bson_doc(pairs):
    body = b''
    for key, val in pairs:
        if isinstance(val, bool):
            body += b'\x08' + key.encode() + b'\x00' + (b'\x01' if val else b'\x00')
        elif isinstance(val, int):
            if -2**31 <= val < 2**31:
                body += b'\x10' + key.encode() + b'\x00' + struct.pack('<i', val)
            else:
                body += b'\x12' + key.encode() + b'\x00' + struct.pack('<q', val)
        elif isinstance(val, str):
            raw = val.encode() + b'\x00'
            body += b'\x02' + key.encode() + b'\x00' + struct.pack('<i', len(raw)) + raw
        elif isinstance(val, list):
            sub = bson_doc([(str(i), e) for i, e in enumerate(val)])
            body += b'\x04' + key.encode() + b'\x00' + sub
        elif isinstance(val, dict):
            body += b'\x03' + key.encode() + b'\x00' + bson_doc(list(val.items()))
        elif val is None:
            body += b'\x0a' + key.encode() + b'\x00'
        else:
            raise TypeError(type(val))
    out = struct.pack('<i', len(body) + 5) + body + b'\x00'
    return out

def mp(v):
    # 最小 msgpack 编码（与 Go 端 EncodeMsgpackValue 对齐）
    if v is None:
        return bytes([0xc0])
    if isinstance(v, bool):
        return bytes([0xc3]) if v else bytes([0xc2])
    if isinstance(v, int):
        if 0 <= v < 128:
            return bytes([v])
        if -32 <= v < 0:
            return bytes([0xe0 | (v + 32)])
        if -128 <= v < 128:
            return bytes([0xd0]) + bytes([v & 0xff])
        if -32768 <= v < 32768:
            return bytes([0xd1]) + struct.pack('>h', v)
        return bytes([0xd2]) + struct.pack('>i', v)
    if isinstance(v, str):
        b = v.encode('utf-8')
        n = len(b)
        if n < 32:
            return bytes([0xa0 | n]) + b
        if n < 256:
            return bytes([0xd9]) + bytes([n]) + b
        return bytes([0xda]) + struct.pack('>H', n) + b
    if isinstance(v, list):
        n = len(v)
        h = bytes([0x90 | n]) if n < 16 else bytes([0xdc]) + struct.pack('>H', n)
        return h + b''.join(mp(e) for e in v)
    if isinstance(v, dict):
        n = len(v)
        h = bytes([0x80 | n]) if n < 16 else bytes([0xde]) + struct.pack('>H', n)
        return h + b''.join(mp(k) + mp(x) for k, x in v.items())
    raise TypeError(type(v))


def frame(cmd, payload):
    return struct.pack('<I', len(payload) + 2) + struct.pack('<H', cmd) + payload

class Client:
    def __init__(self):
        self.sock = socket.create_connection((HOST, PORT), timeout=10)
        self.buf = b''
        self.rx = None
        self.tx = None

    def enable_crypt(self, key):
        # 双向各自独立 RC4 实例（同一 key 各自 KSA），与 C 层双向流一致。
        self.rx = ARC4.new(key)
        self.tx = ARC4.new(key)

    def send(self, cmd, payload):
        wire = frame(cmd, payload)
        if self.tx:
            wire = self.tx.encrypt(wire)
        self.sock.sendall(wire)

    def recv_frame(self):
        while True:
            # 加密流必须先连续解密再按明文长度分帧。
            if len(self.buf) >= 4:
                total = struct.unpack('<I', self.buf[:4])[0]
                if len(self.buf) >= total + 4:
                    clear = self.buf[:total + 4]
                    self.buf = self.buf[total + 4:]
                    cmd = struct.unpack('<H', clear[4:6])[0]
                    return cmd, clear[6:]
            chunk = self.sock.recv(65536)
            if not chunk:
                raise ConnectionError('连接关闭')
            if self.rx:
                chunk = self.rx.decrypt(chunk)
            self.buf += chunk

def main():
    pub = RSA.import_key(open('deploy/data/login_key.pem', 'rb').read()).publickey()
    c = Client()

    # 1. seed_request → seed_reply{seed}
    c.send(0, b'')
    cmd, payload = c.recv_frame()
    assert cmd == 0, cmd
    seed = dict(parse_fields(payload))[1]
    print('seed_reply ok, seed=%d' % seed)

    # 2. session_key：SessionKey{header=pad, session_key=SHA1(32B), seed, tail=pad} RSA-OAEP 加密
    session_key = SHA.new(get_random_bytes(32)).digest()
    sk = field(1, 2, get_random_bytes(16))
    sk += field(2, 2, session_key)
    sk += field(3, 0, varint(seed))
    sk += field(4, 2, get_random_bytes(16))
    cipher = PKCS1_OAEP.new(pub, hashAlgo=SHA)
    # 实机语义：session_key 帧明文发出；此后 rx 期待密文（含 ok）、tx 加密。
    c.send(1, field(1, 2, cipher.encrypt(sk)))
    c.enable_crypt(session_key)
    cmd, _ = c.recv_frame()
    assert cmd == 1, 'session_key_ok 期望命令 1，收到 %d' % cmd
    print('session_key ok, RC4 已启用')

    # 3. connect_server(NEW_CONNECTION) → connect_reply + create_entity(Account)
    c.send(2, field(2, 0, varint(0)) + field(3, 2, b'device-smoke'))
    cmd, payload = c.recv_frame()
    assert cmd == 2, cmd
    cr = dict(parse_fields(payload))
    assert cr[2] == 1, 'connect_reply.type=%r' % cr[2]
    account_id = cr.get(3)
    cmd, payload = c.recv_frame()
    assert cmd == 3, cmd
    etype = dict(parse_fields(dict(parse_fields(payload))[2]))[1]
    assert etype == b'Account', etype
    print('connect_reply(CONNECTED) + create_entity(Account=%r) ok' % account_id)

    # 4. 模拟客户端 salt 交接（应被忽略），随后 quick_login
    em = field(2, 2, account_id) + field(3, 2, field(1, 2, b'set_send_rpc_salt')) \
         + field(4, 2, mp({'_0': {'salt': 'x'}}))
    c.send(3, em)
    em = field(2, 2, account_id) + field(3, 2, field(1, 2, b'quick_login')) \
         + field(4, 2, mp({'_0': {
             'hostnum': 10001, 'account': os.environ.get('SMOKE_ACCOUNT', 'local-test'), 'password': os.environ.get('SMOKE_PASSWORD', 'local-test'),
             'hotfix_index': 0, 'need_guide_ids': [1, 2], 'conn_type': 0}}))
    c.send(3, em)

    got = []
    while len(got) < 5:
        cmd, payload = c.recv_frame()
        if cmd == 3:  # create_entity(Avatar)
            entity = dict(parse_fields(payload))
            etype = dict(parse_fields(entity[2]))[1]
            assert etype == b'Avatar', etype
            avatar_id = entity[3]
            assert len(avatar_id) == 12
            avatar_info = msgpack.unpackb(entity[4], raw=False)
            assert avatar_info['nickname'] and avatar_info['level'] >= 1
            got.append('create_entity:Avatar')
            continue
        assert cmd == 5, cmd
        msg = dict(parse_fields(payload))
        name = dict(parse_fields(msg[3]))[1]
        args = msgpack.unpackb(msg[4], raw=False)
        if name == b'login_result':
            if os.environ.get('SMOKE_EXPECT_AUTH_FAILURE') == '1':
                assert args['_0'] == 9002, '错误密码未被拒绝'
                c.sock.settimeout(.5)
                try:
                    c.recv_frame()
                except (socket.timeout, ConnectionError):
                    pass
                else:
                    raise AssertionError('失败鉴权仍下发角色或玩家数据')
                c.sock.close()
                rejection={'状态':'通过','范围':'加密协议错误密码拒绝；非 Android 验收',
                           '服务器':HOST,'错误码':9002,'下发角色或玩家成功消息':False}
                destination=os.environ.get('SMOKE_REPORT')
                if destination:
                    Path(destination).write_text(json.dumps(rejection,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
                print('错误密码拒绝通过，未下发角色或玩家成功消息')
                return
            assert args['_0'] == 0, '鉴权失败：' + str(args)
        if name == b'sync_server_time':
            assert isinstance(args['_0'], float), '时间必须保持 Float 签名'
            assert abs(args['_0'] - time.time()) < 10
            assert args['_1'] == -28800
            display = datetime.fromtimestamp(args['_0'], timezone(timedelta(seconds=-args['_1'])))
            assert display.utcoffset() == timedelta(hours=8)
        got.append(name.decode())
    want = ['login_result', 'on_get_all_avatars', 'on_hotfix_when_login',
            'create_entity:Avatar', 'sync_server_time']
    assert got == want, '\n得到 %v\n期望 %v'.replace('%v', '%s') % (got, want)
    print('登录推送序列 ok:', ', '.join(got))

    # 5. query_hotfix(0) → on_query_hotfix_success
    em = field(2, 2, avatar_id) + field(3, 2, field(1, 2, b'query_hotfix')) \
         + field(4, 2, mp({'_0': 0}))
    c.send(3, em)
    cmd, payload = c.recv_frame()
    assert cmd == 5, cmd
    name = dict(parse_fields(dict(parse_fields(payload))[3]))[1]
    assert name == b'on_query_hotfix_success', name
    print('query_hotfix → on_query_hotfix_success ok')
    # 6. 无参时间查询使用空字典，验证实际 TCP 上的参数归一。
    em = field(2, 2, avatar_id) + field(3, 2, field(1, 2, b'query_server_time')) + field(4, 2, mp({}))
    c.send(3, em)
    cmd, payload = c.recv_frame()
    assert cmd == 5
    message = dict(parse_fields(payload))
    assert dict(parse_fields(message[3]))[1] == b'sync_server_time'
    args = msgpack.unpackb(message[4], raw=False)
    assert args['_1'] == -28800 and abs(args['_0'] - time.time()) < 10
    c.sock.close()
    result = {'状态': '通过', '范围': '加密协议模拟客户端；非 Android 主城验收',
              '服务器': HOST, '角色编号': avatar_id.hex(), '角色': avatar_info,
              '时间戳': args['_0'], '协议时区秒数': args['_1'],
              '验证': ['RSA/RC4 握手', '账号鉴权和角色创建', '登录推送顺序', '热修查询', '无参时间查询']}
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if os.environ.get('SMOKE_REPORT'):
        Path(os.environ['SMOKE_REPORT']).write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print('协议冒烟通过')

if __name__ == '__main__':
    sys.exit(main())

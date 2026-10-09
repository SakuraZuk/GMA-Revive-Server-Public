# -*- coding: utf-8 -*-
"""
npk_guess_key.py — 暴力测试密钥流 K 的生成构造
已恢复 K 前 16 字节作为指纹，测试候选 key × 算法组合
算法：RC4 / RC4-drop256 / RC4-drop768 / RC4-Sbox直出 / RC4A / 变异KSA
候选 key 来源：日志字符串(patch_key:android_netease)、包名、项目代号、NeoX 常量串
"""
import hashlib
import itertools

K = bytes.fromhex('93311973883021c26e5687f6c70ff1c1')
FP = K[:8]


def rc4_ksa(key, ksa_variant=0):
    S = list(range(256))
    j = 0
    for i in range(256):
        if ksa_variant == 0:      # 标准
            j = (j + S[i] + key[i % len(key)]) % 256
        elif ksa_variant == 1:    # 变异：+i
            j = (j + S[i] + key[i % len(key)] + i) % 256
        S[i], S[j] = S[j], S[i]
    return S


def rc4_prga(S, n, drop=0):
    S = S[:]
    i = j = 0
    out = bytearray()
    for _ in range(drop + n):
        i = (i + 1) % 256
        j = (j + S[i]) % 256
        S[i], S[j] = S[j], S[i]
        out.append(S[(S[i] + S[j]) % 256])
    return bytes(out[drop:])


CANDIDATES = [
    b'android_netease', b'android', b'netease', b'patch_key:android_netease',
    b'com.netease.hsqsl', b'hsqsl', b'h62', b'H62', b'h62.update.netease.com',
    b'NeoX', b'neox', b'NEOX', b'NXNpk', b'nxnpk', b'neox::filesystem',
    b'netease.hsqsl', b'163', b'163.com', b'patch', b'script', b'npk',
    b'hsqsl.npk', b'script.npk', b'netease2013', b'netease2014', b'2011',
    b'android_netease.android', b'android.android', b'pub', b'a584',
    b'\x00', b'\x01', b'\xff', b'1234567890', b'abcdefghijklmnopqrstuvwxyz',
    b'NetEase', b'NETEASE', b'yys', b'leihuo', b'dev', b'taptap',
]

# 扩展候选：所有候选的 md5/sha1 摘要作为 key 也测
ext = []
for c in CANDIDATES:
    ext.append(hashlib.md5(c).digest())
    ext.append(hashlib.sha1(c).digest())
CANDIDATES += ext

hits = []
for key in CANDIDATES:
    if not key:
        continue
    for variant in (0, 1):
        S = rc4_ksa(key, variant)
        for drop in (0, 256, 768):
            out = rc4_prga(S, 8, drop)
            if out == FP:
                hits.append(('RC4', key, variant, drop))
                print('HIT!', 'RC4', key, 'variant', variant, 'drop', drop)
        # Sbox 直出（无 PRGA 打乱）
        if bytes(S[:8]) == FP:
            hits.append(('SBOX', key, variant, 0))
            print('HIT!', 'SBOX', key, variant)

# key 逐字节 XOR 常量的变体
for key in list(CANDIDATES)[:40]:
    if not key:
        continue
    for c in (0x37, 0x5a, 0xa5, 0xff, 0x93):
        k2 = bytes(b ^ c for b in key[:16]) if len(key) >= 16 else key
        S = rc4_ksa(k2, 0)
        if rc4_prga(S, 8, 0) == FP:
            print('HIT! xor-variant', key[:16], hex(c))

print('done. hits:', len(hits))

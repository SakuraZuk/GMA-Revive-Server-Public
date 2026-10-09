# -*- coding: utf-8 -*-
"""Python 3 兼容的旧 rotor 解密器，按 CPython v2.3.7 源码移植。

参考源与完整许可证保留在 work/rotormodule_reference.c：
https://github.com/python/cpython/blob/v2.3.7/Modules/rotormodule.c
Copyright 1994 by Lance Ellinghouse. All Rights Reserved.
本模块用于读取当前游戏文件，不能把与标准库兼容当成游戏格式验证。
"""
# 以下保留参考实现的原始版权与许可声明。
# Copyright 1994 by Lance Ellinghouse,
# Cathedral City, California Republic, United States of America.
# All Rights Reserved
#
# Permission to use, copy, modify, and distribute this software and its
# documentation for any purpose and without fee is hereby granted,
# provided that the above copyright notice appear in all copies and that
# both that copyright notice and this permission notice appear in
# supporting documentation, and that the name of Lance Ellinghouse
# not be used in advertising or publicity pertaining to distribution
# of the software without specific, written prior permission.
#
# LANCE ELLINGHOUSE DISCLAIMS ALL WARRANTIES WITH REGARD TO
# THIS SOFTWARE, INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
# FITNESS, IN NO EVENT SHALL LANCE ELLINGHOUSE BE LIABLE FOR ANY SPECIAL,
# INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING
# FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT,
# NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION
# WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
import math


def signed_short(value):
    value &= 65535
    return value-65536 if value & 32768 else value


def c_divmod(value, divisor):
    # C 有符号除法向零截断，与 Python // 对负数的行为不同。
    quotient = abs(value)//divisor * (-1 if value < 0 else 1)
    return quotient, value-quotient*divisor


class Rotor:
    def __init__(self, key, count=6):
        keys=[995,576,767,671,463]
        for byte in key:
            rot=[(x<<3)|(x>>13) for x in keys]
            keys=[(rot[0]+byte)&65535,(rot[1]^byte)&65535,
                  (rot[2]-byte)&65535,(byte-rot[3])&65535,
                  (rot[4]^~byte)&65535]
        keys[1] |= 1
        self.seed=[signed_short(k) for k in keys[:3]]
        self.positions=[]
        self.advances=[]
        self.inverse=[]
        self.forward=[]
        for _ in range(count):
            self.positions.append(self.rand(256))
            self.advances.append(1+2*self.rand(128))
            forward=list(range(256))
            inverse=[0]*256
            for i in range(256,1,-1):
                q=self.rand(i)
                j=forward[q]
                forward[q]=forward[i-1]
                forward[i-1]=j
                inverse[j]=i-1
            inverse[forward[0]]=0
            self.inverse.append(inverse)
            self.forward.append(forward)

    def rand(self, size):
        next_seed=[]
        for seed,divisor,multiplier,subtractor,modulus in zip(
                self.seed,(177,176,178),(171,172,170),(2,35,63),(30269,30307,30323)):
            q,r=c_divmod(seed,divisor)
            value=multiplier*r-subtractor*q
            if value<0:
                value+=modulus
            next_seed.append(value)
        self.seed=next_seed
        value=sum(x/m for x,m in zip(next_seed,(30269.0,30307.0,30323.0)))
        value-=math.floor(value)
        if value>=1:
            value=0.0
        return int(value*size)%size

    def decrypt(self, data):
        # 兼容 rotor.decrypt：每次重置初始转子位置，不使用 decryptmore。
        positions=self.positions.copy()
        inverse=self.inverse
        advances=self.advances
        count=len(positions)
        out=bytearray(len(data))
        for at,value in enumerate(data):
            for i in range(count-1,-1,-1):
                value=positions[i]^inverse[i][value]
            out[at]=value
            self._advance(positions,advances,count)
        return bytes(out)

    def encrypt(self, data):
        # rotormodule.c RTR_e_char：i 正向,tp = forward[i][tp ^ positions[i]]
        positions=self.positions.copy()
        forward=self.forward
        advances=self.advances
        count=len(positions)
        out=bytearray(len(data))
        for at,value in enumerate(data):
            for i in range(count):
                value=forward[i][(value^positions[i])&255]
            out[at]=value
            self._advance(positions,advances,count)
        return bytes(out)

    @staticmethod
    def _advance(positions,advances,count):
        for i in range(count):
            total=positions[i]+advances[i]
            positions[i]=total&255
            if total>=256 and i<count-1:
                positions[i+1]=(positions[i+1]+1)&255

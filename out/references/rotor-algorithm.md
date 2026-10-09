# rotor 算法参考（CPython 2.3.7 Modules/rotormodule.c, Lance Ellinghouse）
# 用途：NeoX NpkImporter 用 rotor.newrotor(key).decrypt() 解密脚本负载
# 已验证密钥见 ../npk_decoded/script_loader_bindings.json (rotor_key 字段)
# 完整源码获取: https://raw.githubusercontent.com/python/cpython/v2.3.7/Modules/rotormodule.c
# 核心算法要点（Python3 移植要点）:
# - set_key: key 逐字节滚动 5 个 16bit 值 k1..k5 (移位3/循环+^~-运算, & 0xFFFF)
# - r_random: Wichman-Hill PRNG (seed[3], 常数 171/172/170, 177/176/178, 30269/30307/30323)
# - RTR_init: 每 rotor 洗牌置换 (Knuth), positions=r_rand(256), advances=1+2*r_rand(128)
# - e_char: tp = e_rotor[i*256 + ((positions[i]^tp) % 256)] 逐 rotor 前向, 然后 RTR_advance
# - d_char: 逆序 tc = (positions[i] ^ d_rotor[i*256+tc]) % 256
# - advance: positions[i]=(positions[i]+advances[i])%256, 进位时 positions[i+1]+=1
# - size=256, size_mask 被置 0 → 走 % 分支
# - decrypt 与 decryptmore: 前者每次 RTR_init 重置, 后者续态

# -*- coding: utf-8 -*-
"""只读定位资源包原生方法表和 Thumb 指令。"""
import struct
import hashlib
import sys
from pathlib import Path
from elftools.elf.elffile import ELFFile
from capstone import Cs, CS_ARCH_ARM, CS_MODE_THUMB

root = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
path = root / "work/apk/lib/armeabi-v7a/libclient.so"
data = path.read_bytes()
with path.open("rb") as fp:
    elf = ELFFile(fp)
    sections = [(s["sh_addr"], s["sh_offset"], s["sh_size"]) for s in elf.iter_sections() if s["sh_size"] and s["sh_type"] != "SHT_NOBITS"]
def va(offset):
    for address, start, size in sections:
        if start <= offset < start + size:
            return address + offset - start
    return None
def raw(address):
    for start, offset, size in sections:
        if start <= address < start + size:
            return offset + address - start
    raise ValueError("地址不在实际文件段")
md = Cs(CS_ARCH_ARM, CS_MODE_THUMB)
lines = []
lines.append("原生库SHA256：" + hashlib.sha256(data).hexdigest())
lines.append("NXNpk索引解码：0xf44486-0xf444a8，reserved>=62时减62，offset=(-100-uncompressed_size)^offset^0x7a090d89")
for ins in md.disasm(data[raw(0xf44476):raw(0xf444b0)], 0xf44476):
    lines.append(f"{ins.address:#x}: {ins.mnemonic} {ins.op_str}")
lines.append("文件名哈希原生算法：")
for ins in md.disasm(data[raw(0xf44a10):raw(0xf44ae0)],0xf44a10):
    lines.append(f"{ins.address:#x}: {ins.mnemonic} {ins.op_str}")
for ins in md.disasm(data[raw(0xf5c7ce):raw(0xf5c900)],0xf5c7ce):
    lines.append(f"{ins.address:#x}: {ins.mnemonic} {ins.op_str}")
for name in [b"get_indice_vector", b"set_encrypt_index_flag", b"get_info_from_index"]:
    offset = data.find(name + b"\0")
    address = va(offset)
    lines.append(f"方法 {name.decode()} 字符串地址 {address:#x}")
    needle = struct.pack("<I", address)
    start = 0
    while True:
        hit = data.find(needle, start)
        if hit < 0: break
        start = hit + 1
        function = struct.unpack_from("<I", data, hit + 4)[0]
        if not function & 1: continue
        try: code = raw(function & ~1)
        except ValueError: continue
        lines.append(f"方法表地址 {va(hit):#x} 实际函数 {function:#x}")
        for ins in md.disasm(data[code:code + 220], function & ~1):
            lines.append(f"{ins.address:#x}: {ins.mnemonic} {ins.op_str}")
(root / "out/dis/remaining-collection-npk-native.asm").write_text("\n".join(lines)+"\n",encoding="utf-8")
print("\n".join(lines))

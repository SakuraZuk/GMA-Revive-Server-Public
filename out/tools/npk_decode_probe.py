# -*- coding: utf-8 -*-
"""对原始解码函数进行定点反汇编和有界模拟，不把超时当作返回。"""
import sys
from pathlib import Path

from capstone import Cs, CS_ARCH_X86, CS_MODE_32
from exe_xref_scan import EXE, parse_pe, rva_to_off


def disassemble(start, length):
    data = Path(EXE).read_bytes()
    base, sections = parse_pe(data)
    off = rva_to_off(sections, start - base)
    if off is None:
        raise ValueError("地址没有对应的文件节区")
    for ins in Cs(CS_ARCH_X86, CS_MODE_32).disasm(data[off:off+length], start):
        print("%08x  %-8s %s" % (ins.address, ins.mnemonic, ins.op_str))


if __name__ == '__main__':
    if len(sys.argv) > 1 and sys.argv[1] == 'rotor':
        import json,zlib
        from rotor_compat import Rotor
        from mem_marshal_extract import Loader
        root=Path(__file__).resolve().parents[2]/'out/npk_decoded'
        evidence=json.loads((root/'script_loader_bindings.json').read_text(encoding='utf-8'))
        rotor=Rotor(evidence['rotor_key'].encode('ascii'))
        for rel in ['android_base/95AEBFC2.bin','android_patch/9D33611E.bin','pc_base/016E05C2.bin']:
            raw=(root/rel).read_bytes()
            decrypted=rotor.decrypt(raw)
            print(rel,'rotor 头=',decrypted[:16].hex())
            body=zlib.decompress(decrypted)
            body=bytes(b^233 if i<99 else b for i,b in enumerate(body))[1:-1]
            print('marshal 头=',body[:24].hex(),'长度=',len(body))
            loader=Loader(body)
            obj=loader.r_object()
            print('完整=',loader.p==len(body),'文件=',obj['filename'],'方法=',obj['names'])
        sys.exit()
    if len(sys.argv) > 1 and sys.argv[1] == 'payloads':
        import json
        from mem_marshal_extract import Loader
        root=Path(__file__).resolve().parents[2]/'out/npk_decoded'
        for label in ('android_base','pc_base'):
            meta=json.loads((root/label/'manifest.json').read_text(encoding='utf-8'))
            for row in meta['entries']:
                if not row['prefix'].startswith('1973') and not row['prefix'].startswith('19e1'):
                    blob=(root/label/row['file']).read_bytes()
                    print(label,row['hash'],len(blob),'头=',repr(blob[:160]))
                    if blob[:1]==b'c':
                        loader=Loader(blob)
                        obj=loader.r_object()
                        print('code 文件名=',repr(obj['filename']),'名称=',repr(obj['name']),
                              '方法名=',repr(obj['names']),'常量=',repr(obj['consts'])[:1500])
        sys.exit()
    if len(sys.argv) > 1 and sys.argv[1] == 'archives':
        import struct
        for path in [r'.\work\apk\assets\script.npk',
                     r'.\files\netease\h62\Documents\script.npk',
                     r'.\prototype\neteasehsqsl\script.npk']:
            data = Path(path).read_bytes()
            count = struct.unpack_from('<I',data,4)[0]
            index = struct.unpack_from('<I',data,20)[0]
            print(path, '长度=',len(data), '头=',data[:24].hex(),
                  '索引条目=',(len(data)-index)//28, '余数=',(len(data)-index)%28,
                  '头部计数差=',len(data)-index-count*28, '尾=',data[-32:].hex())
            es=[struct.unpack_from('<IIIIQI',data,index+j*28) for j in range(count)]
            tail=index+count*28
            print('索引尾=',hex(tail),'尾后数据=',data[tail:tail+32].hex(),
                  '越过索引的条目=',[(hex(e[0]),e[1],e[2],e[3]) for e in es if e[1]+e[2]>index][:10])
        sys.exit()
    if len(sys.argv) > 1 and sys.argv[1] == 'table':
        import struct
        data = Path(EXE).read_bytes()
        base, sections = parse_pe(data)
        for addr, count in [(0xbf6adc, 32), (0x1a16310, 16), (0x27c25e0, 16)]:
            off = rva_to_off(sections, addr-base)
            print(hex(addr), data[off:off+count].hex())
        sys.exit()
    if len(sys.argv) > 1 and sys.argv[1] == 'blocks':
        from npk_analyze import parse_npk
        from npk_try_invert import lz4_block_decompress
        _, entries, data = parse_npk(r'.\files\netease\h62\Documents\script.npk')
        for e in entries:
            raw = data[e['offset']:e['offset']+e['size']]
            print(e, 'raw=', raw[:20].hex())
            for label, src, expected in [('raw-size', raw, e['csize']),
                                         ('raw-csize', raw[:e['csize']], e['size']),
                                         ('invert-size', bytes(x^255 for x in raw),e['csize'])]:
                try:
                    result = lz4_block_decompress(src, expected)
                    print(label, len(result), result[:24].hex())
                except Exception as exc:
                    print(label, str(exc))
        sys.exit()
    disassemble(int(sys.argv[1], 0) if len(sys.argv) > 1 else 0xF09310,
                int(sys.argv[2], 0) if len(sys.argv) > 2 else 0x600)

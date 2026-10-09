# -*- coding: utf-8 -*-
"""导出已确认 redirect 模块与 C_file 方法表引用，不执行客户端代码。"""
import hashlib
import json
import struct
from pathlib import Path

from exe_xref_scan import EXE, parse_pe, rva_to_off
from mem_marshal_extract import Loader

ROOT=Path(__file__).resolve().parents[2]
METHODS=[b'get_rs_encrypt_key',b'get_rs_encrypt_funtion',b'get_rotor_encrypt_key']


def serializable(value):
    if isinstance(value,bytes):
        try:
            text=value.decode('utf-8')
            if all(ch.isprintable() for ch in text):
                return dict(text=text,hex=value.hex())
        except UnicodeDecodeError:
            pass
        return dict(hex=value.hex())
    if isinstance(value,dict):
        return {k:serializable(v) for k,v in value.items()}
    if isinstance(value,(list,tuple)):
        return [serializable(v) for v in value]
    return value


def main():
    source=ROOT/'out/npk_decoded/android_base/F416002A.bin'
    data=source.read_bytes()
    loader=Loader(data)
    obj=loader.r_object()
    if loader.p!=len(data) or obj['filename']!=b'redirect':
        raise ValueError('并非完整的 redirect code 对象')
    out=ROOT/'out/npk_decoded'
    (out/'redirect.json').write_text(json.dumps(serializable(obj),ensure_ascii=False,indent=2),encoding='utf-8')
    image=Path(EXE).read_bytes()
    base,sections=parse_pe(image)
    bindings=[]
    for name in METHODS:
        start=0
        while True:
            pos=image.find(name+b'\0',start)
            if pos<0:
                break
            start=pos+1
            sec=next(s for s in sections if s[3]<=pos<s[3]+s[4])
            string_va=base+sec[1]+pos-sec[3]
            needle=struct.pack('<I',string_va)
            at=0
            while True:
                ref=image.find(needle,at)
                if ref<0:
                    break
                at=ref+1
                ref_sec=next((s for s in sections if s[3]<=ref<s[3]+s[4]),None)
                if ref_sec is None:
                    continue
                ref_va=base+ref_sec[1]+ref-ref_sec[3]
                fields=struct.unpack_from('<IIII',image,ref)
                fn_off=rva_to_off(sections,fields[1]-base)
                row=dict(name=name.decode('ascii'),string_va=hex(string_va),
                         reference_va=hex(ref_va),reference_section=ref_sec[0],
                         candidate_function_va=hex(fields[1]),candidate_flags=fields[2],
                         table_words=[hex(x) for x in fields],
                         candidate_function_prefix=image[fn_off:fn_off+16].hex() if fn_off else None)
                bindings.append(row)
                print(name.decode('ascii'),json.dumps(row,ensure_ascii=False))
    report=dict(exe_sha256=hashlib.sha256(image).hexdigest(),
                module_sha256=hashlib.sha256(data).hexdigest(),bindings=bindings)
    from capstone import Cs,CS_ARCH_X86,CS_MODE_32
    assembly=[]
    for addr,length in [(0xa7fcd0,16),(0xa7fce0,48),(0xa7fd10,16)]:
        offset=rva_to_off(sections,addr-base)
        for ins in Cs(CS_ARCH_X86,CS_MODE_32).disasm(image[offset:offset+length],addr):
            assembly.append('%08x %s %s'%(ins.address,ins.mnemonic,ins.op_str))
    (out/'script_loader_bindings.asm').write_text('\n'.join(assembly)+'\n',encoding='utf-8')
    # 两个绑定函数的实际反汇编均为 push 字符串 VA; call PyString 构造函数。
    for label,addr in [('rotor_key',0x1bbcda8),('script_decrypt_source',0x1bbced8)]:
        offset=rva_to_off(sections,addr-base)
        end=image.index(b'\0',offset)
        raw=image[offset:end]
        report[label]=raw.decode('ascii')
        print(label,repr(report[label]))
    android=(ROOT/'work/apk/lib/armeabi-v7a/libclient.so').read_bytes()
    report['android_lib_sha256']=hashlib.sha256(android).hexdigest()
    report['android_strings']={}
    for label in ['rotor_key','script_decrypt_source']:
        pos=android.find(report[label].encode('ascii')+b'\0')
        report['android_strings'][label]=dict(exact_match=pos>=0,file_offset=hex(pos) if pos>=0 else None)
        print('Android',label,report['android_strings'][label])
    (out/'script_loader_bindings.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')


if __name__=='__main__':
    main()

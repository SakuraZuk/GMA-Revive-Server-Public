# -*- coding: utf-8 -*-
"""恢复脚本 marshal，并严格验证完整消费；不执行字节码或内嵌源码。

顺序由 redirect.load_module 的方法表/字节码以及 C_file 原函数确认：
rotor.decrypt -> zlib.decompress -> script_decrypt -> marshal.loads。
前缀 XOR 参数为 C_file VA 0xa7fce0 的两个整数常量 233/99。
"""
import hashlib
import json
import sys
import zlib
from pathlib import Path

from mem_marshal_extract import Loader, MarshalError
from rotor_compat import Rotor

ROOT=Path(__file__).resolve().parents[2]


def text_value(value):
    return value.decode('utf-8',errors='strict') if isinstance(value,bytes) else value


def methods(code, prefix=''):
    result=[]
    name=text_value(code['name'])
    full=prefix+'.'+name if prefix else name
    result.append(dict(name=full,argcount=code['argcount'],
                       arguments=[text_value(x) for x in code['varnames'][:code['argcount']]],
                       firstlineno=code['firstlineno'],
                       referenced_names=[text_value(x) for x in code['names']],
                       bytecode_hex=code['bytecode'].hex()))
    for const in code['consts']:
        if isinstance(const,dict) and const.get('type')=='code':
            result.extend(methods(const,full))
    return result


def main(label):
    decoded=ROOT/'out/npk_decoded'/label
    manifest=json.loads((decoded/'manifest.json').read_text(encoding='utf-8'))
    binding=json.loads((ROOT/'out/npk_decoded/script_loader_bindings.json').read_text(encoding='utf-8'))
    rotor=Rotor(binding['rotor_key'].encode('ascii'))
    output=ROOT/'out/npk_scripts'/label
    output.mkdir(parents=True,exist_ok=True)
    rows=[]
    for item in manifest['entries']:
        raw=(decoded/item['file']).read_bytes()
        row=dict(hash=item['hash'],decoded_sha256=hashlib.sha256(raw).hexdigest())
        try:
            if raw[:1].isalpha() and raw[1:3] == bytes((58, 92)):
                # 构建清单本来就是普通文本，仅保存，不套用脚本解密。
                (output/'build_filelist.bin').write_bytes(raw)
                row.update(status='构建清单',file='build_filelist.bin')
                rows.append(row)
                continue
            if raw[:1]==b'c':
                marshal=raw
                transform='原始 marshal'
            else:
                inflated=zlib.decompress(rotor.decrypt(raw))
                marshal=bytes(b^233 if i<99 else b for i,b in enumerate(inflated))[1:-1]
                transform='rotor → zlib → 前99字节XOR233 → 去首尾字节'
            loader=Loader(marshal)
            code=loader.r_object()
            if loader.p!=len(marshal) or not isinstance(code,dict) or code.get('type')!='code':
                raise ValueError('marshal 未完整消费或顶层不是 code')
            filename=text_value(code['filename'])
            name=item['hash']+'.marshal'
            (output/name).write_bytes(marshal)
            row.update(status='完整 code 对象',file=name,filename=filename,
                       marshal_size=len(marshal),marshal_sha256=hashlib.sha256(marshal).hexdigest(),
                       transform=transform,methods=methods(code))
        except (MarshalError,ValueError,TypeError,IndexError,RecursionError,zlib.error) as exc:
            row.update(status='失败',error=str(exc))
        rows.append(row)
    successes=sum(r['status']=='完整 code 对象' for r in rows)
    failures=[r for r in rows if r['status']=='失败']
    report=dict(source_archive=manifest['source'],source_sha256=manifest['archive_sha256'],
                total=len(rows),complete_code_objects=successes,failed=len(failures),entries=rows)
    (output/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
    print('%s：完整 code 对象=%d，构建清单=%d，失败=%d' % (
        label,successes,len(rows)-successes-len(failures),len(failures)))
    for row in failures[:20]:
        print('  %s：%s' % (row['hash'],row['error']))
    return len(failures)


if __name__=='__main__':
    sys.exit(1 if main(sys.argv[1]) else 0)

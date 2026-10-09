# -*- coding: utf-8 -*-
"""根据已解析元数据生成移动端模块索引；方法名不等于 RPC 注册声明。"""
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]/'out/npk_scripts'


def main():
    modules={}
    overlays=[]
    for label in ['android_base','android_patch']:
        manifest=json.loads((ROOT/label/'manifest.json').read_text(encoding='utf-8'))
        if manifest['failed']:
            raise ValueError('不能从存在解析失败的清单生成完整索引')
        for row in manifest['entries']:
            if row['status']!='完整 code 对象':
                continue
            key=row['hash']
            if key in modules:
                overlays.append(key)
            modules[key]=dict(hash=key,filename=row['filename'],source=label,
                              marshal_file=label+'/'+row['file'],marshal_sha256=row['marshal_sha256'],
                              methods=[{k:v for k,v in m.items() if k!='bytecode_hex'} for m in row['methods']])
    output=dict(base_then_patch_by_hash=True,patch_overlays=overlays,module_count=len(modules),
                method_count=sum(len(r['methods']) for r in modules.values()),
                rpc_registration_verified=False,
                modules=sorted(modules.values(),key=lambda r:r['filename']))
    (ROOT/'android_inventory.json').write_text(json.dumps(output,ensure_ascii=False,indent=2),encoding='utf-8')
    print('移动端模块=%d，嵌套 code 对象=%d，热修覆盖=%d；RPC 注册关系仍待验证' % (
        output['module_count'],output['method_count'],len(overlays)))
    for row in output['modules']:
        if row['hash'] in ('B9F5C696','2034D84C','8A6BFA34','D7B7FC3C','1F09877D','315EAFEB'):
            print(row['filename'],row['hash'],row['source'])


if __name__=='__main__':
    main()

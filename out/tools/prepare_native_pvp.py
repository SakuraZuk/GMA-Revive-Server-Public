"""从本版已解码Android资源准备隔离原生引擎；不执行原厂启动脚本。"""
from pathlib import Path
import datetime,hashlib,json,sys,tarfile,io,urllib.request,zipfile

ROOT=Path(__file__).resolve().parents[2]
ENGINE=ROOT/'internal/nativepvp'
BASE=ENGINE/'runtime/native_engine'

def main():
    sys.stdout.reconfigure(encoding='utf-8')
    sys.path.insert(0,str(ENGINE))
    from battle_native import MarshalReader,NativeCode,translate_code,marshal_dump,PYC_MAGIC
    inv=ROOT/'out/npk_scripts/android_inventory.json'
    data=json.loads(inv.read_text(encoding='utf-8'))
    rows=[];failures=[]
    for row in data['modules']:
        source=ROOT/'out/npk_scripts'/row['marshal_file']
        raw=source.read_bytes()
        if hashlib.sha256(raw).hexdigest()!=row['marshal_sha256']:
            raise ValueError('Android原生输入SHA不符：'+row['hash'])
        filename=row['filename'].replace('\\','/')
        if not filename.endswith('.py'):continue
        parts=Path(filename).parts
        if 'script' in parts:parts=parts[parts.index('script')+1:]
        if any(part in ('..','') for part in parts) or Path(*parts).is_absolute():
            raise ValueError('原生模块路径不合法')
        dest=BASE/'script'/Path(*parts).with_suffix('.pyc')
        if not dest.resolve().is_relative_to((BASE/'script').resolve()):raise ValueError('原生模块越界')
        try:
            reader=MarshalReader(raw);co=reader.read()
            if reader.pos!=len(raw) or not isinstance(co,NativeCode):raise ValueError('完整原生code对象校验失败')
            translated=translate_code(co);blob=PYC_MAGIC+b'\0'*4+marshal_dump(translated)
            dest.parent.mkdir(parents=True,exist_ok=True);dest.write_bytes(blob)
            rows.append({'模块':filename,'原始SHA256':row['marshal_sha256'],'输出SHA256':hashlib.sha256(blob).hexdigest()})
        except Exception as error:
            failures.append({'模块':filename,'错误':str(error)})
    report={'时间':datetime.datetime.now().isoformat(),'来源':'本版Android base→patch覆盖后的已解析marshal；不是参考服PC数值','输入索引SHA256':hashlib.sha256(inv.read_bytes()).hexdigest(),'模块数':len(rows),'转换失败':failures,'模块':rows,'边界':'完成字节码转换不等于引擎完整运行或双Android验收'}
    (ROOT/'out/native-pvp-resource-preparation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({k:v for k,v in report.items() if k!='模块'},ensure_ascii=False))
    return 1 if failures else 0

if __name__=='__main__':raise SystemExit(main())

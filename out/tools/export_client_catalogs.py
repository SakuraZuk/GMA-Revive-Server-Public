# -*- coding: utf-8 -*-
"""静态导出 Android 数据表与 RPC 证据，不执行恢复的游戏字节码。

仅解释白名单常量、容器、Record 构造和 gtext 文本包装。遇到未知控制流或
调用立即记为未解析，不调用 eval、exec、marshal.loads 或客户端模块。
"""
import base64
from dataclasses import dataclass
import hashlib
import json
from pathlib import Path
import sys

from mem_marshal_extract import Loader

ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "out/npk_scripts"
DEST = ROOT / "out/client_catalogs"


def text(value):
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)


def instructions(code):
    raw = code["bytecode"]
    offset = extended = 0
    while offset < len(raw):
        op = raw[offset]
        width = 3 if op >= 90 else 1
        if offset + width > len(raw):
            raise ValueError("截断指令：" + str(offset))
        arg = extended | int.from_bytes(raw[offset+1:offset+3],"little") if width == 3 else None
        extended = arg << 16 if op == 160 else 0
        yield offset, op, arg
        offset += width


def walk(code, prefix=""):
    name = text(code["name"])
    path = prefix + "/" + name if prefix else name
    yield path, code
    for value in code["consts"]:
        if isinstance(value, dict) and value.get("type") == "code":
            yield from walk(value, path)


@dataclass
class Ref:
    name: str


@dataclass
class Function:
    code: dict
    defaults: tuple = ()


@dataclass
class RecordClass:
    name: str
    fields: list


def field_order(code):
    """按 __init__ 的 LOAD_FAST(参数)、LOAD_FAST(self)、STORE_ATTR 精确映射。"""
    init = next((value for value in code['consts'] if isinstance(value,dict) and value.get('type')=='code' and text(value['name'])=='__init__'), None)
    if init is None:
        raise ValueError("类没有字段构造器")
    order = [None] * (init['argcount'] - 1)
    ins = list(instructions(init))
    for index, (_,op,arg) in enumerate(ins):
        if op == 132 and index >= 2:
            _, load, parameter = ins[index-2]
            _, self_load, self_index = ins[index-1]
            if load == 97 and self_load == 97 and self_index == 0 and 1 <= parameter <= len(order):
                order[parameter-1] = text(init['names'][arg])
    if any(field is None for field in order) or len(set(order)) != len(order):
        raise ValueError("Record 字段映射不完整")
    return order


def evaluate_table(code):
    stack = []
    globals_ = {'True':True,'False':False,'None':None}
    record_classes = {}
    def popn(n):
        if n > len(stack): raise ValueError("栈参数不足")
        if not n: return []
        values = stack[-n:]
        del stack[-n:]
        return values
    for offset,op,arg in instructions(code):
        try:
            if op == 153:
                stack.append(code['consts'][arg])
            elif op in (101,155):
                name=text(code['names'][arg])
                stack.append(globals_.get(name,Ref(name)))
            elif op == 116:
                globals_[text(code['names'][arg])]=stack.pop()
            elif op in (135,90):
                values=popn(arg)
                stack.append(tuple(values) if op==135 else values)
            elif op == 99:
                stack.append(set(popn(arg)))
            elif op == 151:
                stack.append({})
            elif op == 8:
                key,value=stack.pop(),stack.pop()
                if not isinstance(stack[-1],dict): raise ValueError("字典写入目标不是容器")
                if key in stack[-1]: raise ValueError("重复字典键")
                stack[-1][key]=value
            elif op == 100:
                function=stack.pop()
                defaults=tuple(popn(arg))
                stack.append(Function(function,defaults))
            elif op == 131:
                if arg >> 8: raise ValueError("暂未支持关键字构造")
                values=popn(arg & 255)
                function=stack.pop()
                if isinstance(function,Function) and not values:
                    stack.append(function)
                elif isinstance(function,Ref) and function.name == 'gtext' and len(values)==1:
                    # 文本包装保留原文，不猜本地化 ID。
                    stack.append(values[0])
                elif isinstance(function,RecordClass):
                    if len(values)!=len(function.fields): raise ValueError("Record 参数数量不匹配")
                    stack.append(dict(zip(function.fields,values)))
                else:
                    raise ValueError("未白名单调用："+str(function))
            elif op == 33:
                body,bases,name=stack.pop(),stack.pop(),text(stack.pop())
                if not isinstance(body,Function): raise ValueError("未知类体")
                record=RecordClass(name,field_order(body.code))
                record_classes[name]=record.fields
                stack.append(record)
            elif op == 7:
                stack.pop()
            elif op == 160:
                pass
            elif op == 83:
                stack.pop()
                if stack: raise ValueError("模块结束后栈未清空")
                return globals_.get('data'),record_classes,globals_
            else:
                raise ValueError("暂未支持指令："+str(op))
        except (IndexError,TypeError,ValueError) as error:
            raise ValueError(f"模块指令偏移 {offset}：{error}") from error
    raise ValueError("模块未正常结束")


def json_value(value):
    if isinstance(value,bytes):
        try: return value.decode('utf-8')
        except UnicodeDecodeError: return {'原始字节base64':base64.b64encode(value).decode('ascii')}
    if isinstance(value,(tuple,list)):
        return [json_value(item) for item in value]
    if isinstance(value,dict):
        if value.get('type')=='code':
            return {'代码对象':text(value['name']), '参数个数':value['argcount'],
                    '参数名':json_value(value['varnames']), '引用名':json_value(value['names']),
                    '自由变量':json_value(value.get('freevars',())), '局部闭包变量':json_value(value.get('cellvars',())),
                    '字节码base64':base64.b64encode(value['bytecode']).decode('ascii'),
                    '字节码SHA256':hashlib.sha256(value['bytecode']).hexdigest(), '常量':json_value(value['consts'])}
        # 整数 ID 字典键统一转文本；复合键使用带类型的条目列表避免碰撞。
        if all(isinstance(key,(str,int)) or (isinstance(key,bytes) and valid_utf8(key)) for key in value):
            result={}
            for key,item in value.items():
                key=text(key)
                if key in result: raise ValueError("JSON 键转换碰撞")
                result[key]=json_value(item)
            return result
        return {'键值条目':[{'键':json_value(key),'值':json_value(item)} for key,item in value.items()]}
    if isinstance(value,Function):
        return {'未执行表达式':json_value(value.code),'默认参数':json_value(value.defaults)}
    if isinstance(value,(Ref,RecordClass)):
        raise ValueError('数据含未解析符号')
    if value is None or isinstance(value,(str,int,float,bool)):
        return value
    if isinstance(value,(set,frozenset)):
        return {'集合':[json_value(item) for item in sorted(value,key=repr)]}
    raise ValueError('未支持常量类型：'+type(value).__name__)


def valid_utf8(value):
    try: value.decode('utf-8'); return True
    except UnicodeDecodeError: return False


def count_markers(value,key):
    if isinstance(value,dict): return int(key in value)+sum(count_markers(item,key) for item in value.values())
    if isinstance(value,list): return sum(count_markers(item,key) for item in value)
    return 0


def decorated_rpcs(code,source):
    """只认显式 rpc.rpc('签名') + MAKE_FUNCTION + 装饰 CALL + STORE 模式。"""
    result=[]
    for path,owner in walk(code):
        ins=list(instructions(owner))
        for index,(_,op,arg) in enumerate(ins):
            if op!=116 or index<7: continue
            # 跳过未带显式签名的普通方法，避免按 on_/sync_ 前缀猜 RPC。
            previous=ins[max(0,index-12):index]
            makes=[i for i,item in enumerate(previous) if item[1]==100]
            if not makes: continue
            make=makes[-1]
            if make==0 or previous[make-1][1]!=153 or previous[-1][1]!=131 or previous[-1][2]!=1: continue
            method=owner['consts'][previous[make-1][2]]
            if not isinstance(method,dict) or method.get('type')!='code': continue
            # 装饰器参数调用必须直接紧邻 code 常量。
            call=make-2-previous[make][2]
            if call<2 or previous[call][1]!=131: continue
            nargs=previous[call][2]
            if nargs not in (0,1,2,3): continue
            loads=previous[call-nargs:call]
            if any(item[1]!=153 for item in loads): continue
            values=[owner['consts'][item[2]] for item in loads]
            signature=values[0] if values else ''
            options=values[1] if len(values)>1 else 'server'
            if not isinstance(signature,(str,bytes)) or not isinstance(options,(str,bytes)): continue
            attr=previous[call-nargs-1]
            base=previous[call-nargs-2]
            if attr[1]!=96 or text(owner['names'][attr[2]])!='rpc' or base[1] not in (101,155) or text(owner['names'][base[2]])!='rpc': continue
            name=text(owner['names'][arg])
            default_count=previous[make][2]
            default_loads=previous[call+1:make-1]
            if len(default_loads)==default_count and all(item[1]==153 for item in default_loads):
                defaults=json_value([owner['consts'][item[2]] for item in default_loads])
            else:
                defaults={'状态':'复杂默认值待静态求值','指令证据':[{'偏移':offset,'操作码':opcode,'操作数':operand} for offset,opcode,operand in default_loads]}
            proven_pushes={'login_result','on_get_all_avatars','on_hotfix_when_login','on_query_hotfix_success','sync_server_time','on_kick_avatar','call_client_callback'}
            direction='服务端到客户端（登录链或服务端实际推送已补证）' if name in proven_pushes else '客户端装饰方法，网络方向待调用链确认'
            result.append({**source,'类路径':path,'方法':name,'签名':text(signature),'装饰器选项':text(options),
                           '装饰器附加参数':json_value(values[2:]),'默认参数数量':default_count,'默认参数':defaults,
                           '参数名':[text(item) for item in method['varnames'][:method['argcount']]],
                           '方向':direction,'注册指令偏移':ins[index][0],
                           '证据':'显式 utils.rpc.rpc 装饰器；不按方法名前缀推测'})
    return result


def proxy_calls(code,source):
    result=[]
    for path,owner in walk(code):
        ins=list(instructions(owner))
        for index,(offset,op,arg) in enumerate(ins):
            if op!=96 or text(owner['names'][arg])!='server_proxy': continue
            names=[]
            after=index+1
            while after<len(ins) and ins[after][1]==96:
                names.append(text(owner['names'][ins[after][2]]));after+=1
            if not names: continue
            # LOAD_ATTR 只证明引用，后续 CALL 才能证明直接调用；间接 getattr 单列未恢复。
            calls=[]
            for item in ins[after:after+100]:
                if item[1] in (116,104,83,137,148,172): break
                if item[1] in (131,138,140,141,142): calls.append(item)
            result.append({**source,'调用方法':'/'.join(names),'所在方法':path,'引用指令偏移':offset,
                           '方向':'客户端到服务端','状态':'直接代理引用','后续调用指令偏移':[item[0] for item in calls],
                           '参数签名':'客户端引用不能证明服务端注册签名'})
    return result


def named_forward_calls(code,source):
    result=[]
    for path,owner in walk(code):
        ins=list(instructions(owner))
        for i,(offset,op,arg) in enumerate(ins):
            if op!=96 or text(owner['names'][arg])!='call_server': continue
            if i+1>=len(ins) or ins[i+1][1]!=153: continue
            method=owner['consts'][ins[i+1][2]]
            if not isinstance(method,(str,bytes)): continue
            result.append({**source,'方法':text(method),'所在方法':path,'指令偏移':offset,
                           '方向':'客户端到服务端（call_server 转发调用点）',
                           '签名':'运行时追加 callback_id；其他参数类型待服务端定义或调用点核验'})
    return result


def dump(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(value,ensure_ascii=False,indent=2,allow_nan=False)+'\n',encoding='utf-8')


def main():
    inventory=json.loads((SCRIPTS/'android_inventory.json').read_text(encoding='utf-8'))
    tables=[];rpcs=[];upcalls=[];forwards=[];errors=[];packages=[];configs=[];unparsed_rpc=[]
    for number,module in enumerate(inventory['modules'],1):
        path=SCRIPTS/module['marshal_file']
        raw=path.read_bytes()
        if hashlib.sha256(raw).hexdigest()!=module['marshal_sha256']: raise ValueError('输入哈希不匹配：'+str(path))
        loader=Loader(raw)
        code=loader.r_object()
        if loader.p!=len(raw): raise ValueError('输入未消费至末尾：'+str(path))
        filename=module['filename'].replace('\\','/')
        source={'模块':filename,'模块哈希':module['hash'],'来源包':module['source'],'来源SHA256':module['marshal_sha256']}
        registered=decorated_rpcs(code,source)
        rpcs.extend(registered)
        matched={(item['类路径'],item['注册指令偏移']) for item in registered}
        # 每个 rpc 属性调用都保留覆盖检查，显式列出复杂装饰器候选。
        for classpath,owner in walk(code):
            ins=list(instructions(owner))
            for i,(_,op,arg) in enumerate(ins):
                if op==96 and text(owner['names'][arg]) in ('rpc','rpc_method') and i and ins[i-1][1] in (101,155):
                    after=next((item for item in ins[i+1:] if item[1] in (116,104,83)),None)
                    if after and after[1]==116 and (classpath,after[0]) not in matched:
                        unparsed_rpc.append({**source,'类路径':classpath,'候选绑定名':text(owner['names'][after[2]]),'指令偏移':ins[i][0], '状态':'复杂装饰器候选，方向和签名待核验'})
        upcalls.extend(proxy_calls(code,source))
        forwards.extend(named_forward_calls(code,source))
        if filename.startswith('datas/'):
            entry={**source,'导出文件':'tables/'+filename.removeprefix('datas/').removesuffix('.py')+'.json'}
            try:
                data,classes,globals_=evaluate_table(code)
                if 'data' not in globals_ and filename.endswith('/__init__.py'):
                    entry.update({'状态':'包初始化模块','条目数':0})
                    packages.append(entry)
                    dump(DEST/entry['导出文件'],entry)
                    tables.append(entry)
                    continue
                if 'data' not in globals_: raise ValueError('模块无 data 顶层值')
                converted=json_value(data)
                entry.update({'状态':'静态结构还原通过','条目数':len(data) if isinstance(data,(dict,list,tuple)) else 1,'Record字段':classes,
                              '未执行表达式数':count_markers(converted,'未执行表达式'),'原始字节标记数':count_markers(converted,'原始字节base64')})
                constants={name:json_value(value) for name,value in globals_.items()
                           if name not in ('data','True','False','None') and not isinstance(value,RecordClass)}
                dump(DEST/entry['导出文件'],{**entry,'数据':converted,'模块常量':constants})
            except (ValueError,TypeError) as error:
                entry.update({'状态':'待补充解释','原因':str(error),'条目数':0})
                # 保留原始常量及来源，不将原始常量数组伪称为还原的数据表。
                dump(DEST/entry['导出文件'],{**entry,'常量证据':json_value(code['consts'])})
                errors.append(entry)
            tables.append(entry)
        elif any(word in Path(filename).stem.lower() for word in ('config','conf','const','setting','version')):
            values={}
            ins=list(instructions(code))
            for i,(_,op,arg) in enumerate(ins):
                if op==116 and i and ins[i-1][1]==153:
                    value=code['consts'][ins[i-1][2]]
                    if not (isinstance(value,dict) and value.get('type')=='code'):
                        values[text(code['names'][arg])]=json_value(value)
            configfile='configs/'+filename.removesuffix('.py')+'.json'
            configs.append({**source,'导出文件':configfile,'直接常量数':len(values),
                            '范围':'按模块命名选取的配置候选；只恢复直接常量赋值，不证明所有运行时配置'})
            dump(DEST/configfile,{**configs[-1],'直接常量':values,'顶层常量证据':json_value(code['consts'])})
        if number%500==0: print(f'已扫描 {number}/{len(inventory["modules"])} 模块',flush=True)
    summary={'扫描模块数':len(inventory['modules']),'数据模块数':len(tables),
             '数据静态结构还原通过':sum(item['状态']=='静态结构还原通过' for item in tables),
             '包初始化模块数':len(packages),
             '数据待补充解释':len(errors),'数据条目总数':sum(item['条目数'] for item in tables),
             '显式RPC装饰方法数':len(rpcs),'上行代理引用数':len(upcalls),
             '命名call_server调用点数':len(forwards),'命名call_server方法去重数':len({item['方法'] for item in forwards}),
             '复杂RPC注册候选数':len(unparsed_rpc),'配置候选模块数':len(configs),
             '未执行表达式引用数':sum(item.get('未执行表达式数',0) for item in tables),
             '原始字节标记数':sum(item.get('原始字节标记数',0) for item in tables),
             '上行代理方法去重数':len({item['调用方法'] for item in upcalls}),
             '上行方法名证据合并去重数':len({item['调用方法'] for item in upcalls}|{item['方法'] for item in forwards}),
             '边界':'静态客户端证据导出，不等于完整服务端实现；动态 getattr 调用和参数语义需继续核验'}
    dump(DEST/'table-index.json',{'统计':summary,'数据表':tables})
    dump(DEST/'rpc-catalog.json',{'统计':summary,'显式装饰方法':rpcs,'上行代理引用':upcalls,'命名转发调用':forwards,'复杂注册候选':unparsed_rpc})
    dump(DEST/'config-index.json',{'统计':summary,'配置候选':configs})
    dump(DEST/'summary.json',summary)
    print(json.dumps(summary,ensure_ascii=False,indent=2))


if __name__=='__main__':
    sys.stdout.reconfigure(encoding='utf-8')
    sys.stderr.reconfigure(encoding='utf-8')
    main()

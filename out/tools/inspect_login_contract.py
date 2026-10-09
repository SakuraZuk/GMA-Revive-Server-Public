# -*- coding: utf-8 -*-
"""只读取已恢复的 marshal，输出登录常量及相关模块位置，不执行游戏代码。"""
import json
from pathlib import Path
from mem_marshal_extract import Loader

ROOT = Path(__file__).resolve().parents[2]


def walk(code):
    yield code
    for value in code.get('consts', ()):
        if isinstance(value, dict) and value.get('type') == 'code':
            yield from walk(value)


def text(value):
    return value.decode('utf-8') if isinstance(value, bytes) else str(value)


if __name__ == '__main__':
    for path in sorted((ROOT / 'out/npk_scripts/android_base').glob('*.marshal')):
        code = Loader(path.read_bytes()).r_object()
        filename = text(code['filename']).replace('\\', '/')
        if any(word in filename for word in ('error_code.py', 'error_processor.py', 'RpcMethodArgs', 'rpcmethod', 'common_const.py')):
            print(path.name, filename)
            for item in walk(code):
                if text(item['name']) == 'check_ret_success':
                    print('方法', text(item['name']), '引用', ', '.join(map(text, item['names'])))
            if filename.endswith('error_code.py') or filename.endswith('common_const.py'):
                # 只还原 LOAD_CONST; STORE_NAME 这种无歧义常量赋值。
                bytecode = code['bytecode']
                offset = 0
                previous = None
                while offset < len(bytecode):
                    op = bytecode[offset]
                    arg = int.from_bytes(bytecode[offset+1:offset+3], 'little') if op >= 90 else None
                    if op == 116 and previous and previous[0] == 153:
                        name = text(code['names'][arg])
                        if name.startswith(('RET_LOGIN', 'KICK_AVATAR')) or name == 'RET_SUCCESS':
                            print(name, repr(code['consts'][previous[1]]))
                    previous = (op, arg)
                    offset += 3 if op >= 90 else 1

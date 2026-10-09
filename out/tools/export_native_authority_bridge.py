"""从已核验作者更新包导出隔离PVP桥；不读取账号、私钥或服务端运行数据。"""
from pathlib import Path
import ast, hashlib, json, sys, textwrap, zipfile

ROOT = Path(__file__).resolve().parents[2]
ARCHIVE = Path(__import__('os').environ.get('HS_NATIVE_SOURCE_ARCHIVE', 'out/source/GMA-Revive-Server-main.zip'))
EXPECTED = '1bdf673fdfdd188a07272a91e083d410e608b82ba0f7fe8aa1eebdc5af1f28fa'

def main():
    sys.stdout.reconfigure(encoding='utf-8')
    assert hashlib.sha256(ARCHIVE.read_bytes()).hexdigest() == EXPECTED, '作者更新包已变化，不能沿用旧来源'
    with zipfile.ZipFile(ARCHIVE) as bundle:
        source = bundle.read('GMA-Revive-Server-main/turnclock.py').decode('utf-8')
        target = bundle.read('GMA-Revive-Server-main/battle_targets.py').decode('utf-8')
    tree = ast.parse(source)
    functions = {node.name: node for node in tree.body if isinstance(node, ast.FunctionDef)}
    bridge_fn = functions['client_bridge_source']
    template = next(ast.literal_eval(n.value) for n in bridge_fn.body if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'source' for t in n.targets))
    revision = next(ast.literal_eval(n.value) for n in tree.body if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'BRIDGE_REVISION' for t in n.targets))
    helpers = {name: ast.get_source_segment(source, functions[name]) for name in ('rpc_from_native_command', 'native_control_command', 'native_support_command')}
    target_fn = next(n for n in ast.parse(target).body if isinstance(n, ast.FunctionDef) and n.name == 'skill_target_hooks')
    replacements = {'__TARGET_HOOKS__': ast.get_source_segment(target, target_fn),
                    '__RPC_FROM_NATIVE__': helpers['rpc_from_native_command'],
                    '__NATIVE_CONTROL_COMMAND__': helpers['native_control_command'],
                    '__NATIVE_SUPPORT_COMMAND__': helpers['native_support_command'].replace('isinstance(target, str)', 'isinstance(target, basestring)').replace('type(skill_id) is not int', 'type(skill_id) not in (int, long)').replace('type(value) is not int', 'type(value) not in (int, long)')}
    for marker, body in replacements.items(): template = template.replace(marker, textwrap.indent(body, '    '))
    template = template.replace('__BRIDGE_REVISION__', str(revision))
    # 只保留作者安装函数，禁止执行作者全局安装与重试入口。
    parsed = ast.parse(template)
    installer = next(n for n in parsed.body if isinstance(n, ast.FunctionDef) and n.name == '_install_revival_battle_bridge')
    body = ast.get_source_segment(template, installer).replace('def _install_revival_battle_bridge():', 'def _install_revival_authority_shadow_bridge():', 1)
    parsed = ast.parse(body)
    emit = next(n for n in ast.walk(parsed) if isinstance(n, ast.FunctionDef) and n.name == 'emit')
    payload = next(n for n in emit.body if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'payload' for t in n.targets))
    lines = body.splitlines()
    lines.insert(payload.end_lineno,
        "        payload['generation'] = int((getattr(battle, 'extra_info', {}) or {}).get('server_authority_generation') or 0)")
    body = '\n'.join(lines)
    parsed = ast.parse(body)
    gate = next(n for n in ast.walk(parsed) if isinstance(n, ast.FunctionDef) and n.name == '_revival_is_sync_pvp')
    lines = body.splitlines()
    lines[gate.lineno-1:gate.end_lineno] = [
        '    def _revival_is_sync_pvp(battle):',
        "        return bool(getattr(battle, '_revival_server_authority', False) and",
        "                    (getattr(battle, 'extra_info', {}) or {}).get('server_authoritative_pvp') is True)"]
    wrapper = (ROOT / 'internal/nativepvp/authority_bridge_wrapper.py').read_text(encoding='utf-8')
    result = '# -*- coding: utf-8 -*-\n# 作者新版PVP桥仅安装到专用子类；普通rev12桥与握手1保留。\n' + '\n'.join(lines) + '\n\n' + wrapper
    dest = ROOT / 'internal/game/native_authority_bridge_script.py'
    dest.write_text(result, encoding='utf-8', newline='\n')
    report = {'更新包SHA256': EXPECTED, '作者桥版本': revision,
              '作者turnclock源码SHA256': hashlib.sha256(source.encode()).hexdigest(),
              '包装源SHA256': hashlib.sha256((ROOT / 'internal/nativepvp/authority_bridge_wrapper.py').read_bytes()).hexdigest(),
              '导出桥SHA256': hashlib.sha256(dest.read_bytes()).hexdigest(),
              '边界': '独立子类、显式标志与原生RPC元数据函数包装；尚未加入线上热更或Go房间路线'}
    (ROOT / 'out/native-authority-bridge-export.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print(json.dumps(report, ensure_ascii=False))

if __name__ == '__main__': main()

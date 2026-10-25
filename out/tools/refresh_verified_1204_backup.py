"""核对指定已发布候选后，仅刷新本机私有分发备份并归档证据；默认保留1204历史入口。"""
import hashlib
import json
import os
import shutil
from pathlib import Path
from server_ops import connect, run
from deploy_hotfix_bridge import connect as connect_hotfix, run as run_hotfix

root = Path(__file__).resolve().parents[2]
import argparse
parser = argparse.ArgumentParser()
parser.add_argument('--stage', type=Path, default=Path(os.environ['LOCALAPPDATA']) / 'hs-server/release-workspaces/20261012-234411')
stage = parser.parse_args().stage.resolve()
allowed = (Path(os.environ['LOCALAPPDATA']) / 'hs-server/release-workspaces').resolve()
if stage.parent != allowed:
    raise ValueError('候选必须位于本机既有私有发布目录')
build = json.loads((stage/'out/completion-build.json').read_text(encoding='utf-8'))
assert build['退出码'] == 0
expected_game = build['产物哈希']['out/bin/linux-amd64/gameserver']
expected_hotfix = build['源码哈希']['deploy/data/hotfix.json']
index = json.loads((stage/'deploy/data/hotfix.json').read_text(encoding='utf-8'))['runtime']['index']
label = str(index)[-4:]
verified = json.loads((stage / 'out/native-solo-release-verification.json').read_text(encoding='utf-8'))
assert verified['状态'] == '通过' and verified['游戏SHA256'] == expected_game and verified['热更SHA256'] == expected_hotfix
game = connect()
try:
    assert run(game, 'sha256sum /opt/hs-server/bin/gameserver').split()[0] == expected_game
    assert run(game, 'sha256sum /opt/hs-server/data/hotfix.json').split()[0] == expected_hotfix
    body = run(game, 'cat /opt/hs-server/data/hotfix.json')
finally:
    game.close()
hotfix = connect_hotfix()
try:
    assert run_hotfix(hotfix, 'sha256sum /opt/hs-hotfix/data/hotfix.json').split()[0] == expected_hotfix
finally:
    hotfix.close()
candidate = stage / 'deploy/data/hotfix.json'
assert hashlib.sha256(candidate.read_bytes()).hexdigest() == expected_hotfix
assert json.loads(body) == json.loads(candidate.read_text(encoding='utf-8'))
backup = Path(os.environ['LOCALAPPDATA']) / 'hs-server/deployment-config/hotfix.json'
saved = backup.with_name(('hotfix-before-'+label+'-backup-refresh.json'))
if not saved.exists():
    shutil.copy2(backup, saved)
shutil.copy2(candidate, backup)
for name in ('completion-build.json', 'remote-database-verification.json', 'native-pvp-linux-verification.json', 'native-solo-release-verification.json', 'server-list-recovery-verification.json'):
    source = stage / 'out' / name
    if source.exists():
        shutil.copy2(source, root / 'out' / (('bug-all-'+label+'-final-') + name))
for source in (stage / 'out').glob('completion-release-*.json'):
    shutil.copy2(source, root / 'out' / (('bug-all-'+label+'-final-') + source.name))
print(str(index)+'现网游戏及双端配置哈希一致，本机私有备份与发布证据已更新。')

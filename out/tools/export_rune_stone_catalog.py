"""从既有原版导出表生成契印原石参数和星级/部位鉴定成本。"""
import hashlib
import json
from pathlib import Path
root = Path(__file__).resolve().parents[2]
result = {}
sources = {}
for name in ('rune_stone_params', 'rune_stone_prices'):
    path = root / 'out/client_catalogs/tables' / (name + '.json')
    body = json.loads(path.read_text(encoding='utf-8'))
    result[name] = body['数据']
    sources[name] = {'原包SHA256':body['来源SHA256'],'导出SHA256':hashlib.sha256(path.read_bytes()).hexdigest()}
result['来源'] = sources
(root/'internal/game/rune_stone_catalog.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('契印原石原表参数和20项鉴定价格已生成。')

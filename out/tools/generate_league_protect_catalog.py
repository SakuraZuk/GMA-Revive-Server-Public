# -*- coding: utf-8 -*-
"""学会守门人与宝藏原始表目录，避免把卡模板的技能当守门人技能。"""
import json,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
roles=json.loads((root/'out/client_catalogs/tables/role_info.json').read_text(encoding='utf-8'))['数据']
treasures=[int(rid) for rid,row in roles.items() if 501 in (row.get('role_tag') or [])]
data={'guardian':roles['-1'],'treasure_roles':treasures}
(root/'internal/game/league_protect_catalog.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('宝藏501原生角色',treasures)

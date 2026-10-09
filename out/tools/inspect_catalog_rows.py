"""检查已静态导出的Android表结构，输出少量行供业务取证。"""
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
root=Path(__file__).resolve().parents[2]/"out/client_catalogs/tables"
for name in sys.argv[1:]:
    name,_,key=name.partition(":")
    value=json.loads((root/(name+".json")).read_text(encoding="utf-8"))
    data=value.get("数据",{})
    sample={key:data.get(key)} if key else (list(data.items())[:2] if isinstance(data,dict) else data[:2])
    print(json.dumps({"表":name,"字段":value.get("Record字段"),"条目":len(data),"示例":sample},ensure_ascii=False))

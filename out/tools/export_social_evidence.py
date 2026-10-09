# -*- coding: utf-8 -*-
"""按 Android 原生模块导出社交及竞技接口取证，禁止执行游戏字节码。"""
import json
import hashlib
import sys
import re
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

ROOT = Path(__file__).resolve().parents[2]
HASHES = {
    "driver": "E3877551", "assist-panel": "8AFCE3B9", "async-reward-view": "0179BEAC",
    "camp": "C01C3E87", "player-input": "45F2CECD",
    "friend": "CB7E0654", "friend-type": "136799A0",
    "comment": "861CE174", "async-pvp": "F8D135CF",
    "sync-pvp": "D7C70F60", "record": "A66FD60A",
    "record-view": "3BC9394A", "async-view": "53FBDDD6",
    "comment-type": "89B6A072", "comment-view": "0D047421",
    "errors": "875C5BCF",
    "pvp-utils": "F6204D14",
    "card-type": "2948C748",
    "defence-record": "D0F7BB01",
    "async-record-view": "362EE013",
    "assist": "276BDA1D", "assist-type":"B978EF8A", "assist-utils":"114E38A8",
    "client-battle": "927728D8",
    "battle-base":"2CC3F05D", "battle-performance":"BAC02512", "driver-logic":"8364763B", "battle-entity":"63350DFB", "battle-input":"08C6EB3D", "client-control":"42B11C32", "server-control":"CF8BE798",
}

def main():
    if len(sys.argv)>2 and sys.argv[1]=="table":
        value=json.loads((ROOT/("out/client_catalogs/tables/"+sys.argv[2]+".json")).read_text(encoding="utf-8"))["数据"]
        print(json.dumps(value.get(sys.argv[3]) if len(sys.argv)>3 else list(value.items())[:3],ensure_ascii=False,indent=2))
        return
    inventory = json.loads((ROOT / "out/npk_scripts/android_inventory.json").read_text(encoding="utf-8"))
    if len(sys.argv)>2 and sys.argv[1]=="find-method":
        for row in inventory["modules"]:
            for method in row["methods"]:
                if method["name"].rsplit(".",1)[-1]==sys.argv[2]:
                    print(json.dumps({"hash":row["hash"],"filename":row["filename"],"method":method},ensure_ascii=False))
        return
    if len(sys.argv)>2 and sys.argv[1]=="find-ref":
        for row in inventory["modules"]:
            for method in row["methods"]:
                if sys.argv[2] in method.get("referenced_names",[]):
                    print(json.dumps({"hash":row["hash"],"filename":row["filename"],"method":method},ensure_ascii=False))
        return
    rows = {row["hash"]: row for row in inventory["modules"]}
    output = ROOT / "out/dis"
    for title, digest in HASHES.items():
        row = rows[digest]
        path = ROOT / "out/npk_scripts" / row["marshal_file"]
        code = Loader(path.read_bytes()).r_object()
        if len(sys.argv) > 2:
            if title != sys.argv[1]:
                continue
            pattern = sys.argv[2]
            def walk(c):
                name = c["name"].decode("utf-8") if isinstance(c["name"], bytes) else c["name"]
                if re.search(pattern, name):
                    disasm(c)
                    return
                for child in c["consts"]:
                    if isinstance(child, dict) and child.get("type") == "code":
                        walk(child)
            walk(code)
            continue
        with (output / ("social-" + title + "-native.asm")).open("w", encoding="utf-8", newline="\n") as handle:
            print("Android 1.0.128 原生证据", row["filename"], "源文件SHA256="+hashlib.sha256(path.read_bytes()).hexdigest(), file=handle)
            disasm(code, out=handle)
        print("已导出", title, row["filename"])

if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()

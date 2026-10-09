# -*- coding: utf-8 -*-
"""扫描本版全部原生模块和RPC静态契约；候选不是已证实缺陷或实机验收。"""
import ast
import datetime
import hashlib
import json
import re
import sys
from pathlib import Path

from mem_marshal_extract import Loader

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")


def decode(value):
    return value.decode("utf-8", errors="replace") if isinstance(value, bytes) else str(value)


def functions(code, path=""):
    name = path + "/" + decode(code["name"])
    yield name, code
    for child in code["consts"]:
        if isinstance(child, dict) and child.get("type") == "code":
            yield from functions(child, name)


def main():
    inventory = json.loads((ROOT / "out/npk_scripts/android_inventory.json").read_text(encoding="utf-8"))
    consumers = []
    count = 0
    for module in inventory["modules"]:
        code = Loader((ROOT / "out/npk_scripts" / module["marshal_file"]).read_bytes()).r_object()
        for name, function in functions(code):
            count += 1
            names = {decode(item) for item in function["names"]}
            variables = {decode(item) for item in function["varnames"]}
            if "sum" not in names or not ({"begin_time", "end_time"} & (names | variables)):
                continue
            consumers.append({
                "模块": module["filename"], "模块哈希": module["hash"],
                "函数": name, "起始行": function.get("firstlineno"),
                "调用时间API": "get_activity_time" in names,
                "直接读取结束字段": "end_time" in names,
                "相关调用": sorted(names & {"get_activity_time", "now_time", "sum", "get_time_count_down_end", "get_time_count_down_2"}),
                "状态": "待逐函数核验空值分支、常驻显示和阶段索引；不自动认定缺陷",
            })
    rpc_path = ROOT / "out/client_catalogs/rpc-catalog.json"
    rpc = json.loads(rpc_path.read_text(encoding="utf-8"))
    native_down = {row["方法"] for row in rpc["显式装饰方法"]}
    go_sources = {p: p.read_text(encoding="utf-8") for p in (ROOT / "internal/game").glob("*.go") if not p.name.endswith("_test.go")}
    downstream = {}
    for path, source in go_sources.items():
        for match in re.finditer(r'push\("(Avatar|Account)",\s*"([^"]+)"\s*[,)]', source):
            entity, method = match.groups()
            downstream.setdefault((entity, method), []).append({"文件": path.relative_to(ROOT).as_posix(), "行": source[:match.start()].count("\n") + 1})
    down_candidates = [{"实体": entity, "方法": method, "源码": locations,
                        "状态": "静态RPC目录未匹配；须核对动态注册，不能直接认定错误"}
                       for (entity, method), locations in sorted(downstream.items()) if method not in native_down]
    reference_names = set()
    for path in (ROOT / "_ref/GMA-Revive-Server-main").glob("*.py"):
        try:
            tree = ast.parse(path.read_text(encoding="utf-8-sig"))
        except (SyntaxError, UnicodeError):
            continue
        reference_names.update(item.name for item in ast.walk(tree) if isinstance(item, (ast.FunctionDef, ast.AsyncFunctionDef)))
    source_text = "\n".join(go_sources.values())
    # 从真正的客户端上行引用取集合，避免把参考辅助函数等同于缺失RPC。
    upstream = {}
    for row in rpc['上行代理引用']:
        upstream.setdefault(row['调用方法'], []).append({'模块': row['模块'], '函数': row['所在方法']})
    for row in rpc['命名转发调用']:
        upstream.setdefault(row['方法'], []).append({'模块': row['模块'], '函数': row['所在方法']})
    upstream_candidates = [
        {'方法': name, '客户端来源': locations,
         '状态': '当前Go未见同名字符串；须核对动态路由、目标实体及本版可达性，不能用空应答冒充实现'}
        for name, locations in sorted(upstream.items())
        if not re.search(r'["\x27]' + re.escape(name) + r'["\x27]', source_text)
    ]
    references = []
    for name in sorted(reference_names):
        if not re.search(r'["\x27]' + re.escape(name) + r'["\x27]', source_text):
            references.append(name)
    report = {
        "北京时间": datetime.datetime.now().astimezone().isoformat(),
        "范围": "Android1.0.128完整3099模块、当前Go实际推送、参考项目函数名；不更改角色或服务",
        "扫描模块": len(inventory["modules"]), "扫描函数及代码对象": count,
        "时间求和消费者": consumers, "静态下行未匹配候选": down_candidates,
        "真实客户端上行去重数": len(upstream), "上行未见Go字面量候选": upstream_candidates,
        "参考函数未见Go字面量候选": references,
        "边界": "静态候选只能确定复核范围；方法名不等于业务实现，函数存在不等于参数正确或功能通过。所有功能仍需协议、真实事务与MuMu分别验收。",
        "RPC目录SHA256": hashlib.sha256(rpc_path.read_bytes()).hexdigest(),
    }
    target = ROOT / "out/client-contract-audit-20261008.json"
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"扫描模块": report["扫描模块"], "扫描函数": count,
                      "时间消费者": len(consumers), "下行待核验": len(down_candidates),
                      "上行方法": len(upstream), "上行待核验": len(upstream_candidates),
                      "参考函数名待核验": len(references), "报告": str(target)}, ensure_ascii=False))


if __name__ == "__main__":
    main()

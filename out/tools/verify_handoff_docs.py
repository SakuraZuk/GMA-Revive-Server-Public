# -*- coding: utf-8 -*-
"""校验中文交接、链接和固定发布报告；不连接远端，不验证业务。"""
import hashlib
import json
from pathlib import Path
import re
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")


def load_report(name):
    return json.loads((ROOT / name).read_text(encoding="utf-8-sig"))


def main():
    docs = sorted(ROOT.rglob("*.md"))
    failures = []
    existing_reference_links = []
    inventory = []
    for doc in docs:
        try:
            raw = doc.read_bytes()
            source = raw.decode("utf-8-sig")
        except UnicodeError as error:
            failures.append(f"非UTF-8：{doc.relative_to(ROOT)}：{error}")
            continue
        if "\ufffd" in source:
            failures.append(f"存在替代乱码字符：{doc.relative_to(ROOT)}")
        inventory.append({"文件": str(doc.relative_to(ROOT)), "SHA256": hashlib.sha256(raw).hexdigest()})
        for link in re.findall(r"\]\(([^)]+)\)", source):
            link = link.strip().strip("<>")
            if "://" in link or link.startswith("#"):
                continue
            target = unquote(link.split("#", 1)[0])
            if not target:
                continue
            if not (doc.parent / target).exists():
                item = f"资料链接不存在：{doc.relative_to(ROOT)} -> {target}"
                if doc.is_relative_to(ROOT / "_ref"):
                    existing_reference_links.append(item)
                else:
                    failures.append(item)

    release = load_report("out/server-release.json")
    verification = load_report("out/remote-release-verification.json")
    hotfix = load_report("out/hotfix-bridge-release.json")
    current_hash = release["SHA256"]
    if verification["游戏二进制SHA256"] != current_hash:
        failures.append("固定发布与独立核验报告的游戏哈希不一致")
    for name in ("out/HANDOFF.md", "out/PROGRESS.md"):
        source = (ROOT / name).read_text(encoding="utf-8")
        for value in (current_hash, hotfix["SHA256"]):
            if value not in source:
                failures.append(f"最新固定发布哈希缺失：{name} -> {value}")
        if "PVP" not in source or "服务端单原生权威" not in source or not any(value in source for value in ("普通副本保持Android原生计算", "普通副本路线保留")):
            failures.append(f"最新PVP权威选择或普通副本原生路线缺失：{name}")
    tasks = (ROOT / "out/REPAIR-TASKS.md").read_text(encoding="utf-8")
    for group, count in (("A", 7), ("B", 7), ("C", 10), ("D", 9), ("E", 7), ("F", 6)):
        for index in range(1, count + 1):
            if re.search(rf"\| {group}{index}(?:\s|\|)", tasks) is None:
                failures.append(f"全量施工编号缺失：{group}{index}")
    if "F2已完成" not in tasks:
        failures.append("缺少F2排除施工决定")
    report = {
        "结果": "通过" if not failures else "失败",
        "文档数量": len(docs),
        "编码": "UTF-8；扫描替代乱码字符，不保证可自动识别所有历史错码",
        "游戏哈希": current_hash,
        "热更清单哈希": hotfix["SHA256"],
        "发布备份": release["备份"],
        "校验范围": "当前文档、资料链接与本地固定发布报告；未连接远端、未验证Go源码或MuMu玩法",
        "外部参考原有链接问题": existing_reference_links,
        "错误": failures,
        "文档清单": inventory,
    }
    (ROOT / "out/handoff-doc-check.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({key: value for key, value in report.items() if key != "文档清单"}, ensure_ascii=False, indent=2))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())

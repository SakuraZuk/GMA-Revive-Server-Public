# -*- coding: utf-8 -*-
"""审核Git当前版本中的隐私，仅输出文件和问题类型，不输出匹配的秘密。"""
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    names = subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT).decode("utf-8").split("\0")
    private_values = []
    try:
        from local_ops_credentials import load_target
        for target in ("game", "hotfix"):
            host, _, _, password = load_target(target)
            private_values.extend((host, password))
    except (RuntimeError, KeyError, OSError):
        pass  # 新电脑无需拥有发布者授权；仍执行固定秘密及路径扫描。
    failures = []
    count = size = 0
    for name in filter(None, names):
        path = ROOT / name
        if not path.is_file():
            failures.append({"文件": name, "问题": "索引文件不存在"})
            continue
        raw = path.read_bytes()
        count += 1
        size += len(raw)
        try:
            body = raw.decode("utf-8-sig")
        except UnicodeError:
            failures.append({"文件": name, "问题": "未审核的二进制文件"})
            continue
        reasons = []
        if any(value and value in body for value in private_values):
            reasons.append("真实部署地址或口令")
        if re.search(r"(?:github_pat_|ghp_)[A-Za-z0-9_]{20,}", body):
            reasons.append("访问令牌")
        if re.search(r"(?:^|\n)-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----", body):
            reasons.append("私钥")
        if re.search(r"(?<![A-Za-z])[A-Za-z]:[\\/](?!n)", body):
            reasons.append("机器绝对路径")
        if re.search(r"(?:Users[/\\]+[^/\\\s]+|@(?:qq|163)\.com)", body):
            reasons.append("个人目录或邮箱")
        if any(part in path.relative_to(ROOT).parts for part in ("runtime", "evidence", "files", "work", "keys", "__pycache__")):
            reasons.append("本机资源或数据目录")
        for reason in reasons:
            failures.append({"文件": name, "问题": reason})
    report = {"状态": "通过" if not failures else "失败", "文件数量": count,
              "字节数": size, "问题": failures,
              "范围": "当前Git文件；不证明旧提交、分支、标签或GitHub缓存已清理"}
    (ROOT / "out/github-public-source-audit.json").write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return bool(failures)


if __name__ == "__main__":
    raise SystemExit(main())

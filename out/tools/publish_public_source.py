# -*- coding: utf-8 -*-
"""导出当前服务端树到独立公开Git历史；不推送原仓库历史。"""
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile

ROOT = Path(__file__).resolve().parents[2]
URL = "https://github.com/SakuraZuk/GMA-Revive-Server-Public.git"


def git(args, cwd=ROOT, capture=True):
    result = subprocess.run(["git", *args], cwd=cwd, check=True,
                            stdout=subprocess.PIPE if capture else None,
                            stderr=subprocess.PIPE if capture else None)
    return result.stdout if capture else b""


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    subprocess.run([__import__('sys').executable, str(ROOT / "out/tools/audit_public_source.py")],
                   cwd=ROOT, check=True)
    if git(["status", "--porcelain"]).strip():
        raise RuntimeError("公开前先提交审核后的工作区，不能导出未确定版本")
    # 仓库外的独立检出不包含私有Git历史，也不递归扫描本机凭据父目录。
    base = Path(os.environ.get("LOCALAPPDATA", str(Path.home() / ".local/share")))
    checkout = base / "hs-server" / "github-public"
    checkout.mkdir(parents=True, exist_ok=True)
    expected = checkout.resolve()
    if not (checkout / ".git").exists():
        git(["init", "-b", "main"], cwd=checkout)
        git(["remote", "add", "origin", URL], cwd=checkout)
    if git(["remote", "get-url", "origin"], cwd=checkout).decode().strip() != URL:
        raise RuntimeError("公开检出远端与预期仓库不符")
    git(["config", "credential.helper", "manager"], cwd=checkout)
    git(["config", "http.proxy", ""], cwd=checkout)
    git(["config", "user.name", "SakuraZuk"], cwd=checkout)
    git(["config", "user.email", "SakuraZuk@users.noreply.github.com"], cwd=checkout)
    if git(["status", "--porcelain"], cwd=checkout).strip():
        raise RuntimeError("公开检出有未提交修改，禁止覆盖")
    refs = git(["ls-remote", "origin", "refs/heads/main"], cwd=checkout).strip()
    if refs:
        git(["fetch", "origin", "main"], cwd=checkout)
        git(["merge", "--ff-only", "origin/main"], cwd=checkout)
    archive = git(["archive", "--format=tar", "HEAD"])
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        names = {entry.name for entry in tar.getmembers() if entry.isfile()}
        for name in filter(None, git(["ls-files", "-z"], cwd=checkout).decode().split("\0")):
            path = checkout / name
            if not path.resolve().is_relative_to(expected):
                raise RuntimeError("公开检出路径越界")
            if name not in names and path.is_file():
                path.unlink()  # 只移除独立公开检出中被源清单淘汰的已跟踪文件。
        tar.extractall(checkout, filter="data")
    git(["-c", "core.safecrlf=false", "add", "-A"], cwd=checkout)
    if git(["diff", "--cached", "--name-only"], cwd=checkout).strip():
        git(["commit", "-m", "发布服务端源码、规则、测试与中文资料（隐私清理版）"], cwd=checkout)
    git(["push", "-u", "origin", "main"], cwd=checkout, capture=False)
    local = git(["rev-parse", "HEAD"], cwd=checkout).decode().strip()
    remote = git(["ls-remote", "origin", "refs/heads/main"], cwd=checkout).decode().split()[0]
    if local != remote:
        raise RuntimeError("公开仓库提交核验不一致")
    report = {"状态": "公开源码推送核验通过", "仓库": URL, "提交": local,
              "文件数量": len(names), "历史策略": "独立公开历史，不携带私有仓库旧提交"}
    (ROOT / "out/github-public-upload-report.json").write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()

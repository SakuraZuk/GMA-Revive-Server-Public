# -*- coding: utf-8 -*-
"""停止所属临时预览、清一次性令牌，记录源码/文档清单；不改生产系统。"""
import csv
import ctypes
import datetime
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
expected = (ROOT / "out/bin/admin-preview.exe").resolve()
rows = subprocess.run(["tasklist", "/FI", "IMAGENAME eq admin-preview.exe", "/FO", "CSV", "/NH"], capture_output=True, check=True).stdout.decode("mbcs")
terminated = []
kernel = ctypes.WinDLL("kernel32", use_last_error=True)
kernel.OpenProcess.argtypes = [ctypes.c_uint32, ctypes.c_int, ctypes.c_uint32]
kernel.OpenProcess.restype = ctypes.c_void_p
kernel.QueryFullProcessImageNameW.argtypes = [ctypes.c_void_p, ctypes.c_uint32, ctypes.c_wchar_p, ctypes.POINTER(ctypes.c_uint32)]
kernel.CloseHandle.argtypes = [ctypes.c_void_p]
for row in csv.reader(io.StringIO(rows)):
    if not row or row[0].lower() != "admin-preview.exe":
        continue
    pid = int(row[1])
    handle = kernel.OpenProcess(0x1000, False, pid)
    if not handle:
        raise OSError(ctypes.get_last_error(), "不能核对临时预览进程归属")
    try:
        buf = ctypes.create_unicode_buffer(32768)
        length = ctypes.c_uint32(len(buf))
        if not kernel.QueryFullProcessImageNameW(handle, 0, buf, ctypes.byref(length)):
            raise OSError(ctypes.get_last_error(), "读取预览程序路径失败")
        actual = Path(buf.value).resolve()
        if actual != expected:
            raise RuntimeError("同名预览程序不在项目所属路径，拒绝停止")
    finally:
        kernel.CloseHandle(handle)
    subprocess.run(["taskkill", "/PID", str(pid), "/F"], check=True, capture_output=True)
    terminated.append(pid)
token_file = (ROOT / "out/admin-preview-state.json").resolve()
assert token_file.parent == (ROOT / "out").resolve()
token_file.unlink(missing_ok=True)

files = set(ROOT.rglob("*.md"))
for directory in ("cmd", "internal", "deploy", "out/tools"):
    for path in (ROOT / directory).rglob("*"):
        if path.is_file() and path.suffix in {".go", ".py", ".sh", ".cmd", ".sql", ".json", ".env", ".conf"}:
            files.add(path)
files.update([ROOT / "go.mod", ROOT / "go.sum"])
def evidence_state(relative):
    path=ROOT/relative
    if not path.exists(): return "尚无当次报告"
    report=json.loads(path.read_text(encoding="utf-8"))
    return {key:report[key] for key in ("状态","退出码","PASS数量","SKIP数量","源码检查期间一致","源码构建期间一致") if key in report}

manifest = {"生成时间": datetime.datetime.now().isoformat(timespec="seconds"),
    "范围": "cmd/internal/deploy/out/tools中的源码/目录及全部MD与go.mod/go.sum；仅文件SHA，不复制配置内容或口令",
    "状态": "源码、正式构建、真实PG及生产发布分开记录；报告成功不等于当前源码或Android全验收；生产见HANDOFF",
    "当次检查": evidence_state("out/continuation-go-checks.json"),
    "当次构建": evidence_state("out/completion-build.json"),
    "当次真实PG": evidence_state("out/remote-database-verification.json"),
    "临时预览停止PID": terminated, "一次性令牌文件": "已删除",
    "文件": [{"路径": str(path.relative_to(ROOT)).replace("\\", "/"), "SHA256": hashlib.sha256(path.read_bytes()).hexdigest()} for path in sorted(files)]}
(ROOT / "out/completion-source-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"状态": "临时预览和令牌清理完成，源码清单已保存", "文件数": len(files), "停止PID": terminated}, ensure_ascii=False))

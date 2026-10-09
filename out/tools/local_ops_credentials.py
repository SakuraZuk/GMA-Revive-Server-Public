# -*- coding: utf-8 -*-
"""读取本机加密部署配置；仓库不保存服务器口令，Linux可使用环境变量。"""
import ctypes
from ctypes import wintypes
import json
import os
from pathlib import Path


class _Blob(ctypes.Structure):
    _fields_ = [("size", wintypes.DWORD), ("data", ctypes.POINTER(ctypes.c_ubyte))]


def _crypt(raw, encrypt):
    if os.name != "nt":
        raise RuntimeError("本机加密配置仅支持Windows，其他系统请设置部署环境变量")
    buffer = (ctypes.c_ubyte * len(raw)).from_buffer_copy(raw)
    source = _Blob(len(raw), buffer)
    target = _Blob()
    crypt = ctypes.WinDLL("crypt32", use_last_error=True)
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.LocalFree.argtypes = [ctypes.c_void_p]
    kernel.LocalFree.restype = ctypes.c_void_p
    if encrypt:
        call = crypt.CryptProtectData
        call.argtypes = [ctypes.POINTER(_Blob), wintypes.LPCWSTR, ctypes.c_void_p,
                         ctypes.c_void_p, ctypes.c_void_p, wintypes.DWORD, ctypes.POINTER(_Blob)]
        ok = call(ctypes.byref(source), "幻书本机部署配置", None, None, None, 1, ctypes.byref(target))
    else:
        call = crypt.CryptUnprotectData
        call.argtypes = [ctypes.POINTER(_Blob), ctypes.c_void_p, ctypes.c_void_p,
                         ctypes.c_void_p, ctypes.c_void_p, wintypes.DWORD, ctypes.POINTER(_Blob)]
        ok = call(ctypes.byref(source), None, None, None, None, 1, ctypes.byref(target))
    call.restype = wintypes.BOOL
    if not ok:
        raise RuntimeError("本机部署配置加密或解密失败")
    try:
        return ctypes.string_at(target.data, target.size)
    finally:
        kernel.LocalFree(ctypes.cast(target.data, ctypes.c_void_p))


def config_path():
    return Path(os.environ["LOCALAPPDATA"]) / "hs-server" / "operations.dpapi"


def save_profiles(profiles):
    """迁移时调用；仅保存DPAPI密文，不输出配置值。"""
    path = config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(_crypt(json.dumps(profiles).encode("utf-8"), True))


def load_target(target):
    prefix = "HS_" + target.upper() + "_DEPLOY_"
    names = ["HOST", "PORT", "USER", "PASSWORD"]
    if all(os.environ.get(prefix + name) for name in names):
        values = [os.environ[prefix + name] for name in names]
        return values[0], int(values[1]), values[2], values[3]
    if os.name == "nt" and config_path().is_file():
        profiles = json.loads(_crypt(config_path().read_bytes(), False))
        values = profiles[target]
        return values["host"], int(values["port"]), values["user"], values["password"]
    raise RuntimeError("缺少本机部署配置，请设置" + prefix + "HOST/PORT/USER/PASSWORD")

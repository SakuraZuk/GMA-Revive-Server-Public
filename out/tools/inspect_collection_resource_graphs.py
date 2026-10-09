# -*- coding: utf-8 -*-
"""以本版 ARM 已证索引公式，只读扫描收藏室教学 graph 原始资源。"""
import hashlib
import json
import struct
import sys
import zlib
from pathlib import Path
import lz4.block

sys.stdout.reconfigure(encoding="utf-8")
root = Path(__file__).resolve().parents[2]
dest = root / "out/collection-resource-evidence"
dest.mkdir(exist_ok=True)
report = []
for name in ["work/apk_assets_extra/res.npk", "com.netease.hsqsl/files/netease/h62/Documents/res.npk"]:
    path = root / name
    data = path.read_bytes()
    if data[:4] != b"NXPK":
        raise ValueError("资源包头无效")
    count, index = struct.unpack_from("<I", data, 4)[0], struct.unpack_from("<I", data, 20)[0]
    hits, errors, spans = [], [], []
    for i in range(count):
        key, offset, stored, decoded, h64, codec = struct.unpack_from("<IIIIQI", data, index + i * 28)
        if codec >= 62:
            offset = ((-100 - decoded) & 0xffffffff) ^ offset ^ 0x7a090d89
            codec -= 62
        if offset < 24 or offset + stored > index:
            raise ValueError("本版原生索引解码后范围无效")
        spans.append((offset, offset + stored))
        raw = data[offset:offset + stored]
        try:
            content = lz4.block.decompress(raw, uncompressed_size=decoded) if codec == 2 else zlib.decompress(raw) if codec == 1 else raw if codec == 0 else None
            if content is None or len(content) != decoded:
                raise ValueError("未知编码或长度不符")
        except Exception as exc:
            errors.append({"条目": f"{key:08X}", "编码": codec, "错误": str(exc)})
            continue
        if any(word in content for word in [b"GuideFacilityAcc", b"guide_trophyroom_facility_production", b"acc_second"]):
            target = dest / (path.parent.name + "-" + f"{key:08X}" + ".bin")
            target.write_bytes(content)
            item = {"条目": f"{key:08X}", "原始资源": str(target.relative_to(root)), "SHA256": hashlib.sha256(content).hexdigest(), "长度": len(content)}
            try:
                obj = json.loads(content)
                def visit(v, location=""):
                    if isinstance(v, dict):
                        if "acc_second" in v:
                            item.setdefault("加速节点",[]).append({"位置":location,"数据":v})
                        for k,x in v.items(): visit(x,location+"/"+str(k))
                    elif isinstance(v,list):
                        for k,x in enumerate(v):visit(x,location+"/"+str(k))
                visit(obj)
            except (ValueError,UnicodeDecodeError): pass
            hits.append(item)
    spans.sort()
    if any(a[1] > b[0] for a,b in zip(spans,spans[1:])):
        raise ValueError("解码后索引资源重叠")
    item = {"资源包": name, "SHA256": hashlib.sha256(data).hexdigest(), "扫描数量": count, "命中": hits, "解压错误": errors, "索引解码证据":"out/dis/remaining-collection-npk-native.asm"}
    report.append(item)
    print(json.dumps({"资源包": name, "扫描数量": count, "命中": hits, "错误数量": len(errors), "错误首项": errors[:1]}, ensure_ascii=False))
(root / "out/collection-resource-scan.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

# -*- coding: utf-8 -*-
"""独立核对纯 Python 输出与 PC 原解码器，保存可复现的证据报告。"""
import hashlib
import json
from collections import Counter
from pathlib import Path

from mem_marshal_extract import Loader, MarshalError
from npk_unpack import decode_entry, parse_archive
from uni_lz4 import EXE, lz4_call

ROOT = Path(__file__).resolve().parents[2]
ARCHIVES = {
    'android_base': ROOT/'work/apk/assets/script.npk',
    'android_patch': ROOT/'files/netease/h62/Documents/script.npk',
    'pc_base': ROOT/'prototype/neteasehsqsl/script.npk',
}


def main():
    rows = []
    for name, path in ARCHIVES.items():
        entries, data = parse_archive(path)
        prefixes = Counter()
        complete_marshal = 0
        for entry in entries:
            result = decode_entry(entry, data)
            prefixes[result[:2].hex()] += 1
            # 这是既有解析器的测试结果，不代表所有非匹配对象都是加密。
            try:
                loader = Loader(result)
                obj = loader.r_object()
                complete_marshal += int(loader.p == len(result) and isinstance(obj, dict)
                                        and obj.get('type') == 'code')
            except (MarshalError, ValueError, TypeError, IndexError, RecursionError):
                pass
        ordered = sorted(entries, key=lambda e:e['stored_size'])
        samples = [ordered[0], ordered[len(ordered)//2], ordered[-1]]
        # 覆盖 f1 首字节和 PC 追加条目，不能根据首字节猜存储类型。
        for condition in [lambda e:data[e['offset']] != 0xf0,
                          lambda e:e['after_index']]:
            match = next((e for e in entries if condition(e)), None)
            if match and match not in samples:
                samples.append(match)
        cases = []
        for entry in samples:
            start = entry['offset']
            raw = data[start:start+entry['stored_size']]
            expected = decode_entry(entry, data)
            actual = lz4_call(raw, entry['decoded_size'], len(raw))
            if actual != expected:
                raise AssertionError('原函数与 Python 解码不一致：%08X' % entry['hash'])
            cases.append(dict(hash='%08X' % entry['hash'], stored_size=len(raw),
                              decoded_size=len(actual), raw_prefix=raw[:4].hex(),
                              sha256=hashlib.sha256(actual).hexdigest(),
                              exact_match=True))
        row = dict(archive=name, source=str(path), count=len(entries),
                   source_sha256=hashlib.sha256(data).hexdigest(),
                   decoded_prefixes=dict(prefixes), existing_marshal_loader_complete=complete_marshal,
                   blocks_after_index=sum(e['after_index'] for e in entries), oracle_samples=cases)
        rows.append(row)
        print('%s：全部解压 %d，原函数逐字节核对 %d，既有 marshal 解析器完整对象 %d' %
              (name,len(entries),len(cases),complete_marshal))
        print('  负载两字节前缀：',dict(prefixes))
    report=dict(exe_sha256=hashlib.sha256(Path(EXE).read_bytes()).hexdigest(),
                decoder_rva='0xf09310',decoder_va='0x1309310',archives=rows)
    (ROOT/'out/npk_decoded/verification.json').write_text(
        json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')


if __name__ == '__main__':
    main()

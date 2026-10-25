# -*- coding: utf-8 -*-
"""核验旧生成器会漏掉的编码偏移，以及损坏资源必须拒绝的边界。"""
import hashlib
import struct
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import gen_npkfilelist_md5 as generator


class ChecksumGeneratorTest(unittest.TestCase):
    def test_encoded_offset_and_original_bytes(self):
        payload = b'original-block'
        offset = 24 ^ (((-100-len(payload)) & 0xffffffff) ^ 0x7a090d89)
        header = b'NXPK' + struct.pack('<IIIII', 1, 0, 0, 0, 24+len(payload))
        data = header + payload + struct.pack('<IIIIIII', 17, offset, len(payload), len(payload), 0, 0, 62)
        pairs, report = generator.block_md5s(data)
        self.assertEqual(pairs, [17, hashlib.md5(payload).hexdigest()])
        self.assertEqual(report['编码偏移条目'], 1)
        with self.assertRaises(ValueError):
            generator.block_md5s(data[:-1])
        damaged = bytearray(data)
        struct.pack_into('<I', damaged, 24+len(payload)+8, 999999)
        with self.assertRaises(ValueError):
            generator.block_md5s(damaged)

    def test_matching_hash_overrides_documents_priority(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docs, cache = root/'documents', root/'cache'
            docs.mkdir()
            cache.mkdir()
            (docs/'scene.npk').write_bytes(b'wrong-source')
            (cache/'scene.npk').write_bytes(b'right-source')
            with patch.multiple(generator, ROOT=root, DOCS=docs, CACHE=cache, APK=root/'missing.apk'):
                data, source = generator.matching_source('scene', 12, hashlib.md5(b'right-source').hexdigest())
                self.assertEqual(data, b'right-source')
                self.assertIn('cache', source)
                with self.assertRaises(ValueError):
                    generator.matching_source('scene', 12, '0'*32)
                with self.assertRaises(ValueError):
                    generator.matching_source('../scene', 12, '0'*32)


if __name__ == '__main__':
    unittest.main()

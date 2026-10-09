# -*- coding: utf-8 -*-
"""覆盖已知 LZ4 向量和截断边界；不运行不受信任的脚本负载。"""
import unittest
from npk_unpack import DecodeError, lz4_decompress


class LZ4Tests(unittest.TestCase):
    def test_literal(self):
        self.assertEqual(lz4_decompress(b'\x50hello',5),b'hello')

    def test_extended_literal(self):
        self.assertEqual(lz4_decompress(b'\xf0\x01'+b'a'*16,16),b'a'*16)

    def test_overlapping_match(self):
        self.assertEqual(lz4_decompress(b'\x14a\x01\x00\x50hello',14),b'a'*9+b'hello')

    def test_final_low_nibble_does_not_imply_match(self):
        self.assertEqual(lz4_decompress(b'\x51hello',5),b'hello')

    def test_truncation_and_boundaries(self):
        for src, expected in [(b'\xf0',16),(b'\x50hell',5),
                              (b'\x50hello',4),(b'\x10a\x00\x00',5),
                              (b'\x10a\x02\x00',5),(b'\x10a\x01',5),
                              (b'\x1fa\x01\x00',20)]:
            with self.subTest(src=src):
                with self.assertRaises(DecodeError):
                    lz4_decompress(src,expected)


if __name__ == '__main__':
    unittest.main()

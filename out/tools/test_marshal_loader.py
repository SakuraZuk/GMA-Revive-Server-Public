# -*- coding: utf-8 -*-
"""使用明确的 Python 2 marshal 标量编码验证旧 Loader 修正。"""
import struct
import unittest
from mem_marshal_extract import Loader,MarshalError


class MarshalTests(unittest.TestCase):
    def load(self,raw):
        loader=Loader(raw)
        result=loader.r_object()
        self.assertEqual(loader.p,len(raw))
        return result

    def test_negative_int64(self):
        self.assertEqual(self.load(b'I'+struct.pack('<q',-2)),-2)

    def test_negative_long_with_15_bit_limbs(self):
        raw=b'l'+struct.pack('<iHH',-2,3,2)
        self.assertEqual(self.load(raw),-(3+2*(1<<15)))

    def test_text_float(self):
        self.assertEqual(self.load(b'f\x041.25'),1.25)

    def test_truncated_and_invalid_limb(self):
        for raw in [b'f\x041.2',b'l'+struct.pack('<i',2)+b'\0\0',
                    b'l'+struct.pack('<iH',1,32768)]:
            with self.subTest(raw=raw):
                with self.assertRaises(MarshalError):
                    self.load(raw)


if __name__=='__main__':
    unittest.main()

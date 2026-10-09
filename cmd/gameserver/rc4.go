// 标准 RC4（与 libclient.so 的 async::arc4_crypter 对应，out/gate-protocol-spec.md）。
// KSA+PRGA 无 drop；若实测存在丢弃字节可在联调时调整。
package main

type rc4 struct {
	s    [256]byte
	i, j byte
}

func newRC4(key []byte) *rc4 {
	c := &rc4{}
	for k := range c.s {
		c.s[k] = byte(k)
	}
	var j byte
	for k := 0; k < 256; k++ {
		j += c.s[k] + key[k%len(key)]
		c.s[k], c.s[j] = c.s[j], c.s[k]
	}
	return c
}

func (c *rc4) xor(dst, src []byte) {
	for n := range src {
		c.i++
		c.j += c.s[c.i]
		c.s[c.i], c.s[c.j] = c.s[c.j], c.s[c.i]
		dst[n] = src[n] ^ c.s[c.s[c.i]+c.s[c.j]]
	}
}

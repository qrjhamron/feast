package conn

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// NewCFB8 returns encryption and decryption streams for Minecraft-style AES/CFB8.
//
// The provided key is used as both AES key and IV.
func NewCFB8(key []byte) (encrypt cipher.Stream, decrypt cipher.Stream, err error) {
	if len(key) != 16 {
		return nil, nil, fmt.Errorf("invalid key length %d: expected 16", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	iv := make([]byte, len(key))
	copy(iv, key)
	iv2 := make([]byte, len(key))
	copy(iv2, key)
	return newCFB8(block, iv, true), newCFB8(block, iv2, false), nil
}

type cfb8Stream struct {
	block   cipher.Block
	iv      []byte
	encrypt bool
	tmp     []byte
}

func newCFB8(block cipher.Block, iv []byte, encrypt bool) cipher.Stream {
	return &cfb8Stream{block: block, iv: append([]byte(nil), iv...), encrypt: encrypt, tmp: make([]byte, block.BlockSize())}
}

func (s *cfb8Stream) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("cfb8: output smaller than input")
	}
	for i := range src {
		s.block.Encrypt(s.tmp, s.iv)
		out := src[i] ^ s.tmp[0]
		dst[i] = out

		copy(s.iv, s.iv[1:])
		if s.encrypt {
			s.iv[len(s.iv)-1] = out
		} else {
			s.iv[len(s.iv)-1] = src[i]
		}
	}
}

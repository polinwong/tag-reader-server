// 3D Artefact Exhibition backend
// File: verify_base.go, NFC tag verify for NTAG 424 DNA
// Creater: Kevin Mak, Sep 2021
// (c)Marvel Digital Ltd. 2021
package card

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"log"
	"math"
	"math/big"
	"strconv"
)

// LRP is the implement struct for golang,
// * AN12304: Leakage Resilient Primitive (LRP) Specification
// * sdm-backend: https://github.com/icedevml/sdm-backend
type LRP struct {
	key  []byte
	u    int
	r    []byte
	pad  bool
	text bool

	p  [][]byte
	ku [][]byte
	kp []byte
}

type AESECB struct {
	cipher.BlockMode
}

func NewLRPString(key []byte, u int, r string, pad bool) *LRP {
	l := NewLRP(key, u, make([]byte, 0), pad)
	l.text = true
	for _, v := range r {
		l.r = append(l.r, byte(v))
	}
	return l
}

func NewLRP(key []byte, u int, r []byte, pad bool) *LRP {
	if r == nil {
		r = MakeBytes(16, 0x00)
	}

	l := new(LRP)
	l.key = key
	l.u = u
	l.r = r
	l.pad = pad

	l.p = generatePlaintexts(key)
	l.ku = generateUpdateKeys(key)
	l.kp = l.ku[l.u]

	return l
}

// LRICB decrypt and update counter (LRICBDecs)
// param data: ciphertext
// return: plaintext
func (l *LRP) Decrypt(data []byte) (ret []byte) {
	buf := bytes.NewBuffer(data)
	outBuf := bytes.Buffer{}

	for {
		block := buf.Next(aes.BlockSize)
		if len(block) == 0 {
			break
		}

		y := l.evalLRP(l.p, l.kp, l.r, true)
		pt := d(y, block)
		outBuf.Write(pt)
		l.r = incrCounter(l.r)
	}

	if l.pad {
		ret = PKCS7UnPadding(outBuf.Bytes())
	} else {
		ret = outBuf.Bytes()
	}

	return
}

// AES-ECB block encryption
func e(k, v []byte) []byte {
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil
	}
	var data []byte
	if len(v) != 16 {
		data = PKCS7Padding(v, b.BlockSize())
	} else {
		data = v
	}
	c := make([]byte, len(data))
	size := b.BlockSize()

	for bs, be := 0, size; bs < len(data); bs, be = bs+size, be+size {
		b.Encrypt(c[bs:be], data[bs:be])
	}

	return c
}

// AES-ECB block decryption
func d(k, v []byte) []byte {
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil
	}
	d := make([]byte, len(v))
	size := b.BlockSize()

	for bs, be := 0, size; bs < len(v); bs, be = bs+size, be+size {
		b.Decrypt(d[bs:be], v[bs:be])
	}

	return d
}

func nibbles(x []byte, text bool) []byte {
	var s string
	if text {
		s = string(x)
	} else {
		s = hex.EncodeToString(x)
	}
	buf := make([]byte, 0)
	for _, v := range s {
		nv, err := strconv.ParseInt(string(v), 16, 64)
		if err != nil {
			return nil
		}
		buf = append(buf, byte(nv))
	}
	return buf
}

func incrCounter(r []byte) (fin []byte) {
	buf := bytes.NewReader(r)
	var tr uint32
	err := binary.Read(buf, binary.BigEndian, &tr)
	if err != nil {
		log.Printf("incrCounter error with: %s", err)
		return
	}
	tr++

	fin = make([]byte, 4)
	binary.BigEndian.PutUint32(fin, tr)
	return
}

func (l *LRP) evalLRP(p [][]byte, kp []byte, x []byte, final bool) []byte {
	y := kp

	ni := nibbles(x, l.text)
	// log.Printf("ni: %s", hex.EncodeToString(ni))
	for _, xi := range ni {
		pj := p[xi]
		y = e(y, pj)
	}

	if final {
		y = e(y, MakeBytes(16, 0x00))
	}
	// log.Printf("ni enc: %s", hex.EncodeToString(y))

	return y
}

// The irreducible polynomial
var polynomial = func() *big.Int {
	f := big.NewInt(1 + 2 + 4 + 128)
	e := big.NewInt(2)
	e.Exp(e, big.NewInt(128), nil)
	f.Add(f, e)
	return f
}()

func (l *LRP) cmac(data []byte) []byte {
	buf := bytes.NewBuffer(data)

	mulPoly := func(f1, f2 []byte) []byte {
		z := new(big.Int).SetBytes(polynomial.Bytes())
		for len(f2) > 16 {
			// log.Printf("pol: %s", hex.EncodeToString(z.Bytes()))
			fin := new(big.Int).SetBytes(f2)
			if z.Bytes()[0]&fin.Bytes()[0] == 0 {
				z = new(big.Int).Lsh(polynomial, 1)
				continue
			}
			fin.Xor(fin, z)
			// log.Printf("fin: %s", hex.EncodeToString(fin.Bytes()))

			z = new(big.Int).Lsh(polynomial, 1)
			f2 = fin.Bytes()
		}
		return f2
	}

	mul := func(i []byte) (k1, k2 []byte) {
		bk0 := new(big.Int).SetBytes(i)
		bk1, bk2 := big.NewInt(0), big.NewInt(0)
		bk1 = bk1.Mul(new(big.Int).SetBytes(i), big.NewInt(2))
		bk2 = bk2.Mul(new(big.Int).SetBytes(i), big.NewInt(4))

		k1 = mulPoly(bk0.Bytes(), bk1.Bytes())
		k2 = mulPoly(bk0.Bytes(), bk2.Bytes())
		// log.Printf("k0: %s", hex.EncodeToString(k0))
		return
	}
	bk0 := l.evalLRP(l.p, l.kp, MakeBytes(16, 0x00), true)
	k1, k2 := mul(bk0)

	// log.Printf("k1: %s", hex.EncodeToString(k1))
	// log.Printf("k2: %s", hex.EncodeToString(k2))

	y := MakeBytes(aes.BlockSize, 0x00)
	var x []byte
	var bCount = 0
	for {
		x = buf.Next(aes.BlockSize)
		bCount += aes.BlockSize

		if len(x) < aes.BlockSize || buf.Len() == 0 {
			break
		}
		// PICC will not do following work
		y = xor(x, y)
		y = l.evalLRP(l.p, l.kp, y, true)
	}

	padBytes := 0
	if len(x) < aes.BlockSize {
		padBytes = aes.BlockSize - len(x)
		buf := bytes.NewBuffer(x)
		buf.Write([]byte{0x80})
		for i := 0; i < padBytes-1; i++ {
			buf.Write([]byte{0x00})
		}
		x = buf.Bytes()
	}
	y = xor(x, y)

	if padBytes == 0 {
		y = xor(y, k1)
	} else {
		y = xor(y, k2)
	}
	ret := l.evalLRP(l.p, l.kp, y, true)
	// log.Printf("ret: %s", hex.EncodeToString(ret))
	return ret
}

func generatePlaintexts(k []byte) [][]byte {
	m := 4.0

	h := k
	h = e(h, MakeBytes(16, 0x55))
	p := make([][]byte, 0)
	// log.Printf("first key: %s", hex.EncodeToString(h))

	for i := 0; i < int(math.Pow(2, m)); i++ {
		c := e(h, MakeBytes(16, 0xAA))
		// log.Printf("p%d base key: %s", i, hex.EncodeToString(c))
		p = append(p, c)
		h = e(h, MakeBytes(16, 0x55))
		// log.Printf("p%d cipher: %s", i, hex.EncodeToString(h))
	}
	// log.Printf("\n")
	return p
}

func generateUpdateKeys(k []byte) [][]byte {
	q := 4

	h := k
	h = e(h, MakeBytes(16, 0xaa))
	uk := make([][]byte, 0)
	// log.Printf("first key: %s", hex.EncodeToString(h))

	for i := 0; i < q; i++ {
		c := e(h, MakeBytes(16, 0xAA))
		// log.Printf("k%d base key: %s", i, hex.EncodeToString(c))
		uk = append(uk, c)
		h = e(h, MakeBytes(16, 0x55))
		// log.Printf("k%d cipher: %s", i, hex.EncodeToString(h))
	}
	// log.Printf("\n")
	return uk
}

func xor(data1, data2 []byte) (output []byte) {
	if len(data1) != len(data2) {
		return nil
	}
	output = make([]byte, len(data1))

	for i := range data1 {
		output[i] = data1[i] ^ data2[i]
	}
	return
}

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	paddingSize := blockSize - len(ciphertext)%blockSize
	buf := bytes.NewBuffer(ciphertext)
	buf.WriteByte(0x80)
	buf.Write(MakeBytes(paddingSize-1, 0x00))
	return buf.Bytes()
}

func PKCS7UnPadding(origData []byte) []byte {
	padl := 0
	for i := len(origData) - 1; i >= 0; i-- {
		padl++
		if origData[i] == 0x80 {
			break
		}
		if origData[i] != 0x00 {
			return nil
		}
	}
	return origData[:len(origData)-padl]
}

func MakeBytes(size int, defaultVal byte) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = defaultVal
	}
	return b
}

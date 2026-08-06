// 3D Artefact Exhibition backend
// File: verify_base.go, NFC tag verify for NTAG 424 DNA
// Created by: Kevin Mak, Sep 2021
// (c)Marvel Digital Ltd. 2021

package card

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"strings"

	"github.com/aead/cmac"
)

var (
	ErrVerifySunRead     = errors.New("read uid & ctr error")
	ErrVerifySunNotMatch = errors.New("verify error, not match")

	ErrVerifySunCtrRepeated = errors.New("the CTR is repeated")

	LastErrorID string

	AppMasterKey []byte // Application master key, generated/loaded at startup
	SdmMetaKey   []byte // SDM meta read key, generated/loaded at startup

	sdmTestKey = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	} // Debug key, don't use debug mode on deploy server
)

// VerifyCard the verify function referenced by:
// * AN12196: NTAG 424 DNA and NTAG 424 DNA TagTamper features and hints
// * sdm-backend: https://github.com/icedevml/sdm-backend
// Notice: Used external CMAC lib github.com/aead/cmac
// The struct only implemented AES, SDM MAC verify in plaintext and Encrypted PICC data
// Not provide SMDFileData, LRP mode encrypt & TagTamper function
type VerifyCard struct {
	PublicKeyHexRaw string
	PublicKey       ecdsa.PublicKey
}

func MakeVerifyCard(dbPath string) (card *VerifyCard) {
	card = new(VerifyCard)
	card.genPublicKey()
	MakeAppMasterKeyFile(dbPath)
	MakeMetaKeyFile(dbPath)

	return card
}

func MakeAppMasterKeyFile(targetPath string) {
	AppMasterKey = loadOrCreateAESKey(targetPath + "/appmasterkey")
}

func MakeMetaKeyFile(targetPath string) {
	SdmMetaKey = loadOrCreateAESKey(targetPath + "/metakey")
}

func loadOrCreateAESKey(keyPath string) []byte {
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		panic("must create key file for NFC NTAG424DNA")
	}
	defer keyFile.Close()

	s, _ := keyFile.Stat()
	if s.Size() == 0 {
		raw := MakeBytes(16, 0)
		if n, err := rand.Read(raw); err != nil || n != 16 {
			panic("create key failed")
		}
		if n, err := keyFile.Write(raw); err != nil || n != 16 {
			panic("write key failed")
		}
		return raw
	} else if s.Size() == 16 {
		raw := MakeBytes(16, 0)
		if n, err := keyFile.Read(raw); err != nil || n != 16 {
			panic("read current key failed")
		}
		return raw
	} else {
		panic("key file format error")
	}
}

// GenPublicKey make the Public key data for ready to verify signature and card UID
func (vc *VerifyCard) genPublicKey() {
	vc.PublicKeyHexRaw = "048A9B380AF2EE1B98DC417FECC263F8449C7625CECE82D9B916C992DA209D68422B81EC20B65A66B5102A61596AF3379200599316A00A1410"
	src := []byte(vc.PublicKeyHexRaw)
	dst := make([]byte, hex.DecodedLen(len(src)))
	// Dropped error check because the public key is critical element,
	// any problem happen must let it panic to stop program instead solve it.
	hex.Decode(dst, src)

	// Remove deprecated method - Kevin Mak, 10/12/2024
	vc.PublicKey = ecdsa.PublicKey{
		Curve: elliptic.P224(),
		X:     big.NewInt(0).SetBytes(dst[1:29]),
		Y:     big.NewInt(0).SetBytes(dst[29:]),
	}
}

func calculateSdmMac(sdmFileReadKey, piccData []byte, lrp bool, encFile ...[]byte) (ret []byte, err error) {
	// if len(encFile) > 0 {
	// 	err = errors.New("encFile not implemented")
	// 	return
	// }

	// The mac is use pattern as [2,4,6...]
	finBytes := func(input []byte) []byte {
		fin := make([]byte, 0)
		for i := range input {
			if i%2 == 1 {
				fin = append(fin, input[i])
			}
		}
		return fin
	}

	if !lrp {
		buf := bytes.Buffer{}
		buf.Write([]byte{0x3c, 0xc3, 0x00, 0x01, 0x00, 0x80})
		buf.Write(piccData)

		// TODO: if encfile input must do padding
		// for (buf.Len())%aes.BlockSize != 0 {
		// 	buf.Write([]byte{0x00})
		// }

		if c2, err2 := makeCMAC(sdmFileReadKey, buf.Bytes()); err2 != nil {
			err = err2
			return
		} else {
			// SDM CMAC finally will apply the file data, even file data is null
			// here not implemented the file SDM CMAC, user a empty byte array

			// Input length was verified will never reach error. (2022-08-11 update)
			c3, _ := makeCMAC(c2, []byte{})
			ret = finBytes(c3)
		}
	} else {
		// Current only AES method, lrp not support
		// return nil, errors.New("not implemented")

		buf := bytes.Buffer{}
		buf.Write([]byte{0x00, 0x01, 0x00, 0x80})
		buf.Write(piccData)

		// TODO: if encfile input must do padding
		// for (buf.Len()+2)%aes.BlockSize != 0 {
		// 	buf.Write([]byte{0x00})
		// }

		buf.Write([]byte{0x1E, 0xE1})
		bin := buf.Bytes()

		lrpMaster := NewLRP(sdmFileReadKey, 0, MakeBytes(16, 0x00), true)
		masterKey := lrpMaster.cmac(bin)

		lrpSessMacing := NewLRP(masterKey, 0, MakeBytes(16, 0x00), true)
		var h string
		var macDigest []byte
		if len(encFile) > 0 {
			h = strings.ToUpper(hex.EncodeToString(encFile[0]))
			macDigest = lrpSessMacing.cmac([]byte(h)) // LRP will CMAC the all PICC data
		} else {
			macDigest = lrpSessMacing.cmac([]byte{})
		}

		// log.Printf("enc: %s", h)
		// log.Printf("bin: %s", hex.EncodeToString(bin))
		// log.Printf("mac: %s", hex.EncodeToString(macDigest))
		ret = finBytes(macDigest)
	}

	return
}

func makeCMAC(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cmac.Sum(data, block, 16)
}

func (vc *VerifyCard) VerifyPICCData(piccEncData, sdmmac []byte, db ICardDB, debug bool) (cardid []byte, err error) {
	// "output" is buffer for decrypted data,
	// the buffer length must same as input
	output := make([]byte, len(piccEncData))

	lrp := false
	if len(piccEncData) == 24 { // LRP case, not fully tested
		lrp = true
	} else if len(piccEncData) != 16 { // AES case
		return nil, ErrVerifySunNotMatch
	}

	metaKey := SdmMetaKey
	if debug {
		metaKey = sdmTestKey
	}

	// Decryption for transfer data
	if !lrp {
		// Key must use 16 byte (AES-128)
		b, cerr := aes.NewCipher(metaKey)
		if cerr != nil {
			err = cerr
			return
		}

		// Block cipher method must CBC, IV is 16 byte array with "0"
		bm := cipher.NewCBCDecrypter(b, make([]byte, 16))
		bm.CryptBlocks(output, piccEncData)
	} else {
		// LRP decryption
		piccRand := piccEncData[0:8]
		piccEncDataStripped := piccEncData[8:]

		cipher := NewLRP(metaKey, 0, piccRand, false)
		output = cipher.Decrypt(piccEncDataStripped)
	}
	sdmBuf := bytes.NewBuffer(output)

	// Get message header
	// Reference document: chapter 4.3, AN12196
	config, _ := sdmBuf.ReadByte()
	uidMirrored := config&0x80 == 0x80
	sdmReadCtrMirrored := config&0x40 == 0x40
	uidLength := config & 0x0F

	sdmmacBuf := bytes.Buffer{}

	if uidLength != 0x07 {
		// fake SDMMAC calculation to avoid potential timing attacks
		calculateSdmMac(sdmTestKey, make([]byte, 10), lrp)
		err = ErrVerifySunNotMatch
		return
	}

	// Get `uid` and `ctr`
	var uid []byte
	if uidMirrored {
		uid = sdmBuf.Next(int(uidLength))
		sdmmacBuf.Write(uid)
	}
	var ctrBuf []byte
	if sdmReadCtrMirrored {
		ctrBuf = sdmBuf.Next(3)
		sdmmacBuf.Write(ctrBuf)
	}

	var cardFileKey, cardCtr []byte
	cardFileKey, cardCtr, err = db.ReadCardFilekey(uid)
	if err != nil {
		return
	}
	var ret []byte
	var err2 error

	if debug {
		cardFileKey = sdmTestKey
	}

	// The server disabled enc file function only do sdmmac verify
	// if lrp {
	// 	ret, err2 = calculateSdmMac(cardFileKey, sdmmacBuf.Bytes(), lrp, piccEncData)
	// } else {
	// 	ret, err2 = calculateSdmMac(cardFileKey, sdmmacBuf.Bytes(), lrp)
	// }
	ret, err2 = calculateSdmMac(cardFileKey, sdmmacBuf.Bytes(), lrp)
	if err2 != nil {
		err = err2
		return
	}

	if bytes.Equal(ret, sdmmac) {
		ctrReq, nReq := binary.Uvarint(ctrBuf)
		ctrSav, nSav := binary.Uvarint(cardCtr)
		if nReq <= 0 || nSav <= 0 {
			err = ErrVerifySunRead
			return
		}
		if ctrReq > ctrSav {
			status := StatusCtrNormal
			if ctrReq-ctrSav > 2 {
				status = StatusCtrJump
			}
			db.UpdateCardCTR(uid, ctrBuf, status)
			cardid = uid
		} else {
			db.UpdateCardCTR(uid, cardCtr, StatusCtrRepeated)
			LastErrorID = hex.EncodeToString(uid)
			err = ErrVerifySunCtrRepeated
			// cardid = uid // TODO: debug only
		}
		return
	}

	err = ErrVerifySunNotMatch
	return
}

func (vc *VerifyCard) VerifyPlainSUN(uid, readCtr, sdmmac []byte) error {
	buf := bytes.Buffer{}
	buf.Write(uid)

	revReadCtr := make([]byte, len(readCtr))
	for i, j := 0, len(readCtr)-1; i <= j; i, j = i+1, j-1 {
		revReadCtr[i], revReadCtr[j] = readCtr[j], readCtr[i]
	}
	buf.Write(revReadCtr)

	if ret, err := calculateSdmMac(sdmTestKey, buf.Bytes(), false); err != nil {
		return err
	} else {
		if bytes.Equal(ret, sdmmac) {
			return nil
		} else {
			return ErrVerifySunNotMatch
		}
	}
}

// VerifyUIDBase64 verify use base data
func (vc *VerifyCard) VerifyUIDBase64(sign string, uid string) bool {
	var signRaw, uidRaw []byte
	if b, err := base64.URLEncoding.DecodeString(sign); err != nil {
		return false
	} else {
		signRaw = b
	}

	if b, err := base64.URLEncoding.DecodeString(uid); err != nil {
		return false
	} else {
		uidRaw = b
	}
	return vc.VerifyUID(signRaw, uidRaw)
}

// VerifyUID verify the signature and card UID
func (vc *VerifyCard) VerifyUID(sign []byte, uid []byte) bool {
	// signature is the concatenation of (r, s), base64Url encoded.
	sigLength := len(sign)
	expectedOctetLength := 2 * ((vc.PublicKey.Params().BitSize + 7) >> 3)
	if sigLength != expectedOctetLength {
		return false
	}

	rBytes, sBytes := sign[:sigLength/2], sign[sigLength/2:]
	r := new(big.Int).SetBytes(rBytes)
	s := new(big.Int).SetBytes(sBytes)

	return ecdsa.Verify(&vc.PublicKey, uid, r, s)
}

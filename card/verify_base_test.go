package card_test

import (
	"bytes"
	"errors"
	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
)

// SdmMetaKey = []byte{
// 	0x4b, 0xe2, 0x52, 0x43, 0xa6, 0x8e, 0x40, 0xf7,
// 	0xb7, 0x11, 0x61, 0x65, 0x00, 0xa3, 0x86, 0xd9,
// }

func TestMakeMetaKeyFile(t *testing.T) {
	tempPath := MakeTempPath()
	defer os.RemoveAll(tempPath)

	// 1. Create the meta key first
	t.Run(`1.CreateMetaKey`, func(t *testing.T) {
		card.MakeMetaKeyFile(tempPath)
		keyOnDisk, err := os.ReadFile(tempPath + "/metakey")
		if err != nil {
			t.Fatalf(`Should metakey be generated: %v`, err)
		}
		if len(keyOnDisk) != 16 {
			t.Fatalf(`Should generate a 16-byte metakey, got %d bytes`, len(keyOnDisk))
		}
		if !bytes.Equal(keyOnDisk, card.SdmMetaKey) {
			t.Errorf(`Should load the newly generated metakey into memory`)
		}
	})

	// 2. Read and verify from the meta key
	t.Run(`2.CheckMemoryMetaKey`, func(t *testing.T) {
		var buf [16]byte
		if f, err := os.Open(tempPath + "/metakey"); err != nil {
			t.Fatalf(`Should file created`)
		} else if s, err := f.Read(buf[:]); err != nil || s != 16 {
			t.Fatalf(`Should read the metakey success`)
		}

		card.MakeMetaKeyFile(tempPath)
		if !bytes.Equal(buf[:], card.SdmMetaKey) {
			t.Errorf(`Should the loaded key same as filekey`)
		}
	})

	// 3. Should fail when key create failed
	t.Run(`3.CheckError_KeyFileCreate`, func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.FailNow()
			}
		}()

		card.MakeMetaKeyFile("./errorpath")
		t.Errorf(`Should throw panic when file cannot created`)
	})

	// 4. Should fail when key file not correct format
	t.Run(`4.CheckError_KeyFormatWrong`, func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.FailNow()
			}
		}()
		if err := os.WriteFile(tempPath+"/metakey", []byte{0x00, 0x1a}, 0755); err != nil {
			t.Fatalf(`failed to change raw`)
		}

		card.MakeMetaKeyFile(tempPath)
		t.Errorf(`Should throw panic when file in wrong format`)
	})
}

func TestMakeAppMasterKeyFile(t *testing.T) {
	tempPath := MakeTempPath()
	defer os.RemoveAll(tempPath)

	card.MakeAppMasterKeyFile(tempPath)
	keyOnDisk, err := os.ReadFile(tempPath + "/appmasterkey")
	if err != nil {
		t.Fatalf(`Should app master key be generated: %v`, err)
	}
	if len(keyOnDisk) != 16 {
		t.Fatalf(`Should generate a 16-byte app master key, got %d bytes`, len(keyOnDisk))
	}
	if !bytes.Equal(keyOnDisk, card.AppMasterKey) {
		t.Errorf(`Should load the newly generated app master key into memory`)
	}

	card.AppMasterKey = nil
	card.MakeAppMasterKeyFile(tempPath)
	if !bytes.Equal(keyOnDisk, card.AppMasterKey) {
		t.Errorf(`Should reload the same app master key from disk`)
	}
}

func TestMakeVerfiyCard(t *testing.T) {
	tempPath := MakeTempPath()
	defer os.RemoveAll(tempPath)

	// 1. Should read the signature and get metakeys
	t.Run(`1.ReadAndWriteKey`, func(t *testing.T) {
		v := card.MakeVerifyCard(tempPath)
		x := v.PublicKey.X
		if x.String() != "14596949609115213286476537388079987038015606750940661749886531244762" {
			t.Errorf(`Should return same signature`)
		}
	})
}

func TestVerifyPICCData(t *testing.T) {
	ctrl := gomock.NewController(t)
	cardDBmock := mock.NewMockICardDB(ctrl)

	tempPath := MakeTempPath()
	defer os.RemoveAll(tempPath)
	v := card.MakeVerifyCard(tempPath)

	t.Run("1.VerifyWithSampleCase", func(t *testing.T) {
		card.SdmMetaKey = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		picc := []byte{
			0xEF, 0x96, 0x3F, 0xF7, 0x82, 0x86, 0x58, 0xA5,
			0x99, 0xF3, 0x04, 0x15, 0x10, 0x67, 0x1E, 0x88,
		}
		sdmmac := []byte{
			0x94, 0xEE, 0xD9, 0xEE, 0x65, 0x33, 0x70, 0x86,
		}

		cardDBmock.EXPECT().UpdateCardCTR(gomock.Any(), []byte{0x3D, 0x00, 0x00}, gomock.Any()).Return(nil)
		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(card.SdmMetaKey, []byte{0x00, 0x00, 0x00}, nil)

		if id, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, false); err != nil ||
			!bytes.Equal(id, []byte{0x04, 0xde, 0x5f, 0x1e, 0xac, 0xc0, 0x40}) {
			t.Errorf("Should pass the AES verify")
		}
	})

	t.Run("2.VerfiyWithLRP", func(t *testing.T) {
		card.SdmMetaKey = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		picc := []byte{
			0x1F, 0xCB, 0xE6, 0x1B, 0x3E, 0x4C, 0xAD, 0x98,
			0x0C, 0xBF, 0xDD, 0x33, 0x3E, 0x7A, 0x4A, 0xC4,
			0xA5, 0x79, 0x56, 0x9B, 0xAF, 0xD2, 0x2C, 0x5F,
		}
		sdmmac := []byte{
			0x42, 0x31, 0x60, 0x8B, 0xA7, 0xB0, 0x2B, 0xA9,
		}

		cardDBmock.EXPECT().UpdateCardCTR(gomock.Any(), []byte{0x03, 0x00, 0x00}, gomock.Any()).Return(nil)
		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(card.SdmMetaKey, []byte{0x00, 0x00, 0x00}, nil)

		if id, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, true); err != nil ||
			!bytes.Equal(id, []byte{0x04, 0x94, 0x0e, 0x2a, 0x2f, 0x70, 0x80}) {
			t.Errorf("Should pass the LRP verify")
		}
	})

	t.Run("3.InvalidInput", func(t *testing.T) {
		card.SdmMetaKey = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
		}
		dummyPicc := make([]byte, 16)
		dummy := []byte{}
		if _, err := v.VerifyPICCData(dummyPicc, dummy, cardDBmock, false); err == nil {
			t.Errorf("Should fail invalid meta key")
		}

		// plain := []byte{
		// 	0xc6, 0x04, 0xde, 0x5f, 0x1e, 0xac, 0xc0, 0x40,
		// 	0x08, 0x00, 0x00, 0x31, 0xad, 0x56, 0x3a, 0x33,
		// }
		// [0] Then len config data is "c6"
		// - uidMirror: true
		// - sdmReadCtr: true
		// - uidLength: 6 (invalid)
		// [1-7] id in 7 byte
		// [8-10] ctr in 7 byte
		// [11-15] random fill
		invalidLenPicc := []byte{
			0xf2, 0x3a, 0x30, 0x15, 0xa3, 0x35, 0x8e, 0xcb,
			0x0c, 0xe3, 0x86, 0xc4, 0x24, 0x24, 0xe7, 0xc5,
		}

		if _, err := v.VerifyPICCData(invalidLenPicc, dummy, cardDBmock, true); err != card.ErrVerifySunNotMatch {
			t.Errorf("Should fail len config not equal 7")
		}

		if _, err := v.VerifyPICCData(dummy, dummy, cardDBmock, true); err != card.ErrVerifySunNotMatch {
			t.Errorf("Should fail when picc is empty")
		}

		// CTR set to 0xff, 0xff, 0xff
		picc := []byte{
			0xcb, 0xea, 0x53, 0x9f, 0x65, 0x09, 0x14, 0xec,
			0x51, 0x80, 0xa4, 0x4b, 0xd5, 0x7b, 0x78, 0x40,
		}
		sdmmac := []byte{
			0xa1, 0xc2, 0xab, 0x3f, 0xca, 0xe5, 0xbc, 0xf9,
		}

		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(card.SdmMetaKey, []byte{0x00, 0x00, 0x00}, nil)
		if _, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, true); err != card.ErrVerifySunRead {
			t.Errorf("Should fail when picc ctr is invalid")
		}

		picc = []byte{
			0xEF, 0x96, 0x3F, 0xF7, 0x82, 0x86, 0x58, 0xA5,
			0x99, 0xF3, 0x04, 0x15, 0x10, 0x67, 0x1E, 0x88,
		}
		sdmmac = []byte{
			0x94, 0xEE, 0xD9, 0xEE, 0x65, 0x33, 0x70, 0x86,
		}
		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(card.SdmMetaKey, []byte{0xff, 0xff, 0xff}, nil)
		if _, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, true); err != card.ErrVerifySunRead {
			t.Errorf("Should fail when card file ctr is invalid")
		}

		picc = []byte{
			0x6b, 0x57, 0x97, 0x66, 0xef, 0x0d, 0xfd, 0x08,
			0xea, 0x89, 0xae, 0x78, 0x6d, 0x7e, 0x54, 0xa0,
		}
		sdmmac = []byte{
			0xa6, 0xa9, 0xbc, 0x88, 0xaa, 0xe5, 0xa0, 0xac,
		}
		dbfileCtr := []byte{0x09, 0x00, 0x00}
		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(card.SdmMetaKey, dbfileCtr, nil)
		cardDBmock.EXPECT().UpdateCardCTR(gomock.Any(), dbfileCtr, card.StatusCtrRepeated)
		if _, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, true); err != card.ErrVerifySunCtrRepeated {
			t.Errorf("Should fail when ctr is old")
		}

		sdmmac = make([]byte, 16)
		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(card.SdmMetaKey, dbfileCtr, nil)
		if _, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, true); err != card.ErrVerifySunNotMatch {
			t.Errorf("Should fail when sdmmac not match")
		}
	})

	t.Run("4.DBReadFailed", func(t *testing.T) {
		picc := []byte{
			0xEF, 0x96, 0x3F, 0xF7, 0x82, 0x86, 0x58, 0xA5,
			0x99, 0xF3, 0x04, 0x15, 0x10, 0x67, 0x1E, 0x88,
		}
		sdmmac := []byte{
			0x94, 0xEE, 0xD9, 0xEE, 0x65, 0x33, 0x70, 0x86,
		}

		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).Return(nil, nil, errors.New("db error"))
		if _, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, true); err == nil {
			t.Errorf("Should failed when db have error")
		}

		// 2.2. Should fail cmac when filekey error
		card.SdmMetaKey = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		failFileFey := []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
		}

		cardDBmock.EXPECT().ReadCardFilekey(gomock.Any()).
			Return(failFileFey, []byte{0x00, 0x00, 0x00}, nil)
		if _, err := v.VerifyPICCData(picc, sdmmac, cardDBmock, false); err == nil {
			t.Errorf("Should failed when db have error")
		}
	})
}

func TestVerifyPlainSUN_UID(t *testing.T) {
	tempPath := MakeTempPath()
	defer os.RemoveAll(tempPath)
	vc := card.MakeVerifyCard(tempPath)

	t.Run("VerifyPlainSUN", func(t *testing.T) {
		uid := []byte{0x04, 0x1E, 0x3C, 0x8A, 0x2D, 0x6B, 0x80}
		ctr := []byte{0x00, 0x00, 0x06}
		sdmmac := []byte{0x4B, 0x00, 0x06, 0x40, 0x04, 0xB0, 0xB3, 0xD3}
		if vc.VerifyPlainSUN(uid, ctr, sdmmac) != nil {
			t.Errorf("Should pass plain SDM verify")
		}

		sdmmac = make([]byte, 8)
		if vc.VerifyPlainSUN(uid, ctr, sdmmac) != card.ErrVerifySunNotMatch {
			t.Errorf("Should fail to match invalid sdmmac")
		}

		ctr = []byte{0x00, 0x00, 0x06, 0x00}
		if vc.VerifyPlainSUN(uid, ctr, sdmmac) == nil {
			t.Errorf("Should fail sdmmac error")
		}
	})

	// case referred by:
	// https://github.com/icedevml/nfc-ev2-crypto/blob/master/test_ecc.py
	t.Run("VerifyUIDBase64", func(t *testing.T) {
		dataset := make(map[string]string)
		dataset["BHcogk1lgA=="] = "nHBoY2ooX-uh4kBmVACdzm6KnJzjGL4CYu0WJRm5cLMvuAKFhCDWZ9Es680yu0vIr-IcdTpabCY="
		dataset["BB08ii1rgA=="] = "pDj-s2jcRfm4iZwpJqmFvl0I40LOAW6fCPP9rjiLtNaz39feouGXfM0uTJ-C8kXU3gj7f8uAQ-M="
		dataset["BBw8ii1rgA=="] = "tG0FOfZTMODTwmL9aXHC4d4bYJ5o4tADkD-q_MCkrG23jGOKhSR_5a0q1TUQW1zwQiFDvygXuGI="
		dataset["BCQogk1lgA=="] = "fcftpFUtI989kONuSry4wRbZcdcWJZbAC10fak7_D75WLRm3yV1lgvDadLQ-_S2zbNShRRzogzA="
		dataset["BBs8ii1rgA=="] = "tFdp7OumTf5ucvEiiuBcw2KgYhbaqDE6V8zplRNfQjr83q7P7pNbieqPTd6p9mmhnFda2PfqdDI="
		dataset["BH0ogk1lgA=="] = "tc7Hf5ytMz6-jOWM5tceVrGkbkpr0_ak0kS58miKuCOJL3kIhgBWjKnjD0OTcos1c7bZjUEh7E0="

		for id, sign := range dataset {
			if !vc.VerifyUIDBase64(sign, id) {
				t.Errorf("Should pass check signature")
			}
		}
	})

	t.Run("InvalidRandom", func(t *testing.T) {
		dataset := make(map[string]string)
		dataset["IabY_VUo0w=="] = "gXLvuhDhiE5JU2_CHq0a5VrwihQj2-Q8g2TEVlNGlLzvC_hb8ymg_4siEBOvhgl2qwKRVb2Vu84="
		dataset["pTzPVjDvMA=="] = "tOmQNaimJE63e3DupQDInJ-h6zNN3cl1XCqkj6MZybkoKfPPXlG7ixjFOrzlVy3_DExAe_3EM4g="
		dataset["iM8aX5ICBQ=="] = "JUvWluVwrXV4FBX5XFw-zcinm4GEup2CJRsZz8TQ36-pZi94o-ll9kJKGdQZg3wkHLW1WXhM0VQ="

		for id, sign := range dataset {
			if vc.VerifyUIDBase64(sign, id) {
				t.Errorf("Should false with random value")
			}
		}
	})

	t.Run("InvalidFlip", func(t *testing.T) {
		dataset := make(map[string]string)
		dataset["BCQogk1lgA=="] = "fcftpFUtI989kONuSry4wRbZcdcWJZbAC10fak7_D75WLRm3yV1lgvDadLQ-_S2zbNShRRzogz8="
		dataset["BBs8ii1rgA=="] = "tFdp7OumTf5ucvEiiuBcw2KgYhbaqDE6V8zplRNfQjr83q7P7pNbieqPTd6p9mmhnFda2PfqdDE="
		dataset["BH0ogk1lgA=="] = "tc7Hf5ytMz6-jOWM5tceVrGkbkpr0_ak0kS58miKuCOJL3kIhgBWjKnjD0OTcos1c7bZjUEh7E4="
		dataset["BHcogk1lgA=="] = "nHBoY2ooX-uh4kBmVACdzm6KnJzjGL4CYu0WJRm5cLMvuAKFhCDWZ9Es680yu0vIr-IcdTpabCU="
		dataset["BB08ii1rgA=="] = "pDj-s2jcRfm4iZwpJqmFvl0I40LOAW6fCPP9rjiLtNaz39feouGXfM0uTJ-C8kXU3gj7f8uAQ-I="
		dataset["BBw8ii1rgA=="] = "tG0FOfZTMODTwmL9aXHC4d4bYJ5o4tADkD-q_MCkrG23jGOKhSR_5a0q1TUQW1zwQiFDvygXuGE="

		for id, sign := range dataset {
			if vc.VerifyUIDBase64(sign, id) {
				t.Errorf("Should false with random value")
			}
		}
	})

	t.Run("ErrorCase", func(t *testing.T) {
		sign := "fcftpFUtI989kONuSry4wRbZcdcWJZbAC10fak7_D75WLRm3yV1lgvDadLQ-_S2zbNShRRzogz8="
		id := "BCQogk1lgA"
		if vc.VerifyUIDBase64(sign, id) {
			t.Errorf("Should fail invalid id")
		}

		if vc.VerifyUIDBase64(sign[:75], id) {
			t.Errorf("Should fail invalid sign")
		}

		bsign := make([]byte, 52)
		bid := make([]byte, 7)
		if vc.VerifyUID(bsign, bid) {
			t.Errorf("Should fail with invalid len signature")
		}
	})
}

// func MakePICCChiper(plain []byte) (picc, sdmmac []byte) {
// 	picc = make([]byte, 16)
// 	perkey := []byte{
// 		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
// 		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
// 	}
// 	if b, err := aes.NewCipher(perkey); err == nil {
// 		bm := cipher.NewCBCEncrypter(b, make([]byte, 16))
// 		bm.CryptBlocks(picc, plain)
// 	}

// 	finBytes := func(input []byte) []byte {
// 		fin := make([]byte, 0)
// 		for i := range input {
// 			if i%2 == 1 {
// 				fin = append(fin, input[i])
// 			}
// 		}
// 		return fin
// 	}

// 	buf := bytes.Buffer{}
// 	buf.Write([]byte{
// 		0x3c, 0xc3, 0x00, 0x01, 0x00, 0x80,
// 	})
// 	buf.Write(plain[1:11])
// 	pass01, _ := card.MakeCMAC(make([]byte, 16), buf.Bytes())
// 	pass02, _ := card.MakeCMAC(pass01, []byte{})
// 	sdmmac = finBytes(pass02)
// 	log.Printf("%x, %x", picc, sdmmac)

// 	return
// }

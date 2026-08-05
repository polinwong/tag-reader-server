package server_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"
	"marveldigital/tag-reader-server/server"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	uuid "github.com/iris-contrib/go.uuid"
	"github.com/kataras/iris/v12"
	"go.etcd.io/bbolt"
)

// defaultModelSet to make default linkdb
func defaultModelSet() map[string]string {
	set := make(map[string]string)
	set["01234567-89ab-cdef-0123-456789abcdef"] = `{"image": "i1", "desc": "info1", "name": "foo"}`
	set["20000000-0000-0000-0000-000000000000"] = `{"image": "", "desc": "info2", "name": "bar"}`
	set["30000000-0000-0000-0000-000000000000"] = `{"image": "i3", "desc": "info3", "name": ""}`
	set["40000000-0000-0000-0000-000000000000"] = `{"image": "i4", "desc": "info4", "name": "boo"}`
	set["50000000-0000-0000-0000-000000000000"] = `{"image": "i5", "desc": "info5", "name": "banana"}`
	return set
}

// defaultCardSet to make default carddb
func defaultCardSet() map[string]string {
	set := make(map[string]string)
	set[string([]byte{0x00, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "30000000-0000-0000-0000-000000000000", "filekey": "01123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0x01, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "20000000-0000-0000-0000-000000000000", "filekey": "02123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0xa2, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "40000000-0000-0000-0000-000000000000", "filekey": "03123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE=", "status": "REPEATED"}`
	set[string([]byte{0xa3, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "60000000-0000-0000-0000-000000000000", "filekey": "04123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0xb4, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "70000000-0000-0000-0000-000000000000", "filekey": "05123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE="}`
	return set
}

func loginMock(mockBuk *mock.MockIBucket, mockTx *mock.MockITx, mockUuid *mock.MockUUID) (
	form iris.Map, uuidDummy string, salt, hashRet []byte) {
	var (
		orgid = "sampleuser"
		orgpw = "samplepw"
	)
	salt = []byte("samplesalt")
	hashRet = []byte("c848f9e0c52559d77a9408959a6caa48b799b133d23dbd0be6ca2239dc8b0151")
	uuidDummy = "e1a0c578-3214-4ee3-a9e9-feb537f11cfd"
	form = iris.Map{
		"inid": orgid,
		"inpw": orgpw,
	}

	mockBuk.EXPECT().Get([]byte("rec")).Return(hashRet)
	mockBuk.EXPECT().Get([]byte("salt")).Return(salt)
	mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk).Times(2)
	mockUuid.EXPECT().NewV4().Return(uuid.FromString(uuidDummy))
	return
}

func TestVerify(t *testing.T) {
	server.UpdateLogio(log.Default())

	// DB mock
	e, _, _, mockTx, mockBuk := makeHttpTest(t)

	tmplId := []byte{0x04, 0x77, 0x28, 0x82, 0x4d, 0x65, 0x80}
	const refLink = "https://somelink"
	const refKey = "00123456789abcdefedcba9876543210"
	const refSign = "nHBoY2ooX+uh4kBmVACdzm6KnJzjGL4CYu0WJRm5cLMvuAKFhCDWZ9Es680yu0vIr+IcdTpabCY="
	const refCtr = "AAAA"
	const tmplJSON = "{\"ctr\":\"" + refCtr +
		"\",\"filekey\":\"" + refKey +
		"\",\"link\":\"" + refLink +
		"\",\"sign\":\"" + refSign +
		"\",\"status\":\"" + card.StatusCtrNormal + "\"}"

	t.Run("1.RunAPIVerifyUID", func(t *testing.T) {
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(2)
		retJson1 := []byte(tmplJSON)
		mockBuk.EXPECT().Get(tmplId).Return(retJson1)

		formInput := iris.Map{
			"u": "BHcogk1lgA==",
			"s": "nHBoY2ooX-uh4kBmVACdzm6KnJzjGL4CYu0WJRm5cLMvuAKFhCDWZ9Es680yu0vIr-IcdTpabCY=",
		}
		// 1.1 Should ID verify pass
		e.POST("/verify/api").WithForm(formInput).Expect().Body().Contains("key verify")

		// 1.2 Should fail when DB error
		mockBuk.EXPECT().Get(tmplId).Return([]byte("Not JSON"))
		e.POST("/verify/api").WithForm(formInput).Expect().Body().Contains("access error")

		// 1.3 Should fail decode invalid Base64 input
		formInput["u"] = "BHcogk1lgA==*"
		e.POST("/verify/api").WithForm(formInput).Expect().Body().Contains("false")

		// 1.4 Should return not found when non input to block try
		formInput["u"] = ""
		formInput["s"] = ""
		e.POST("/verify/api").WithForm(formInput).Expect().Body().Contains("Not Found")
	})

	card.SdmMetaKey = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	t.Run("2.1.RunAPIVerifySUN", func(t *testing.T) {
		e.GET("/verify/sun").Expect().Body().Contains("Coinllectibles Verify")
		e.POST("/verify/sun").Expect().Body().Contains("Not Found")

		curId := []byte{0x04, 0xde, 0x5f, 0x1e, 0xac, 0xc0, 0x40}
		formInput := iris.Map{
			"d": "EF963FF7828658A599F3041510671E8894EED9EE65337086",
		}

		newJSON := "{\"ctr\":\"" + refCtr +
			"\",\"filekey\":\"" + "00000000000000000000000000000000" +
			"\",\"link\":\"" + refLink +
			"\",\"sign\":\"" + refSign +
			"\",\"status\":\"" + card.StatusCtrNormal + "\"}"

		mockSun := func() {
			mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(3)
			mockBuk.EXPECT().Get(curId).Return([]byte(newJSON)).Times(3)
			mockBuk.EXPECT().Put(curId, gomock.Any()).Return((nil))
		}
		// 2.1.1 Should GET request pass
		mockSun()
		e.GET("/verify/sun").WithQuery("d", formInput["d"]).Expect().
			Body().Contains(`linkId = "https://somelink"`)

		// 2.1.2 Should POST request pass
		mockSun()
		e.POST("/verify/sun").WithForm(formInput).Expect().
			Body().Contains(`"https://somelink"`)

		// 2.1.3 Should split input pass
		formSplit := iris.Map{
			"picc_data": "EF963FF7828658A599F3041510671E88",
			"cmac":      "94EED9EE65337086",
		}
		mockSun()
		e.POST("/verify/sun").WithForm(formSplit).Expect().
			Body().Contains(`"https://somelink"`)

		// 2.1.4 Should apps with header requeset passed
		mockSun()
		res := e.POST("/verify/sun").WithForm(formInput).WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect()
		res.Headers().ContainsKey("Access-Control-Allow-Credentials")
		res.Body().Contains(`"https://somelink"`)
	})

	t.Run("2.2.RunAPIVerifySUN-Failed", func(t *testing.T) {
		newJSON := "{\"ctr\":\"" + "/wAA" +
			"\",\"filekey\":\"" + "00000000000000000000000000000000" +
			"\",\"link\":\"" + refLink +
			"\",\"sign\":\"" + refSign +
			"\",\"status\":\"" + card.StatusCtrNormal + "\"}"

		card.SdmMetaKey = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		curId := []byte{0x04, 0xde, 0x5f, 0x1e, 0xac, 0xc0, 0x40}
		formInput := iris.Map{
			"d": "EF963FF7828658A599F3041510671E8894EED9EE65337086",
		}

		mockSun := func() {
			mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(2)
			mockBuk.EXPECT().Get(curId).Return([]byte(newJSON)).Times(2)
			mockBuk.EXPECT().Put(curId, gomock.Any()).Return((nil))
		}
		// 2.2.1 Should fail when CTR have outdated.
		// CTR is b64: "/wAA" as {0x255, 0x00, 0x00}
		mockSun()
		e.GET("/verify/sun").WithQuery("d", formInput["d"]).Expect().
			Body().Contains(`inputData = "FAIL"`)

		// 2.2.2 Should return fail when database have panic
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).DoAndReturn(
			func(n []byte) card.IBucket {
				panic("buk panic")
			})
		e.GET("/verify/sun").WithQuery("d", formInput["d"]).Expect().
			Body().Contains(`data invalid`)

		// 2.2.3 Should return fail when request HEX data have invalid data
		formInput = iris.Map{
			"d": "EF963FF7828658A599F3041510671E8894EED9EE6533708M",
		}
		e.GET("/verify/sun").WithQuery("d", formInput["d"]).Expect().
			Body().Contains(`read data empty error`)

		// 2.2.4 Should return fail when data is lost fit required size
		formInput = iris.Map{
			"d": "EF963FF7828658A599F3041510671E88",
		}
		e.GET("/verify/sun").WithQuery("d", formInput["d"]).Expect().
			Body().Contains(`data invalid`)
	})

	t.Run("2.3.RunAPIVerifySUN-LRP", func(t *testing.T) {
		curId := []byte{0x04, 0x94, 0x0e, 0x2a, 0x2f, 0x70, 0x80}
		newJSON := "{\"ctr\":\"" + refCtr +
			"\",\"filekey\":\"" + "00000000000000000000000000000000" +
			"\",\"link\":\"" + refLink +
			"\",\"sign\":\"" + refSign +
			"\",\"status\":\"" + card.StatusCtrNormal + "\"}"

		formInput := iris.Map{
			"d": "1FCBE61B3E4CAD980CBFDD333E7A4AC4A579569BAFD22C5F" + "4231608BA7B02BA9",
		}

		// 2.3 Should pass input as LRP, LRP have tested in verify_base,
		// so there will not repeat to test algorithm.
		mockSun := func() {
			mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(3)
			mockBuk.EXPECT().Get(curId).Return([]byte(newJSON)).Times(3)
			mockBuk.EXPECT().Put(curId, gomock.Any()).Return((nil))
		}
		mockSun()
		e.GET("/verify/sun").WithQuery("d", formInput["d"]).Expect().
			Body().Contains(`linkId = "https://somelink"`)
	})

	t.Run("3.GetLinkModelDetail", func(t *testing.T) {
		dataset := defaultModelSet()
		mockDetail := func(id string) {
			mockBuk.EXPECT().Get([]byte(id)).Return([]byte(dataset[id]))
			mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk)
		}

		validId1 := "01234567-89ab-cdef-0123-456789abcdef"
		mockDetail(validId1)
		e.GET("/verify/api/linkmodel/" + validId1).Expect().
			Body().Contains(`"img":"/verify/source/img/i1"`)

		validId2 := "20000000-0000-0000-0000-000000000000"
		mockDetail(validId2)
		e.GET("/verify/api/linkmodel/" + validId2).Expect().
			Body().Contains(`"img":""`)

		invalidId := "30000000-1000-2000-3000-400000000000"
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk)
		mockBuk.EXPECT().Get([]byte(invalidId)).Return(nil)
		e.GET("/verify/api/linkmodel/" + invalidId).Expect().
			Body().Contains("Not Found")
	})

	server.Close()
}

func TestAdminCardView(t *testing.T) {
	ctrl := gomock.NewController(t)
	server.UpdateLogio(log.Default())

	// DB mock
	e, _, _, mockTx, mockBuk := makeHttpTest(t)

	t.Run("1.VerifyApiLogin", func(t *testing.T) {
		mockUuid := mock.NewMockUUID(ctrl)
		tmpUuid := card.DbUUID
		card.DbUUID = mockUuid

		formPw, uuidDummy, salt, hashRet := loginMock(mockBuk, mockTx, mockUuid)
		mockBuk.EXPECT().Put([]byte(uuidDummy), gomock.Any()).Return(nil)
		// 1.1 Should pass the login check
		res := e.POST("/verify/api/login").WithForm(formPw).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect()
		res.Body().Contains("OK").Contains(uuidDummy)

		// 1.2 Should fail when OnlLoginUser throw error
		formPw, uuidDummy, _, _ = loginMock(mockBuk, mockTx, mockUuid)
		mockBuk.EXPECT().Put([]byte(uuidDummy), gomock.Any()).Return(errors.New("put error"))
		e.POST("/verify/api/login").WithForm(formPw).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect().
			Body().Contains("FAIL")

		// 1.3 Should fail when password not match
		mockBuk.EXPECT().Get([]byte("rec")).Return(hashRet)
		mockBuk.EXPECT().Get([]byte("salt")).Return(salt)
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk)
		wrongPw := formPw
		wrongPw["inpw"] = "failpw"
		e.POST("/verify/api/login").WithForm(wrongPw).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect().
			Body().Contains("FAIL")

		// 1.4 Should fail when db have panic
		mockTx.EXPECT().Bucket(gomock.Any()).DoAndReturn(func(k []byte) {
			panic("buk error")
		})
		e.POST("/verify/api/login").WithForm(wrongPw).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect().
			Body().Contains("Internal Server Error")

		// 1.5 Should not found when apps name not provided
		e.POST("/verify/api/login").WithForm(wrongPw).Expect().
			Body().Contains("Not Found")

		card.DbUUID = tmpUuid
	})

	t.Run("2.GetLoginRecord", func(t *testing.T) {
		defaultLoginData := func() [][]byte {
			set := make([][]byte, 4)
			set[0] = []byte{0xf4, 0xf6, 0xd1, 0xae, 0x0c, 0x00, 0x00, 0x00}
			set[1] = []byte{0xc4, 0xf6, 0xd1, 0xae, 0x0c, 0x00, 0x00, 0x00}
			set[2] = []byte{0xf4, 0xe6, 0xd1, 0xae, 0x0c, 0x00, 0x00, 0x00}
			set[3] = []byte{0xf4, 0xf6, 0xc1, 0xae, 0x0c, 0x00, 0x00, 0x00}
			return set
		}
		loginSet := defaultLoginData()
		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range loginSet {
					var id []byte = card.MakeBytes(4, 0)
					binary.PutVarint(id, int64(i))
					if err := fn(id, v); err != nil {
						return err
					}
				}
				return nil
			})
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk)
		// 2.1 Should get the record
		retBody := e.GET("/verify/api/loginrec").Expect().Body().Raw()

		retMap := make(iris.Map)
		json.Unmarshal([]byte(retBody), &retMap)
		if dataMap := retMap["data"].([]interface{}); len(dataMap) != 4 {
			t.Fail()
		}
	})

	t.Run("3.DelLoginRec", func(t *testing.T) {
		idOK := "ecc74ecb-8d46-4faf-a651-5ac29451aa79"
		mockBuk.EXPECT().Delete([]byte(idOK)).Return(nil)
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk).Times(2)

		// 3.1 Should success delete login id
		e.DELETE("/verify/api/logindel/" + idOK).Expect().Body().Contains("OK")

		idFail := "5d7cd95f-4daf-487d-a39a-7e14e8e62ece"
		err := errors.New("del error")
		mockBuk.EXPECT().Delete([]byte(idFail)).Return(err)
		// 3.2 Should fail del when error returned
		e.DELETE("/verify/api/logindel/" + idFail).Expect().Body().Contains("FAIL").Contains(err.Error())
	})

	t.Run("4.1.CardData", func(t *testing.T) {
		cardSet := defaultCardSet()

		for i := 10; i < 40; i++ {
			cardSet[string([]byte{0x00, byte(i), 0x01, 0x00, 0x01, 0x00, 0x01})] =
				`{"link": "08000000-0000-0000-0000-000000000000", "filekey": "bb123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE="}`
		}
		for i := 41; i < 69; i++ {
			cardSet[string([]byte{0xb4, byte(i), 0x23, 0x45, 0x67, 0x89, 0xcd})] =
				`{"link": "50000000-0000-0000-0000-000000000000", "filekey": "cc123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE="}`
		}

		modelSet := defaultModelSet()

		mockBuk.EXPECT().Stats().Return(bbolt.BucketStats{KeyN: len(cardSet)})
		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range cardSet {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			})
		mockBuk.EXPECT().Get(gomock.Any()).Times(server.ListMax).
			DoAndReturn(func(k []byte) []byte {
				if string(k) == "08000000-0000-0000-0000-000000000000" {
					return nil
				} else {
					return []byte(modelSet["50000000-0000-0000-0000-000000000000"])
				}
			})
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk).Times(1 + server.ListMax)

		retBody := e.GET("/verify/api/carddata").WithQuery("page", 0).Expect().Body().Raw()

		retMap := make(iris.Map)
		json.Unmarshal([]byte(retBody), &retMap)
		dataMap := retMap["data"].([]interface{})
		// 4.1.1 Should return set is 50
		if len(dataMap) != 50 {
			t.Fail()
		}

		// 4.1.2 Should return have a record name with "No record"
		func() {
			for i := range dataMap {
				cur := dataMap[i].(iris.Map)
				if cur["name"].(string) == "No record" {
					return
				}
			}
			t.Fail()
		}()
	})

	t.Run("4.2.CardData-Fail", func(t *testing.T) {
		mockBuk.EXPECT().Stats().Return(bbolt.BucketStats{KeyN: 0})
		mockBuk.EXPECT().ForEach(gomock.Any()).Return(nil)
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk)

		// 4.2.1 Should return empty when no record
		e.GET("/verify/api/carddata").WithQuery("page", 0).Expect().Body().Contains("empty")

		// 4.2.2 Should return "Bad request"
		e.GET("/verify/api/carddata").Expect().Body().Contains("Bad request")
	})

	t.Run("5.WriteTag", func(t *testing.T) {
		// cardData := defaultCardSet()

		id := "BHcogk1lgA=="
		sign := "nHBoY2ooX-uh4kBmVACdzm6KnJzjGL4CYu0WJRm5cLMvuAKFhCDWZ9Es680yu0vIr-IcdTpabCY="
		link := "50000000-0000-0000-0000-000000000000"

		getForm := func() iris.Map {
			return iris.Map{
				"id":   id,
				"sign": sign,
				"link": link,
			}
		}
		bId := []byte{0x04, 0x77, 0x28, 0x82, 0x4d, 0x65, 0x80}

		// 5.1. Should input passed
		mockBuk.EXPECT().Get(bId).Return(nil).Times(2)
		mockBuk.EXPECT().Put(bId, gomock.Any()).Return(nil)
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(2)
		e.POST("/verify/api/cardwrite").WithForm(getForm()).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect()

		// 5.2. Should return fail message when db failed
		err := errors.New("db error")
		mockBuk.EXPECT().Put(bId, gomock.Any()).Return(err)
		e.POST("/verify/api/cardwrite").WithForm(getForm()).Expect().
			Body().Contains(err.Error())

		// 5.3 Should return fail when ID verify failed
		id = "IabY_VUo0w=="
		sign = "gXLvuhDhiE5JU2_CHq0a5VrwihQj2-Q8g2TEVlNGlLzvC_hb8ymg_4siEBOvhgl2qwKRVb2Vu84="
		e.POST("/verify/api/cardwrite").WithForm(getForm()).Expect().
			Body().Contains("FAIL")

		// 5.4 Should return fail when Sign decode failed
		sign = "WrongSign!"
		e.POST("/verify/api/cardwrite").WithForm(getForm()).Expect().
			Body().Contains("FAIL")

		// 5.5 Should return fail when ID decode failed
		id = "WrongID!"
		e.POST("/verify/api/cardwrite").WithForm(getForm()).Expect().
			Body().Contains("FAIL")
	})

	t.Run("6.CardEdit", func(t *testing.T) {
		cardSet := defaultCardSet()

		curCardId := []byte{0xa2, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd}
		curCard := cardSet[string(curCardId)]
		mockBuk.EXPECT().Put(curCardId, gomock.Any()).Return(nil).Times(2)
		mockBuk.EXPECT().Get(curCardId).Return([]byte(curCard)).Times(4)
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(4)

		// 6.1 Should checked the card
		form := iris.Map{
			"id": hex.EncodeToString(curCardId),
		}
		e.POST("/verify/api/cardchecked").WithForm(form).Expect().
			Body().Contains("OK").Contains(form["id"].(string))

		// 6.2 Should update the card password
		form = iris.Map{
			"id": hex.EncodeToString(curCardId),
			"pw": "aa123456789abcdefedcba9876543210",
		}
		e.POST("/verify/api/cardpwupdate").WithForm(form).Expect().
			Body().Contains("OK").Contains(form["id"].(string))

		// 6.3. Should delete the target card
		form = iris.Map{
			"id": hex.EncodeToString(curCardId),
			"pw": "03123456789abcdefedcba9876543210",
		}
		mockBuk.EXPECT().Delete(curCardId).Return(nil)
		e.POST("/verify/api/carddel").WithForm(form).Expect().
			Body().Contains("OK").Contains(form["id"].(string))

		// 6.4. Should fail when db panic
		mockBuk.EXPECT().Put(curCardId, gomock.Any()).Return(errors.New("delete"))
		e.POST("/verify/api/cardchecked").WithForm(form).Expect().
			Body().Contains("FAIL").Contains("error happen")

		// 6.5. Should fail with invalid id
		form = iris.Map{"id": "g1234567890abc"}
		e.POST("/verify/api/cardchecked").WithForm(form).Expect().
			Body().Contains("FAIL")

		// 6.6. Should fail with missed bytes id
		form = iris.Map{"id": "0123456789"}
		e.POST("/verify/api/cardchecked").WithForm(form).Expect().
			Body().Contains("FAIL")
	})

	t.Run("7.CardSearch", func(t *testing.T) {
		cardData := defaultCardSet()

		// 7.1 Should get the result
		// Search result for 7.1 & 7.2
		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range cardData {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).Times(2)
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(2)

		// Return link result
		mockBuk.EXPECT().Get(gomock.Any()).
			Return([]byte(`{"image": "i5", "desc": "info5", "name": "banana"}`))
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk)
		res := e.GET("/verify/api/cardsearch").WithQuery("key", "a2").Expect()
		res.Body().Contains("OK")

		retData7_1 := make(iris.Map)
		retBody7_1 := res.Body().Raw()
		json.Unmarshal([]byte(retBody7_1), &retData7_1)
		retItems7_1 := retData7_1["items"].([]interface{})
		if len(retItems7_1) != 1 {
			t.Fail()
		}

		// 7.2 Should fail when db error
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(nil)
		e.GET("/verify/api/cardsearch").WithQuery("key", "a2").
			Expect().Body().Contains("FAIL")

		// 7.3 Should return bad request with out query
		e.GET("/verify/api/cardsearch").Expect().Body().Contains("Bad request")

		// 7.4 Should return search id result existed
		mockBuk.EXPECT().Get(gomock.All()).
			Return([]byte(`{"link": "30000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`))
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(2)
		res = e.GET("/verify/api/cardsearch").WithQuery("id", "ou8jRWeJzQ==").Expect()
		res.JSON().Object().Value("msg").Equal("EXISTED")

		// 7.5 Should return search id result not exist and feedback allow cros for apps
		mockBuk.EXPECT().Get(gomock.All()).Return(nil)
		res = e.GET("/verify/api/cardsearch").WithQuery("id", "Vu8jRWeJzQ==").
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect()
		res.JSON().Object().Value("msg").Equal("NOTEXIST")
		res.Headers().ContainsKey("Access-Control-Allow-Credentials")

		// 7.6 Should fail when id is not base64 format
		res = e.GET("/verify/api/cardsearch").WithQuery("id", "Vu8[jRWeJzQ==").Expect()
		res.Status(400)

	})

	server.Close()
}

func TestAdminModelView(t *testing.T) {
	ctrl := gomock.NewController(t)
	server.UpdateLogio(log.Default())
	imgPath := filepath.Join("source", "img")

	// DB mock
	e, _, _, mockTx, mockBuk := makeHttpTest(t)

	t.Run("0.LoginReady", func(t *testing.T) {
		mockUuid := mock.NewMockUUID(ctrl)
		tmpUuid := card.DbUUID
		card.DbUUID = mockUuid

		formPw, uuidDummy, _, _ := loginMock(mockBuk, mockTx, mockUuid)
		mockBuk.EXPECT().Put([]byte(uuidDummy), gomock.Any()).Return(nil)
		// 0.1 Should pass the login check
		e.POST("/verify/api/login").WithForm(formPw).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect()

		card.DbUUID = tmpUuid
	})

	t.Run("1.ModelList", func(t *testing.T) {
		modelData := defaultModelSet()

		mockBuk.EXPECT().Stats().Return(bbolt.BucketStats{KeyN: len(modelData)}).Times(2)
		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range modelData {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).Times(2)
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk).Times(2)
		// 1.1 Should pass with same-site request
		res := e.GET("/verify/api/modellist").WithQuery("page", 0).Expect()
		res.Headers().NotContainsKey("Access-Control-Allow-Origin")

		retMap := make(iris.Map)
		json.Unmarshal([]byte(res.Body().Raw()), &retMap)
		if dataMap := retMap["items"].([]interface{}); len(dataMap) != 5 {
			t.Fail()
		}

		// 1.2 Should pass with apps request
		e.GET("/verify/api/modellist").WithQuery("page", 0).
			WithHeader("X-Requested-With", "com.mdl.arttagscanner").Expect()

		// // 1.3 Should fail when get list failed
		mockTx.EXPECT().Bucket(gomock.Any()).Return(nil)
		e.GET("/verify/api/modellist").WithQuery("page", 0).Expect().
			Body().Contains("FAIL")

		// // 1.4 Should fail when request
		e.GET("/verify/api/modellist").Expect().Body().Contains("Bad request")
	})

	t.Run("2.ModelImgClean", func(t *testing.T) {
		modelData := defaultModelSet()

		for i := 1; i <= 5; i++ {
			addr := filepath.Join(imgPath, "i"+strconv.Itoa(i))
			os.WriteFile(addr, []byte{}, 0600)
		}

		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range modelData {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			})
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk)
		e.POST("/verify/api/modelimgclean").Expect().Body().Contains("OK")
		if e, err := os.ReadDir(imgPath); err != nil || len(e) != 4 {
			t.Fail()
		}

		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(nil)
		e.POST("/verify/api/modelimgclean").Expect().Body().Contains("FAIL")

		os.RemoveAll(imgPath)

		e.POST("/verify/api/modelimgclean").Expect().
			Body().Contains("FAIL").Contains("Image path reading error")
	})

	t.Run("3.ModelWrite", func(t *testing.T) {
		os.MkdirAll(imgPath, 0770)
		mockUuid := mock.NewMockUUID(ctrl)
		tmpUuid := card.DbUUID
		card.DbUUID = mockUuid
		dummyUuid := "035fc933-3e1b-46b3-824f-dc161cabdc0b"

		form := iris.Map{
			"modelIdName": "New record",
			"modelDesc":   "New desc",
		}

		jsonSrc3_1 := "{\"desc\":\"New desc\",\"image\":\"867e65d3b7cf6eb7318aab7fd0aa3bed179ba9fffd4feb39c622d588e1c751a2\",\"name\":\"New record\"}"
		mockUuid.EXPECT().NewV4().Return(uuid.FromString(dummyUuid))
		mockBuk.EXPECT().Get(gomock.Any()).Return(nil)
		mockBuk.EXPECT().Put([]byte(dummyUuid), []byte(jsonSrc3_1)).Return(nil)
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk).Times(2)
		// 3.1.1. Should success write model
		e.POST("/verify/api/modelwrite").WithMultipart().WithForm(form).
			WithFileBytes("modelImg", "someimg.png", []byte("emupng")).
			Expect().Body().Contains("OK")
		os.RemoveAll(imgPath)

		// 3.1.2. Should failed when image path not exist
		e.POST("/verify/api/modelwrite").WithMultipart().WithForm(form).
			WithFileBytes("modelImg", "someimg.png", []byte("emupng")).
			Expect().Body().Contains("FAIL")

		// 3.2. Should fail uuid repeated
		mockUuid.EXPECT().NewV4().Return(uuid.FromString(dummyUuid))
		mockBuk.EXPECT().Get([]byte(dummyUuid)).
			Return([]byte(`{"image": "i5", "desc": "info5", "name": "banana"}`))
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk)
		e.POST("/verify/api/modelwrite").WithForm(form).Expect().
			Body().Contains("FAIL").Contains("database error")

		// 3.3 Should fail with any error when write database
		mockUuid.EXPECT().NewV4().Return(uuid.UUID{}, errors.New("any error"))
		e.POST("/verify/api/modelwrite").WithForm(form).Expect().
			Body().Contains("FAIL").Contains("any error")

		// 3.4 Should copy any record when not updated
		oldForm := iris.Map{
			"modelIdName": "",
			"modelDesc":   "",
			"modelId":     dummyUuid,
		}

		jsonSrc3_4 := "{\"desc\":\"info5\",\"image\":\"i5\",\"name\":\"banana\"}"
		mockBuk.EXPECT().Get([]byte(dummyUuid)).Return([]byte(jsonSrc3_4))
		mockBuk.EXPECT().Put([]byte(dummyUuid), []byte(jsonSrc3_4)).Return(nil)
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk).Times(2)
		e.POST("/verify/api/modelwrite").WithForm(oldForm).Expect().
			Body().Contains("OK")

		// 3.5 Should fail with "id" and "name" are empty
		failForm1 := iris.Map{
			"modelIdName": "",
			"modelDesc":   "",
			"modelId":     "",
		}
		e.POST("/verify/api/modelwrite").WithForm(failForm1).Expect().
			Body().Contains("FAIL").Contains("name is empty")

		// 3.6 Should pass to delete image
		delForm1 := iris.Map{
			"modelId":  dummyUuid,
			"modelDel": "1",
		}
		cardData := defaultCardSet()
		mockBuk.EXPECT().Get([]byte(dummyUuid)).Return([]byte(jsonSrc3_4)).Times(2)
		mockBuk.EXPECT().Delete([]byte(dummyUuid)).Return(nil)
		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range cardData {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).Times(2)
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk).Times(2)
		mockTx.EXPECT().Bucket([]byte(card.DB_CARD)).Return(mockBuk).Times(2)
		e.POST("/verify/api/modelwrite").WithForm(delForm1).Expect().
			Body().Contains("OK").Contains("removed")

		// 3.7 Should fail delete when model have linked card
		cardData[string([]byte{0xb8, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
			`{"link": "` + dummyUuid + `", "filekey": "05123456789abcdefedcba9876543210", "sign": "5678", "ctr": "AAE="}`
		e.POST("/verify/api/modelwrite").WithForm(delForm1).Expect().
			Body().Contains("FAIL")

		// 3.8 Should fail delete when other error
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(nil)
		e.POST("/verify/api/modelwrite").WithForm(delForm1).Expect().
			Body().Contains("FAIL")

		// 3.9 Should fail delete when id missed
		delForm2 := iris.Map{
			"modelId":  "",
			"modelDel": "1",
		}
		e.POST("/verify/api/modelwrite").WithForm(delForm2).Expect().
			Body().Contains("FAIL")

		card.DbUUID = tmpUuid
	})

	t.Run("4.ModelSearch", func(t *testing.T) {
		modelData := defaultModelSet()

		// 4.1 Should get the model from search result
		mockBuk.EXPECT().ForEach(gomock.Any()).DoAndReturn(
			func(fn func(k, v []byte) error) error {
				for i, v := range modelData {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			})
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk)
		res := e.GET("/verify/api/modelsearch").WithQuery("key", "banana").Expect()
		res.Body().Contains("OK")

		retData4_1 := make(iris.Map)
		retBody4_1 := res.Body().Raw()
		json.Unmarshal([]byte(retBody4_1), &retData4_1)
		retItems4_1 := retData4_1["items"].([]interface{})
		if len(retItems4_1) != 1 {
			t.Fail()
		}

		// 4.2 Should failed request without key
		e.GET("/verify/api/modelsearch").Expect().Body().Contains("Bad request")
	})

	t.Run("5.corsPreflight", func(t *testing.T) {
		headerReq := map[string]string{
			"X-Requested-With": "com.mdl.arttagscanner",
		}
		headerRes := iris.Map{
			"Access-Control-Allow-Credentials": []string{"true"},
			"Access-Control-Allow-Headers":     []string{"X-token, X-Requested-With"},
			"Access-Control-Allow-Methods":     []string{"GET, POST, OPTIONS"},
		}
		e.OPTIONS("/verify/api/cardwrite").WithHeaders(headerReq).
			Expect().Headers().ContainsMap(headerRes)
		e.OPTIONS("/verify/api/modellist").WithHeaders(headerReq).
			Expect().Headers().ContainsMap(headerRes)
		e.OPTIONS("/verify/api/cardsearch").WithHeaders(headerReq).
			Expect().Headers().ContainsMap(headerRes)
		e.OPTIONS("/verify/api/cardwrite").Expect().Body().Contains("Not Found")
	})

	server.Close()
}

func TestAdminToken(t *testing.T) {
	logsub := log.Default()
	server.UpdateLogio(logsub)

	t.Run("1.AccessWithToken", func(t *testing.T) {
		// DB mock
		e, _, _, mockTx, mockBuk := makeHttpTest(t)
		tokenId := "b5b18a10-7784-420e-bab4-1c1a5efa3402"

		headerReq := map[string]string{
			"X-Requested-With": "com.mdl.arttagscanner",
			"X-Token":          tokenId,
		}

		timeRaw := make([]byte, 8)
		binary.PutVarint(timeRaw, time.Now().Unix())
		mockBuk.EXPECT().Get([]byte(tokenId)).Return(timeRaw).Times(2)
		mockTx.EXPECT().Bucket([]byte("admin-record")).Return(mockBuk).Times(2)

		mockBuk.EXPECT().Stats().Return(bbolt.BucketStats{KeyN: 0}).Times(2)
		mockBuk.EXPECT().ForEach(gomock.Any()).Return(nil).Times(2)
		mockTx.EXPECT().Bucket([]byte(card.DB_LINK)).Return(mockBuk).Times(2)

		// 1.1. Should get the session from DB
		e.GET("/verify/api/modellist").WithHeaders(headerReq).
			WithQuery("page", 0).Expect()

		// 1.2. Should also get the seesion without run more process
		e.GET("/verify/api/modellist").WithHeaders(headerReq).
			WithQuery("page", 0).Expect()

		// 1.3. Should return not found when apps session check failed
		mockBuk.EXPECT().Get([]byte(tokenId)).Return(nil)
		mockTx.EXPECT().Bucket([]byte("admin-record")).Return(mockBuk)
		e.GET("/verify/api/modellist").WithHeaders(headerReq).
			WithQuery("page", 0).Expect().Body().Contains("Not Found")

		// 1.4. Should return not found when apps try to aceess other api
		e.GET("/verify/admin").WithHeaders(headerReq).
			WithQuery("page", 0).Expect().Body().Contains("Not Found")

		// 1.5. Should success change the time config
		if server.AppSess.ChangeTime(time.Minute*10, time.Microsecond) != nil {
			t.Errorf("Should success to change time")
		}

		// 1.6. Should fail change time when it have 0 value
		if server.AppSess.ChangeTime(0, time.Minute*10) == nil {
			t.Errorf("Should failed when 0 value")
		}

		// 1.7. Should fail when any panic captured
		mockTx.EXPECT().Bucket(gomock.Any()).DoAndReturn(
			func(v []byte) card.IBucket {
				panic("any panic")
			})
		e.GET("/verify/api/modellist").WithHeaders(headerReq).
			WithQuery("page", 0).Expect().Body().Contains("500 Internal Server Error")

		buf := bytes.Buffer{}
		logsub.SetOutput(&buf)
		// 1.8 Should remove the old session after kill
		mockTx.EXPECT().Bucket([]byte("admin-record")).Return(nil)
		server.AppSess.Kill()

		t.Logf("is: %t", strings.Contains(buf.String(), "auto session update failed"))

		server.Close()
	})
}

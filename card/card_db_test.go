// The test file for "card_db.go"
// Some of not cover branch is about JSON marshal,
// when mock it will make the process compilcated so will ignore it.
package card_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	uuid "github.com/iris-contrib/go.uuid"
	"go.etcd.io/bbolt"
)

// func MakeTempPath() string {
// 	d, err := os.MkdirTemp("", "tag-server")
// 	if err != nil {
// 		panic(d)
// 	}
// 	return d
// }

func makeDefultDB(ctrl *gomock.Controller) *mock.MockIDB {
	ntx := mock.NewMockITx(ctrl)
	ntx.EXPECT().CreateBucketIfNotExists(gomock.Any()).AnyTimes()

	return makeDefultDBWithITx(ctrl, ntx)
}

func makeDefultDBWithITx(ctrl *gomock.Controller, ntx *mock.MockITx) *mock.MockIDB {
	db := mock.NewMockIDB(ctrl)
	db.EXPECT().Update(gomock.Any()).
		DoAndReturn(func(fn func(tx card.ITx) error) error {
			return fn(ntx)
		}).AnyTimes()
	db.EXPECT().Close().AnyTimes()
	return db
}

// defaultModelSet to make default linkdb
func defaultModelSet() map[string]string {
	set := make(map[string]string)
	set["01234567-89ab-cdef-0123-456789abcdef"] = `{"image": "i1", "desc": "info1", "name": "foo"}`
	set["20000000-0000-0000-0000-000000000000"] = `{"image": "i2", "desc": "info2", "name": "bar"}`
	set["30000000-0000-0000-0000-000000000000"] = `{"image": "i3", "desc": "info3", "name": "alice"}`
	set["40000000-0000-0000-0000-000000000000"] = `{"image": "i4", "desc": "info4", "name": "boo"}`
	set["50000000-0000-0000-0000-000000000000"] = `{"image": "i5", "desc": "info5", "name": "banana"}`
	return set
}

// defaultCardSet to make default carddb
func defaultCardSet() map[string]string {
	set := make(map[string]string)
	set[string([]byte{0x00, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "30000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0x01, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "20000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0xa2, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "40000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0xa3, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "60000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
	set[string([]byte{0xb4, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
		`{"link": "70000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "AAE="}`
	return set
}

func TestModelInfo(t *testing.T) {
	model, nft, image, id := "model", "nftlink", "imagelink", "01234567-0123-4567-89ab-cdef01234567"
	info, err := card.NewModelInfo(model, nft, image, id)
	if err != nil {
		t.Errorf("Failed to crate model info")
	}
	if info.Name != model || info.Desc != nft || info.Image != image || info.GetID() != id {
		t.Fail()
	}
	if info.GetID() != id {
		t.Fail()
	}
	if info.SetID("00000000-1234-5678-9abc-01234567890a"); info.GetID() != "00000000-1234-5678-9abc-01234567890a" {
		t.Fail()
	}

	info, err = card.NewModelInfo(model, nft, image, "")
	if err != nil || len(info.GetID()) != 36 {
		t.Fail()
	}
}

func TestModelInfo_BadID(t *testing.T) {
	model, nft, image, id := "model", "nftlink", "imagelink", "z1234567-0123-4567-89ab-cdef01234567"
	_, err := card.NewModelInfo(model, nft, image, id)
	if err == nil {
		t.Errorf("Should failed when read wrong id")
	}
}

func TestCreateCardDB(t *testing.T) {
	t.Run("A1.Should run and create DB", func(t *testing.T) {

		ctrl := gomock.NewController(t)
		card.DbOpen = func(path string, mode os.FileMode, options *bbolt.Options) (card.IDB, error) {
			return makeDefultDB(ctrl), nil
		}

		dbpath := MakeTempPath()
		defer os.RemoveAll(dbpath)
		db, err := card.CreateCardDB(dbpath)
		if err != nil || db == nil {
			t.Errorf("Failed to create DB")
		}
		defer db.Close()
	})

	t.Run("A2.Should fail on create bucket", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		bukerror := errors.New("BucketError")
		ntx := mock.NewMockITx(ctrl)
		ntx.EXPECT().CreateBucketIfNotExists(gomock.Any()).Return(nil, bukerror)
		dbmock := makeDefultDBWithITx(ctrl, ntx)
		card.DbOpen = func(path string, mode os.FileMode, options *bbolt.Options) (card.IDB, error) {
			return dbmock, nil
		}

		if _, err := card.CreateCardDB("/test/path"); err != bukerror {
			t.Fail()
		}
	})

	t.Run("A3.Should capture the error on create", func(t *testing.T) {
		card.DbOpen = func(path string, mode os.FileMode, options *bbolt.Options) (card.IDB, error) {
			return nil, errors.New("some error")
		}

		db, err := card.CreateCardDB("/not/exist/path")
		if err == nil {
			t.Fail()
		} else {
			return
		}
		defer db.Close()
	})

	t.Run("A4.Should fail on update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		var mockNum = 0
		card.DbOpen = func(path string, mode os.FileMode, options *bbolt.Options) (card.IDB, error) {
			if mockNum == 0 {
				mockNum++
				db := mock.NewMockIDB(ctrl)
				// Stub only Update
				db.EXPECT().Update(gomock.Any())
				return db, nil
			}
			return nil, errors.New("second run error")
		}

		db, err := card.CreateCardDB("some/path")
		if err == nil {
			t.Fail()
		} else {
			return
		}
		db.Close()
	})

	t.Run("A5.Should Throw error when empty path", func(t *testing.T) {
		if _, err := card.CreateCardDB(""); err == nil {
			t.Errorf(`Should Throw error when empty path`)
		}
	})

	t.Run("A6.Should throw error when db nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := mock.NewMockIDB(ctrl)
		if _, err := card.CreateCardDBRaw(nil, mockDB); err != card.ErrCardDBRead {
			t.Errorf(`Should throw error when db nil`)
		}
		if _, err := card.CreateCardDBRaw(mockDB, nil); err != card.ErrCardDBRead {
			t.Errorf(`Should throw error when db nil`)
		}
	})
}

func TestDBModel(t *testing.T) {
	ctrl := gomock.NewController(t)

	var dataSet map[string]string = make(map[string]string)
	var mInput map[string]bool = make(map[string]bool)

	var db *card.CardDatabase

	// A. ModelGetRemainImages
	t.Run("A.ModelGetRemainImages", func(t *testing.T) {
		bukMock := mock.NewMockIBucket(ctrl)
		txMock := mock.NewMockITx(ctrl)
		cardDbMock := makeDefultDB(ctrl)
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)
		dataSet = defaultModelSet()

		// A1. Should not error when nil image list returned
		// Stub view
		cardDbMock.EXPECT().View(gomock.Any()).DoAndReturn(func(fn func(tx card.ITx) error) error {
			return fn(txMock)
		}).AnyTimes()
		bukMock.EXPECT().ForEach(gomock.Any()).Return(nil).Times(1)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		if err := db.ModelGetRemainImages(mInput); err != nil {
			t.Errorf("Should not error when nil image list returned")
		}

		// A2. Should removed the data with key "i1"

		bukMock = mock.NewMockIBucket(ctrl)
		txMock = mock.NewMockITx(ctrl)
		bukMock.EXPECT().ForEach(gomock.Any()).
			DoAndReturn(func(fn func(k, v []byte) error) error {
				for i, v := range dataSet {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).AnyTimes()
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()

		mInput["i1"] = true
		if err := db.ModelGetRemainImages(mInput); err != nil &&
			len(mInput) != 1 && mInput["i1"] {
			t.Errorf(`Should removed the data with key "i1"`)
		}

		// A3. Should failed when invalid json inputed
		dataSet["006"] = `Not JSON`
		mInput["i1"] = true
		if err := db.ModelGetRemainImages(mInput); err == nil {
			t.Errorf("Should failed when invalid json inputed")
		}
		delete(dataSet, "006")

		// A4. Should failed when return a nil bucket
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if err := db.ModelGetRemainImages(mInput); err != card.ErrCardDBNotExists {
			t.Errorf("Should failed when return a nil bucket")
		}
	})

	// B. ModelSearchJSON
	t.Run("B.ModelSearchJSON", func(t *testing.T) {
		bukMock := mock.NewMockIBucket(ctrl)
		txMock := mock.NewMockITx(ctrl)
		cardDbMock := makeDefultDB(ctrl)
		dataSet = defaultModelSet()
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)

		cardDbMock.EXPECT().View(gomock.Any()).DoAndReturn(func(fn func(tx card.ITx) error) error {
			return fn(txMock)
		}).AnyTimes()
		bukMock.EXPECT().ForEach(gomock.Any()).
			DoAndReturn(func(fn func(k, v []byte) error) error {
				for i, v := range dataSet {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).AnyTimes()

		// B1.1. Should found key "bar"
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		if v := db.ModelSearchJSON("bar"); len(v) != 1 && v[0]["name"].(string) == "bar" {
			t.Errorf(`Should found key "bar"`)
		}

		// B1.2. Should found 3 object when key "b"
		if v := db.ModelSearchJSON("b"); len(v) != 3 {
			t.Errorf(`Should found 3 object when key "b"`)
		}

		// B2. Should failed when return invalid JSON
		dataSet["006"] = `Not JSON`
		if v := db.ModelSearchJSON("Not"); v != nil {
			t.Errorf(`Should failed when return invalid JSON`)
		}
		delete(dataSet, "006")

		// B3. Should return max 50 result
		for i := 0; i < 55; i++ {
			surfix := strconv.Itoa(i)
			dataSet["iTest"+surfix] = `{"image": "test` + surfix + `", "desc": "info", "name": "many` + surfix + `"}`
		}
		if v := db.ModelSearchJSON("many"); len(v) > 50 {
			t.Errorf(`Should return max 50 result`)
		}

		// B4. Should failed when return a nil bucket
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if ret := db.ModelSearchJSON(""); ret != nil {
			t.Errorf("Should failed when return a nil bucket")
		}
	})

	// C. ModelLinkListJSON
	t.Run("C.ModelLinkListJSON", func(t *testing.T) {
		bukMock := mock.NewMockIBucket(ctrl)
		txMock := mock.NewMockITx(ctrl)
		cardDbMock := makeDefultDB(ctrl)
		dataSet = defaultModelSet()

		bukMock.EXPECT().ForEach(gomock.Any()).
			DoAndReturn(func(fn func(k, v []byte) error) error {
				for i, v := range dataSet {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).AnyTimes()
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		cardDbMock.EXPECT().View(gomock.Any()).DoAndReturn(func(fn func(tx card.ITx) error) error {
			return fn(txMock)
		}).AnyTimes()
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)

		// C1. Should get the list, will use last list
		bukMock.EXPECT().Stats().Return(bbolt.BucketStats{KeyN: len(dataSet)}).AnyTimes()
		if v, s := db.ModelLinkListJSON(0, 30); len(v) == 0 && s == 0 {
			t.Errorf(`Should get the list, will use last list`)
		}

		// C2. Should failed when return invalid JSON
		dataSet["007"] = `Not JSON`
		if v, _ := db.ModelLinkListJSON(0, 30); v != nil {
			t.Errorf(`Should failed when return invalid JSON`)
		}
		delete(dataSet, "007")

		// C3. Should get max page content only
		dataSet["60000000-0000-0000-0000-000000000000"] = `{"image": "i6", "desc": "info6", "name": "banana"}`
		dataSet["70000000-0000-0000-0000-000000000000"] = `{"image": "i7", "desc": "info7", "name": "banana"}`
		dataSet["80000000-0000-0000-0000-000000000000"] = `{"image": "i8", "desc": "info8", "name": "banana"}`
		dataSet["90000000-0000-0000-0000-000000000000"] = `{"image": "i9", "desc": "info9", "name": "banana"}`
		dataSet["01000000-0000-0000-0000-000000000000"] = `{"image": "i10", "desc": "info10", "name": "banana"}`
		if v, _ := db.ModelLinkListJSON(0, 5); v != nil && len(v) != 5 {
			t.Errorf(`Should get max page content only`)
		}

		// C4. Should failed when return a nil bucket
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if v, _ := db.ModelLinkListJSON(0, 30); len(v) != 0 {
			t.Errorf("Should failed when return a nil bucket")
		}
	})

	// D. ModelGetLink
	t.Run("D.ModelGetLink", func(t *testing.T) {
		dataSet = defaultModelSet()
		validId := "01234567-89ab-cdef-0123-456789abcdef"
		bukMock := mock.NewMockIBucket(ctrl)
		txMock := mock.NewMockITx(ctrl)
		cardDbMock := makeDefultDB(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		cardDbMock.EXPECT().View(gomock.Any()).DoAndReturn(func(fn func(tx card.ITx) error) error {
			return fn(txMock)
		}).AnyTimes()
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)

		// D1. Should match result with dataset
		bukMock.EXPECT().Get(gomock.Any()).Return([]byte(dataSet[validId]))
		if v, err := db.ModelGetLink(validId); err != nil || v.GetID() != validId || v.Desc != "info1" || v.Name != "foo" {
			t.Errorf(`Should match result with dataset`)
		}

		// D2. Should failed invalid JSON
		bukMock.EXPECT().Get(gomock.Any()).Return([]byte(`Not JSON`))
		if _, err := db.ModelGetLink(validId); err == nil {
			t.Errorf(`Should failed invalid JSON`)
		}

		// D3. Should failed when return a nil bucket
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if _, err := db.ModelGetLink(validId); err != card.ErrCardDBNotExists {
			t.Errorf("Should failed when return a nil bucket")
		}
	})

	// E. ModelAddLink
	t.Run("E.ModelAddLink", func(t *testing.T) {
		validId := "01234567-89ab-cdef-0123-456789abcdef"
		// E1. Should add link success
		buklinkMock := mock.NewMockIBucket(ctrl)
		buklinkMock.EXPECT().Put(gomock.Eq([]byte(validId)), gomock.Any()).Return(nil)

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Eq([]byte("link-data"))).Return(buklinkMock).AnyTimes()

		cardDbMock := makeDefultDBWithITx(ctrl, txMock)
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)

		info, _ := card.NewModelInfo("foobar", "info", "i10", validId)
		if err := db.ModelAddLink(info); err != nil {
			t.Errorf(`Should add link success`)
		}

		// E2. Should fail to add link with empty id
		info.SetID("")
		if err := db.ModelAddLink(info); err == nil {
			t.Errorf(`Should fail to add link with empty id`)
		}

		// E3. Should failed when return a nil bucket
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		cardDbMock = makeDefultDBWithITx(ctrl, txMock)
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)
		if err := db.ModelAddLink(info); err != card.ErrCardDBNotExists {
			t.Errorf("Should failed when return a nil bucket")
		}
	})

	// F. ModelDelLink
	t.Run("F.ModelDelLink", func(t *testing.T) {
		validId := "01234567-89ab-cdef-0123-456789abcdef"
		cardSet := defaultCardSet()

		buklinkMock := mock.NewMockIBucket(ctrl)
		buklinkMock.EXPECT().Get(gomock.All()).
			DoAndReturn(func(id []byte) []byte {
				modelset := defaultModelSet()
				if modelset[string(id)] != "" {
					return []byte("ok")
				} else {
					return nil
				}
			}).AnyTimes()
		buklinkMock.EXPECT().Delete(gomock.Eq([]byte(validId))).Return(nil)
		bukcardMock := mock.NewMockIBucket(ctrl)
		bukcardMock.EXPECT().ForEach(gomock.All()).
			DoAndReturn(func(fn func(k, v []byte) error) error {
				for i, v := range cardSet {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).AnyTimes()

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Eq([]byte("link-data"))).Return(buklinkMock).AnyTimes()
		txMock.EXPECT().Bucket([]byte(card.DB_LINK)).Return(buklinkMock).AnyTimes()
		txMock.EXPECT().Bucket([]byte(card.DB_CARD)).Return(bukcardMock).AnyTimes()

		cardDbMock := makeDefultDBWithITx(ctrl, txMock)
		cardDbMock.EXPECT().Batch(gomock.Any()).
			DoAndReturn(func(fn func(tx card.ITx) error) error {
				return fn(txMock)
			}).AnyTimes()
		db, _ = card.CreateCardDBRaw(makeDefultDB(ctrl), cardDbMock)

		// F1. Should delete success
		info, _ := card.NewModelInfo("foobar", "info", "i10", validId)
		if err := db.ModelDelLink(info); err != nil {
			t.Errorf("Should delete success")
		}

		// F2. Should fail on invalid JSON record
		cardSet[string([]byte{0xaa, 0xbb})] = `Not JSON`
		if err := db.ModelDelLink(info); err == nil {
			t.Errorf("Should fail on invalid JSON record")
		}
		delete(cardSet, string([]byte{0xaa, 0xbb}))

		// F3. Should fail when del model have link card data
		info.SetID("20000000-0000-0000-0000-000000000000")
		if err := db.ModelDelLink(info); err != card.ErrCardModelDelLinking {
			t.Errorf("Should fail when del model have link card data")
		}

		// F4. Should fail del a not exist id
		info.SetID("")
		if err := db.ModelDelLink(info); err == nil {
			t.Errorf("Should fail del a not exist id")
		}

		// F5. Database access test
		// F5.1 Should failed carddb nil
		info.SetID(validId)
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket([]byte(card.DB_LINK)).Return(buklinkMock)
		txMock.EXPECT().Bucket([]byte(card.DB_CARD)).Return(nil)
		if err := db.ModelDelLink(info); err != card.ErrCardDBNotExists {
			t.Errorf("Should failed carddb nil")
		}

		// F5.2 Should failed linkdb nil
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket([]byte(card.DB_LINK)).Return(nil)
		if err := db.ModelDelLink(info); err != card.ErrCardDBNotExists {
			t.Errorf("Should failed linkdb nil")
		}
	})
}

func TestLogin(t *testing.T) {
	ctrl := gomock.NewController(t)

	defaultLoginData := func() [][]byte {
		set := make([][]byte, 4)
		set[0] = []byte{0xf4, 0xf6, 0xd1, 0xae, 0x0c, 0x00, 0x00, 0x00}
		set[1] = []byte{0xc4, 0xf6, 0xd1, 0xae, 0x0c, 0x00, 0x00, 0x00}
		set[2] = []byte{0xf4, 0xe6, 0xd1, 0xae, 0x0c, 0x00, 0x00, 0x00}
		set[3] = []byte{0xf4, 0xf6, 0xc1, 0xae, 0x0c, 0x00, 0x00, 0x00}
		return set
	}
	loginSet := defaultLoginData()

	t.Run("A.GetLoginRecord", func(t *testing.T) {
		bukMock := mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().ForEach(gomock.Any()).
			DoAndReturn(func(fn func(k, v []byte) error) error {
				for i, v := range loginSet {
					var id []byte = card.MakeBytes(4, 0)
					binary.PutVarint(id, int64(i))
					if err := fn(id, v); err != nil {
						return err
					}
				}
				return nil
			}).AnyTimes()

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()

		recDbMock := makeDefultDBWithITx(ctrl, txMock)
		recDbMock.EXPECT().View(gomock.Any()).
			DoAndReturn(func(fn func(tx card.ITx) error) error {
				return fn(txMock)
			}).AnyTimes()

		// A1. Shuold check dataset OK
		db, _ := card.CreateCardDBRaw(recDbMock, makeDefultDB(ctrl))
		if rec := db.GetLoginRecord(); rec == nil && len(rec) < 4 {
			t.Errorf(`Shuold check dataset OK`)
		}

		// A2. Error when read data failed
		loginSet = append(loginSet, []byte{0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
		if rec := db.GetLoginRecord(); rec != nil {
			t.Errorf(`Error when read data failed`)
		}

		// A3. Should failed on bucket nil
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if rec := db.GetLoginRecord(); rec != nil {
			t.Errorf(`Should failed on bucket nil`)
		}
	})

	t.Run("B.RemoveLoginRecord", func(t *testing.T) {
		// B1. Should pass when id match
		id := "001"

		bukMock := mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().Delete([]byte(id)).Return(nil)

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock)
		recDbMock := makeDefultDBWithITx(ctrl, txMock)
		db, _ := card.CreateCardDBRaw(recDbMock, makeDefultDB(ctrl))

		if err := db.RemoveLoginRecord(id); err != nil {
			t.Errorf(`Should pass when id match`)
		}

		// B2. Should failed on bucket nil
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if err := db.RemoveLoginRecord(id); err != card.ErrCardDBNotExists {
			t.Errorf(`Should failed on bucket nil`)
		}
	})

	t.Run("C.CheckLoginSession", func(t *testing.T) {
		id := "001"
		idTime := []byte{0x80, 0x9c, 0xb5, 0xae, 0x0c, 0x00, 0x00, 0x00}

		bukMock := mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().Get([]byte(id)).Return(idTime).AnyTimes()

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()

		recDbMock := makeDefultDBWithITx(ctrl, txMock)
		recDbMock.EXPECT().View(gomock.Any()).
			DoAndReturn(func(fn func(tx card.ITx) error) error {
				return fn(txMock)
			}).AnyTimes()
		db, _ := card.CreateCardDBRaw(recDbMock, makeDefultDB(ctrl))

		var (
			tDur   time.Duration = 604800000000000 //nanosceond
			tInput time.Time     = time.Unix(1659283200, 0)
		)

		mockClock := mock.NewMockClock(ctrl)
		card.DbClock = mockClock

		// C1.1. Should login success
		t.Logf("Duration in hour test:%f", tDur.Hours())
		mockClock.EXPECT().Since(tInput).Return(tDur)
		if sess, err := db.CheckLoginSession(id); err != nil || sess == 0 {
			t.Errorf(`Should login success C1.1.`)
		}

		// C1.2. Should login success
		tDur -= 10000000000000
		t.Logf("Duration in hour test:%f", tDur.Hours())
		mockClock.EXPECT().Since(tInput).Return(tDur)
		if sess, err := db.CheckLoginSession(id); err != nil || sess == 0 {
			t.Errorf(`Should login success C1.2.`)
		}

		// C2. Should failed timeout
		tDur = 604810000000000
		t.Logf("Duration in hour test:%f", tDur.Hours())
		mockClock.EXPECT().Since(tInput).Return(tDur)
		if _, err := db.CheckLoginSession(id); err == nil {
			t.Errorf(`Should failed timeout`)
		}

		// C3. Should failed bucket get return invalid value
		bukMock = mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().Get(gomock.Any()).Return([]byte{
			0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		if _, err := db.CheckLoginSession(id); err == nil {
			t.Errorf(`Should failed bucket get return invalid value`)
		}

		// C4. Should failed bucket get return nil
		bukMock.EXPECT().Get(gomock.Any()).Return(nil)
		if _, err := db.CheckLoginSession(id); err == nil {
			t.Errorf(`Should failed bucket get return nil`)
		}

		// C5. Should failed nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		if _, err := db.CheckLoginSession(id); err == nil {
			t.Errorf(`Should failed nil bucket returned`)
		}
	})

	t.Run("D.OnLoginUser", func(t *testing.T) {
		bukMock := mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().Put(gomock.All(), gomock.Any()).Return(nil)

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock)

		recDbMock := makeDefultDBWithITx(ctrl, txMock)
		db, _ := card.CreateCardDBRaw(recDbMock, makeDefultDB(ctrl))

		// D1. Should get the id without error
		if id, err := db.OnLoginUser(); err != nil || id == "" {
			t.Errorf(`Should get the id without error`)
		}

		// D2. Should failed nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		recDbMock = makeDefultDBWithITx(ctrl, txMock)
		db, _ = card.CreateCardDBRaw(recDbMock, makeDefultDB(ctrl))
		if _, err := db.OnLoginUser(); err == nil {
			t.Errorf(`Should failed nil bucket returned`)
		}
	})

	t.Run("E.genPW", func(t *testing.T) {
		var (
			id = "sampleuser"
			pw = "samplepw"
		)

		// E1. Should fit the hash result, GenPWHash
		hash := card.GenPWHash(id, pw)
		if hash != "fa0cbc50029b7a9eddc677aceb0b29ec7bab52958450520034270ac42e9754e3" {
			t.Errorf(`Should fit the hash result`)
		}
	})

	t.Run("F.ChangeAdminPW", func(t *testing.T) {
		var (
			orgid = "sampleuser"
			orgpw = "samplepw"
			newid = "newuser"
			newpw = "newpw"
			salt  = []byte("samplesalt")

			hashRet = []byte("c848f9e0c52559d77a9408959a6caa48b799b133d23dbd0be6ca2239dc8b0151")

			dbAdminHashID = "rec"
		)

		bukMock := mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().Put([]byte("salt"), gomock.Any()).Return(nil)
		bukMock.EXPECT().Put([]byte(dbAdminHashID), gomock.Any()).Return(nil)
		bukMock.EXPECT().Get([]byte("salt")).Return(salt).AnyTimes()
		bukMock.EXPECT().Get([]byte(dbAdminHashID)).Return(hashRet).AnyTimes()

		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()

		dbMockFn := func() card.IDB {
			dbm := makeDefultDBWithITx(ctrl, txMock)
			dbm.EXPECT().View(gomock.Any()).
				DoAndReturn(func(fn func(tx card.ITx) error) error {
					return fn(txMock)
				}).AnyTimes()
			return dbm
		}
		db, _ := card.CreateCardDBRaw(dbMockFn(), mock.NewMockIDB(ctrl))

		// F1. Should success changed the password
		if err := db.ChangeAdminPW(newid, newpw, orgid, orgpw); err != nil {
			t.Errorf(`Should success changed the password`)
		}

		// F2. Should fail on orignal id & pw failed
		if err := db.ChangeAdminPW(newid, newpw, orgid, "wrongPw"); err != card.ErrCardAdminFail {
			t.Errorf(`Should fail on orignal id & pw failed`)
		}

		// F3. Should fail on put hash error
		bukMock.EXPECT().Put([]byte("salt"), gomock.Any()).Return(nil)
		bukMock.EXPECT().Put([]byte(dbAdminHashID), gomock.Any()).Return(errors.New("some error"))
		if err := db.ChangeAdminPW(newid, newpw, orgid, orgpw); err != card.ErrCardDBWrite {
			t.Errorf(`Should fail on put hash error`)
		}

		// F4. Should fail on put salt error
		bukMock.EXPECT().Put([]byte("salt"), gomock.Any()).Return(errors.New("some error"))
		if err := db.ChangeAdminPW(newid, newpw, orgid, orgpw); err != card.ErrCardDBWrite {
			t.Errorf(`Should fail on put hash error`)
		}

		// F5. Should fail on get salt error
		bukMock = mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().Get([]byte(dbAdminHashID)).Return(hashRet)
		bukMock.EXPECT().Get([]byte("salt")).Return(nil)
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).Times(2)
		db, _ = card.CreateCardDBRaw(dbMockFn(), mock.NewMockIDB(ctrl))
		if err := db.ChangeAdminPW(newid, newpw, orgid, orgpw); err != card.ErrCardAdminFail {
			t.Errorf(`Should fail on get db salt failed`)
		}

		// F6. Should fail on return nil bucket
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(dbMockFn(), mock.NewMockIDB(ctrl))
		if err := db.ChangeAdminPW(newid, newpw, orgid, orgpw); err != card.ErrCardDBNotExists {
			t.Errorf(`Should fail on return nil bucket`)
		}
	})

	// Skip check ChangeAdminPW with GenPWHash
}

func TestCard(t *testing.T) {
	ctrl := gomock.NewController(t)

	cardSet := defaultCardSet()

	addCard := func(num int) {
		for i := 0; i < num; i++ {
			cardSet[string([]byte{0x38, 0x00, 0x00 + byte(i)})] =
				`{"link": "001", "filekey": "1234", "sign": "5678", "ctr": "AAE=", "status": "NORMAL"}`
		}
	}

	defaultBucket := func() *mock.MockIBucket {
		bukMock := mock.NewMockIBucket(ctrl)
		bukMock.EXPECT().ForEach(gomock.Any()).
			DoAndReturn(func(fn func(k, v []byte) error) error {
				for i, v := range cardSet {
					if err := fn([]byte(i), []byte(v)); err != nil {
						return err
					}
				}
				return nil
			}).AnyTimes()
		return bukMock
	}

	defaultDB := func(txMock *mock.MockITx) *mock.MockIDB {
		dbMock := makeDefultDBWithITx(ctrl, txMock)
		dbMock.EXPECT().View(gomock.Any()).
			DoAndReturn(func(fn func(tx card.ITx) error) error {
				return fn(txMock)
			}).AnyTimes()
		return dbMock
	}

	var db *card.CardDatabase

	t.Run("A.CardSearchJSON", func(t *testing.T) {
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// A1. Should find target "01 ef 23 45 67 89 cd"
		if ret := db.CardSearchJSON("01"); len(ret) != 1 &&
			ret[0]["link"] != "002" {
			t.Errorf(`Should find target "01 ef 23 45 67 89 cd"`)
		}

		// A2. Should find mulitple result
		if ret := db.CardSearchJSON("a"); len(ret) != 2 {
			t.Errorf(`Should find mulitple result`)
		}

		// A3. Should return 0 whitout reuslt
		if ret := db.CardSearchJSON("aaaaaa"); len(ret) != 0 {
			t.Errorf(`Should return 0 whitout reuslt`)
		}

		// A4. Should fail when search JSON invalid
		cardSet[string([]byte{0xdf, 0xdc})] = `Not JSON`
		if ret := db.CardSearchJSON("df"); ret != nil {
			t.Errorf(`Should fail when one record failed`)
		}
		delete(cardSet, string([]byte{0xdf, 0xdc}))

		// A5. Should max 50 record
		addCard(55)
		if ret := db.CardSearchJSON("38"); len(ret) > 50 {
			t.Errorf(`Should max 50 record`)
		}

		// A6.1. Should fill the status when not exist
		if ret := db.CardSearchJSON("b4ef"); len(ret) != 1 {
			t.Errorf(`Should get record`)
		} else if m := ret[0]; m["status"] != card.StatusCtrNormal {
			t.Errorf(`Should get back a status`)
		}

		// A6.2. Should fail with invalid ctr
		cardSet[string([]byte{0xb4, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd})] =
			`{"link": "80000000-0000-0000-0000-000000000000", "filekey": "1234", "sign": "5678", "ctr": "///."}`
		if ret := db.CardSearchJSON("b4ef"); ret != nil {
			t.Errorf(`Should fail with invalid ctr`)
		}
		delete(cardSet, string([]byte{0xb4, 0xef, 0x23, 0x45, 0x67, 0x89, 0xcd}))

		// A7. Should fail nil bucket return
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if ret := db.CardSearchJSON("df"); ret != nil {
			t.Errorf(`Should fail nil bucket return`)
		}
	})

	// B. GetCardList
	t.Run("B.GetCardList", func(t *testing.T) {
		bstats := &bbolt.BucketStats{KeyN: 5}
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		bukMock.EXPECT().Stats().Return(*bstats).AnyTimes()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// B1.1. Should Return the full list
		if ret, size := db.GetCardList(0, 20); len(ret) != 5 && size != 5 {
			t.Errorf(`Should Return the full list`)
		}

		// B1.2. Should Return the max page number
		addCard(50)
		bstats.KeyN = 55
		if ret, size := db.GetCardList(0, 20); len(ret) != 20 && size != 55 {
			t.Errorf(`Should Return the max page number`)
		}

		// B2.1. Should Return next page
		if ret, _ := db.GetCardList(1, 20); len(ret) != 20 {
			t.Errorf(`Should Return next page`)
		}

		// B2.1. Should Return last page and remain item
		if ret, _ := db.GetCardList(2, 20); len(ret) != 15 {
			t.Errorf(`Should Return last page and remain item`)
		}

		// B3. Should return nil when unmarshal JSON error happen
		cardSet = defaultCardSet()
		cardSet[string([]byte{0x00, 0x00, 0x00})] = `Not JSON`
		if ret, _ := db.GetCardList(0, 20); ret != nil {
			t.Errorf(`Should return nil when error happen`)
		}
	})

	// Need mock UUID to give fixed id
	uuidMock := mock.NewMockUUID(ctrl)
	tmpluuid := card.DbUUID
	card.DbUUID = uuidMock

	// Dummy dataset
	tmplId := []byte{0x00, 0x12, 0x34, 0x89, 0x9a, 0xbc, 0xde}
	tmplSign := []byte{
		0x00, 0x12, 0x34, 0x89, 0x9a, 0xbc, 0xde, 0x12, 0x34, 0x56, 0x78, 0x90,
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x90,
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
	}
	tmplCtr := make([]byte, 3)
	uuidMockval := uuid.UUID{
		0x00, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
	}

	// For better reading the dummy set, following whill use
	// `strings.Replace()` and refer to `tmplJSON` to make to
	// test set
	const refLink = "https://somelink"
	const refKey = "00123456789abcdefedcba9876543210"
	const refSign = "ABI0iZq83hI0VniQq83vEjRWeJCrze+QASNFZ4mrze8BI0VnASNFZ4mrze8BI0VnASNFZ4mrze8="
	const refCtr = "AAAA"
	const tmplJSON = "{\"ctr\":\"" + refCtr +
		"\",\"filekey\":\"" + refKey +
		"\",\"link\":\"" + refLink +
		"\",\"sign\":\"" + refSign +
		"\",\"status\":\"" + card.StatusCtrNormal + "\"}"

	// C. WriteCardData
	t.Run("C.WriteCardData", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// C1. Should success write card data
		retJson1 := []byte(tmplJSON)
		uuidMock.EXPECT().NewV4().Return(uuidMockval, nil)
		bukMock.EXPECT().Get(gomock.Any()).Return(nil)
		bukMock.EXPECT().Put(tmplId, retJson1).Return(nil)
		if fkey, err := db.WriteCardData(tmplId, tmplCtr, tmplSign, refLink); err != nil || fkey == "" {
			t.Errorf(`Should success write card data`)
		}

		// C2. Should failed when uuid error
		fakeuuidError := errors.New("fake uuid error")
		uuidMock.EXPECT().NewV4().Return(uuid.UUID{}, fakeuuidError)
		bukMock.EXPECT().Get(gomock.Any()).Return(nil)
		if fkey, err := db.WriteCardData(tmplId, tmplCtr, tmplSign, refLink); err != fakeuuidError || fkey != "" {
			t.Errorf(`Should failed when uuid error`)
		}

		// C3. Should update the existed record
		link3 := "https://newlink"
		pwN := "aa123456789abcdefedcba9876543210"
		rawJson3 := strings.Replace(tmplJSON, refKey, pwN, 1)
		retJson3 := strings.Replace(rawJson3, refLink, link3, 1)
		bukMock.EXPECT().Get(gomock.Any()).Return([]byte(rawJson3))
		bukMock.EXPECT().Put(tmplId, []byte(retJson3)).Return(nil)
		if fkey, err := db.WriteCardData(tmplId, tmplCtr, tmplSign, link3); err != nil || fkey != pwN {
			t.Errorf(`Should failed when uuid error`)
		}

		// C4. Should failed recived invalid JSON
		bukMock.EXPECT().Get(gomock.Any()).Return([]byte("Not JSON"))
		if _, err := db.WriteCardData(tmplId, tmplCtr, tmplSign, link3); err != card.ErrCardDBRead {
			t.Errorf(`Should failed recived invalid JSON`)
		}

		// C5.1. Should Check invalid len cardId
		badIdSign := []byte{0x00, 0x22, 0x33, 0x44}
		if _, err := db.WriteCardData(badIdSign, tmplCtr, tmplSign, link3); err != card.ErrCardIdSignWrong {
			t.Errorf(`Should Check invalid len cardId`)
		}

		// C5.2. Should Check invalid len cardSign
		if _, err := db.WriteCardData(tmplId, tmplCtr, badIdSign, link3); err != card.ErrCardIdSignWrong {
			t.Errorf(`Should Check invalid len cardSign`)
		}

		// C6. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if _, err := db.WriteCardData(tmplId, tmplCtr, badIdSign, link3); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})

	card.DbUUID = tmpluuid

	t.Run("D.ReadCardData", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// D1. Should get the dataset
		retJson1 := []byte(tmplJSON)
		bukMock.EXPECT().Get(tmplId).Return(retJson1)
		if cardSign, link, err := db.ReadCardData(tmplId); err != nil ||
			!bytes.Equal(cardSign, tmplSign) || link != refLink {
			t.Errorf(`Should get the dataset`)
		}

		// D2. Should fail cardSign error
		retJson2 := []byte(strings.Replace(
			tmplJSON, refSign, "("+refSign, 1))
		bukMock.EXPECT().Get(tmplId).Return(retJson2)
		if cardSign, link, err := db.ReadCardData(tmplId); err == nil ||
			len(cardSign) != 0 || link != "" {
			t.Errorf(`Should fail cardSign error`)
		}

		// D3. Should fail JSON error
		retJson3 := []byte("{Not JSON}")
		bukMock.EXPECT().Get(tmplId).Return(retJson3)
		if _, _, err := db.ReadCardData(tmplId); err == nil {
			t.Errorf(`Should fail JSON error`)
		}

		// D4. Should fail on invalid cardId
		badIdSign := []byte{0x00, 0x22, 0x33, 0x44}
		if _, _, err := db.ReadCardData(badIdSign); err != card.ErrCardIdSignWrong {
			t.Errorf(`Should fail on invalid cardId`)
		}

		// D6. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if _, _, err := db.ReadCardData(tmplId); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})

	t.Run("E.ReadCardFilekey", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		fkeyRet := []byte{
			0x00, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde,
			0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
		}

		// E1. Should get the fkey and ctr
		retJson1 := []byte(tmplJSON)
		bukMock.EXPECT().Get(tmplId).Return(retJson1)
		if fkey, ctr, err := db.ReadCardFilekey(tmplId); err != nil ||
			!bytes.Equal(fkey, fkeyRet) || !bytes.Equal(ctr, []byte{0x00, 0x00, 0x00}) {
			t.Errorf(`Should get the fkey and ctr`)
		}

		// E2. Should fail key is invalid format
		retJson2 := []byte(strings.Replace(
			tmplJSON, refKey, "zz123456789abcdefedcba9876543210", 1))
		bukMock.EXPECT().Get(tmplId).Return(retJson2)
		if fkey, ctr, err := db.ReadCardFilekey(tmplId); err == nil ||
			len(fkey) != 0 || len(ctr) != 0 {
			t.Errorf(`Should fail key is invalid format`)
		}

		// E3. Should fail JSON error
		retJson3 := []byte("{Not JSON}")
		bukMock.EXPECT().Get(tmplId).Return(retJson3)
		if _, _, err := db.ReadCardFilekey(tmplId); err == nil {
			t.Errorf(`Should fail JSON error`)
		}

		// E4. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if _, _, err := db.ReadCardFilekey(tmplId); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})

	t.Run("F.CheckedCardStatus", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// F1. Should success check the card status
		retJson1 := []byte(strings.Replace(tmplJSON, card.StatusCtrNormal, card.StatusCtrJump, 1))
		putJson1 := []byte(tmplJSON)
		bukMock.EXPECT().Get(tmplId).Return(retJson1)
		bukMock.EXPECT().Put(tmplId, putJson1).Return(nil)
		if err := db.CheckedCardStatus(tmplId); err != nil {
			t.Errorf(`Should success check the card status`)
		}

		// F2. Should fail db return nil
		bukMock.EXPECT().Get(tmplId).Return(nil)
		if err := db.CheckedCardStatus(tmplId); err != card.ErrCardDBNotExists {
			t.Errorf(`Should fail db return nil`)
		}

		// F3. Should fail JSON error
		retJson3 := []byte("{Not JSON}")
		bukMock.EXPECT().Get(tmplId).Return(retJson3)
		if err := db.CheckedCardStatus(tmplId); err != card.ErrCardDBRead {
			t.Errorf(`Should fail JSON error`)
		}

		// F4. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if err := db.CheckedCardStatus(tmplId); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})

	t.Run("G.UpdateCardPW", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// G1. Should success update card password
		pwN := "aa123456789abcdefedcba9876543210"
		retJson1 := []byte(tmplJSON)
		putJson1 := []byte(strings.Replace(tmplJSON, refKey, pwN, 1))
		bukMock.EXPECT().Get(tmplId).Return(retJson1)
		bukMock.EXPECT().Put(tmplId, putJson1).Return(nil)
		if err := db.UpdateCardPW(tmplId, pwN); err != nil {
			t.Errorf(`Should success update card password`)
		}

		// G2. Should fail db return nil
		bukMock.EXPECT().Get(tmplId).Return(nil)
		if err := db.UpdateCardPW(tmplId, pwN); err != card.ErrCardDBNotExists {
			t.Errorf(`Should fail db return nil`)
		}

		// G3. Should fail JSON error
		retJson3 := "{Not JSON}"
		bukMock.EXPECT().Get(tmplId).Return([]byte(retJson3))
		if err := db.UpdateCardPW(tmplId, pwN); err != card.ErrCardDBRead {
			t.Errorf(`Should fail JSON error`)
		}

		// G4.1. Should fail invalid passward format - short
		pwShort := "123456789abcdefedcba9876543210"
		if err := db.UpdateCardPW(tmplId, pwShort); err != card.ErrCardInput {
			t.Errorf(`Should fail invalid passward format - short`)
		}

		// G4.2. Should fail invalid passward format - invalid character
		pwChar := "zz123456789abcdefedcba9876543210"
		if err := db.UpdateCardPW(tmplId, pwChar); err != card.ErrCardInput {
			t.Errorf(`Should fail invalid passward format - invalid character`)
		}

		// G5. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if err := db.UpdateCardPW(tmplId, pwN); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})

	t.Run("H.UpdateCardCTR", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// H1. Should CTR updated
		bCtrN := []byte{0x00, 0x00, 0x03}
		ctrN := "AAAD" // The value of bCtrN encode into base64
		resJson1 := []byte(tmplJSON)
		putJson1 := []byte(strings.Replace(tmplJSON, refCtr, ctrN, 1))
		bukMock.EXPECT().Get(tmplId).Return(resJson1)
		bukMock.EXPECT().Put(tmplId, putJson1).Return(nil)
		if err := db.UpdateCardCTR(tmplId, bCtrN, card.StatusCtrNormal); err != nil {
			t.Errorf(`Should CTR updated`)
		}

		// H2. Should fail db return nil
		bukMock.EXPECT().Get(tmplId).Return(nil)
		if err := db.UpdateCardCTR(tmplId, bCtrN, card.StatusCtrNormal); err != card.ErrCardDBNotExists {
			t.Errorf(`Should fail db return nil`)
		}

		// H3. Should fail JSON error
		retJson3 := "{Not JSON}"
		bukMock.EXPECT().Get(tmplId).Return([]byte(retJson3))
		if err := db.UpdateCardCTR(tmplId, bCtrN, card.StatusCtrNormal); err != card.ErrCardDBRead {
			t.Errorf(`Should fail JSON error`)
		}

		// H4. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if err := db.UpdateCardCTR(tmplId, bCtrN, card.StatusCtrNormal); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})

	t.Run("I.DelUpdateCard", func(t *testing.T) {
		cardSet = defaultCardSet()
		bukMock := defaultBucket()
		txMock := mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(bukMock).AnyTimes()
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))

		// I1. Should removed the card as input
		resJson1 := []byte(tmplJSON)
		bukMock.EXPECT().Get(tmplId).Return(resJson1)
		bukMock.EXPECT().Delete(tmplId).Return(nil)
		if err := db.DelUpdateCard(tmplId, refKey); err != nil {
			t.Errorf(`Should remove the card`)
		}

		// I2. Should failed when password not match
		resJson2 := []byte(tmplJSON)
		bukMock.EXPECT().Get(tmplId).Return(resJson2)
		if err := db.DelUpdateCard(tmplId, "wrongkey"); err != card.ErrCardDBRead {
			t.Errorf(`Should failed when password not match`)
		}

		// I3. Should fail JSON error
		retJson3 := "{Not JSON}"
		bukMock.EXPECT().Get(tmplId).Return([]byte(retJson3))
		if err := db.DelUpdateCard(tmplId, refKey); err != card.ErrCardDBRead {
			t.Errorf(`Should fail JSON error`)
		}

		// I4. Should return err when nil bucket returned
		txMock = mock.NewMockITx(ctrl)
		txMock.EXPECT().Bucket(gomock.Any()).Return(nil)
		db, _ = card.CreateCardDBRaw(mock.NewMockIDB(ctrl), defaultDB(txMock))
		if err := db.DelUpdateCard(tmplId, refKey); err != card.ErrCardDBNotExists {
			t.Errorf(`Should return err when nil bucket returned`)
		}
	})
}

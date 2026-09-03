package server_test

import (
	"errors"
	"log"
	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"
	"marveldigital/tag-reader-server/server"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	uuid "github.com/iris-contrib/go.uuid"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

func makeDefaultDBWithITx(ctrl *gomock.Controller, ntx *mock.MockITx) *mock.MockIDB {
	db := mock.NewMockIDB(ctrl)
	fn := func(fn func(tx card.ITx) error) error {
		return fn(ntx)
	}
	db.EXPECT().Update(gomock.Any()).DoAndReturn(fn).AnyTimes()
	db.EXPECT().View(gomock.Any()).DoAndReturn(fn).AnyTimes()
	db.EXPECT().Batch(gomock.Any()).DoAndReturn(fn).AnyTimes()
	db.EXPECT().Close().AnyTimes()
	return db
}

func makeHttpTest(t *testing.T) (
	exp *httptest.Expect, recDB, cardDB *mock.MockIDB, tx *mock.MockITx, buk *mock.MockIBucket) {
	ctrl := gomock.NewController(t)
	buk = mock.NewMockIBucket(ctrl)
	tx = mock.NewMockITx(ctrl)
	tx.EXPECT().CreateBucketIfNotExists(gomock.Any()).AnyTimes()
	recDB = makeDefaultDBWithITx(ctrl, tx)
	cardDB = makeDefaultDBWithITx(ctrl, tx)
	server.CreateCardDBLocal = func(dbpath string) (d *card.CardDatabase, err error) {
		return card.CreateCardDBRaw(recDB, cardDB)
	}

	app := iris.New()
	// Isolate the on-disk user store so the real ../local/userdb.db is never
	// polluted by login/bootstrap tests (which write to the user store). Views
	// still need the real html/js/css, so symlink them into the temp dir.
	tmp := t.TempDir()
	for _, name := range []string{"html", "js", "css"} {
		if abs, aerr := filepath.Abs(filepath.Join("../local", name)); aerr == nil {
			_ = os.Symlink(abs, filepath.Join(tmp, name))
		}
	}
	server.SetDbPath(tmp)
	server.MakeAdminPage(app)
	server.MakeCardPage(app)
	exp = httptest.New(t, app, httptest.URL("http://test.com"))

	return
}

func TestLoginBasic(t *testing.T) {
	ctrl := gomock.NewController(t)
	server.UpdateLogio(log.Default())

	// DB mock
	e, _, _, mockTx, mockBuk := makeHttpTest(t)

	var (
		orgid   = "sampleuser"
		orgpw   = "samplepw"
		salt    = []byte("samplesalt")
		hashRet = []byte("c848f9e0c52559d77a9408959a6caa48b799b133d23dbd0be6ca2239dc8b0151")
	)

	t.Run("1.1.RunTheMainPage", func(t *testing.T) {
		e.GET("/verify").Expect().Body().Contains("Admin page - Login")
	})

	t.Run("1.2.ShouldFailNotLogin", func(t *testing.T) {
		e.GET("/verify/admin/security").Expect().Body().Contains("Not Found")
	})

	// t.Run("A2.ShouldFailNotLogin", func(t *testing.T) {
	// 	e.GET("/verify/admin/security").WithHeader("X-token", "").
	// 		Expect().Body().Contains("Internal Server Error")
	// })

	t.Run("2.RunTheLogin", func(t *testing.T) {
		mockBuk.EXPECT().Get([]byte("rec")).Return(hashRet)
		mockBuk.EXPECT().Get([]byte("salt")).Return(salt)
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk)

		e.POST("/verify/admin/login").WithForm(iris.Map{"inid": orgid, "inpw": orgpw}).Expect().Body().Contains("NFC tag verify server")
	})

	t.Run("3.AdminShouldLogin", func(t *testing.T) {
		e.GET("/verify").Expect().Body().Contains("NFC tag verify server")
	})

	t.Run("4.AdminLoginFailed", func(t *testing.T) {
		mockBuk.EXPECT().Get([]byte("rec")).Return(hashRet)
		mockBuk.EXPECT().Get([]byte("salt")).Return(salt)
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk)

		e.POST("/verify/admin/login").WithForm(iris.Map{"inid": orgid + "a", "inpw": orgpw}).Expect()
	})

	t.Run("5.AdminSrcurity", func(t *testing.T) {
		e.GET("/verify/admin/security").Expect().Body().Contains("Change password")
	})

	pwform := iris.Map{
		"orgid":     orgid,
		"orgpw":     orgpw,
		"changeid":  "newuserid",
		"changepw":  "newuserpw",
		"changepw2": "newuserpw",
	}

	t.Run("6.1.AdminChangePW", func(t *testing.T) {
		mockuuid := mock.NewMockUUID(ctrl)
		tmpUUID := card.DbUUID
		card.DbUUID = mockuuid
		mockBuk.EXPECT().Get([]byte("rec")).Return(hashRet)
		mockBuk.EXPECT().Get([]byte("salt")).Return(salt)
		mockBuk.EXPECT().Put([]byte("rec"), []byte("366e67cb32f9b563b0b7bb9b496ccc36beedb19abf6f37c43072242f3ded953c")).Return(nil)
		mockBuk.EXPECT().Put([]byte("salt"), []byte("91b1a1b3-269c-48c9-9244-4c6841e3f7c9")).Return(nil)
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk).Times(2)
		mockuuid.EXPECT().NewV4().Return(uuid.FromString("91b1a1b3-269c-48c9-9244-4c6841e3f7c9"))

		e.POST("/verify/admin/changepw").WithForm(pwform).Expect().Body().Contains("OK")
		card.DbUUID = tmpUUID
	})

	t.Run("6.2.AdminChangePW-DBFail", func(t *testing.T) {
		mockTx.EXPECT().Bucket(gomock.Any()).Return(mockBuk).Times(2)
		mockBuk.EXPECT().Get([]byte("rec")).Return(hashRet)
		mockBuk.EXPECT().Get([]byte("salt")).Return([]byte("fakesalt"))

		e.POST("/verify/admin/changepw").WithForm(pwform).Expect().Body().Contains("FAIL")
	})

	t.Run("6.3.AdminChangePW-NotSamePW", func(t *testing.T) {
		changedForm := pwform
		changedForm["changepw2"] = "newuserpw2"
		e.POST("/verify/admin/changepw").WithForm(pwform).Expect().Body().Contains("FAIL")
	})

	t.Run("6.4.AdminChangePW-MissAnyValue", func(t *testing.T) {
		changedForm := pwform
		delete(changedForm, "changepw2")
		e.POST("/verify/admin/changepw").WithForm(pwform).Expect().Body().Contains("FAIL")
	})

	t.Run("7.AdminLogout", func(t *testing.T) {
		e.POST("/verify/admin/logout").Expect().Body().Contains("Admin page - Login")
		e.GET("/verify/admin/security").Expect().Body().Contains("Not Found")
	})

	server.Close()
}

func TestIndexEnter(t *testing.T) {
	server.MakeIndex()
	// e, _, _, mockTx, mockBuk := makeHttpTest(t)
	e, _, _, _, _ := makeHttpTest(t)

	e.GET("/").Expect().Body().Contains("Admin page - Login")
	server.Close()
}

func TestServerUp(t *testing.T) {
	ctrl := gomock.NewController(t)

	tx := mock.NewMockITx(ctrl)
	tx.EXPECT().CreateBucketIfNotExists(gomock.Any()).AnyTimes()
	recDB := mock.NewMockIDB(ctrl)
	cardDB := mock.NewMockIDB(ctrl)
	recDB.EXPECT().Close().AnyTimes()
	cardDB.EXPECT().Close().AnyTimes()

	server.SetDbPath("../local")
	tmpLogFatalf := server.LogFatalf
	defer func() { server.LogFatalf = tmpLogFatalf }()

	tmpCreateCard := server.CreateCardDBLocal
	defer func() { server.CreateCardDBLocal = tmpCreateCard }()

	tmpVerifyCard := server.MakeVerifyCardLocal
	defer func() { server.MakeVerifyCardLocal = tmpVerifyCard }()

	t.Run("1.ImageCacheFailed", func(t *testing.T) {
		app := iris.New()
		server.CreateCardDBLocal = func(dbpath string) (d *card.CardDatabase, err error) {
			return card.CreateCardDBRaw(recDB, cardDB)
		}

		os.RemoveAll("source/")
		os.Mkdir("source/", 0750)
		os.WriteFile(filepath.Join("source", "img"), []byte{}, 0750)
		defer os.RemoveAll("source/")

		server.LogFatalf = func(f string, v ...interface{}) {
			if !strings.Contains(f, "Cannot create image cache folder") {
				t.Fail()
			}
		}

		server.MakeCardPage(app)
		server.Close()
	})

	t.Run("2.CreateCardDBFail", func(t *testing.T) {
		app := iris.New()
		server.CreateCardDBLocal = func(dbpath string) (d *card.CardDatabase, err error) {
			return nil, errors.New("test error")
		}
		server.LogFatalf = func(f string, v ...interface{}) {
			if !strings.Contains(v[0].(string), "test error") {
				t.Fail()
			}
		}

		server.MakeCardPage(app)
	})

	t.Run("3.CreateVerifyCard", func(t *testing.T) {
		app := iris.New()
		server.MakeVerifyCardLocal = func(dbPath string) (card *card.VerifyCard) {
			return nil
		}
		server.LogFatalf = func(f string, v ...interface{}) {
			if !strings.Contains(f, "Error") {
				t.Fail()
			}
		}

		server.MakeCardPage(app)
	})
}

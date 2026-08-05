package card_test

import (
	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

// A makeRoute create the setup
func makeRoute(t *testing.T, cardMock *mock.MockICardDB) *httptest.Expect {
	app := iris.New()
	sess := card.MakeCardAdmin("test.com", "local")

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(sess.AdminIn(ctx, cardMock))
	})

	app.Get("/check", func(ctx iris.Context) {
		ctx.StatusCode(sess.AdminCheck(ctx))
	})

	app.Post("/logout", func(ctx iris.Context) {
		sess.AdminOut(ctx)
	})

	e := httptest.New(t, app, httptest.URL("http://test.com"))
	return e
}

func TestAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	cardMock := mock.NewMockICardDB(ctrl)

	e := makeRoute(t, cardMock)

	// A. AdminIn
	// A1. Should login with cookies returned
	cardMock.EXPECT().CheckAdminLogin("testac", "testpw").Return(true)
	res := e.POST("/login").WithForm(iris.Map{"inid": "testac", "inpw": "testpw"}).Expect()
	res.Status(card.SessionPassed).Cookies().Contains(card.SourceSession)

	// A2. Should login fail when input not match
	cardMock.EXPECT().CheckAdminLogin("testac", "testpw").Return(false)
	e.POST("/login").WithForm(iris.Map{"inid": "testac", "inpw": "testpw"}).Expect().
		Status(card.SessionNoVerify).Cookies().Empty()

	// B. AdminCheck
	// B1. Should checked login
	cookie := res.Cookie(card.SourceSession)
	e.GET("/check").WithCookie(card.SourceSession, cookie.Raw().Value).Expect().Status(card.SessionPassed)

	// B2. Should fail to check invaild session
	e.GET("/check").WithCookie(card.SourceSession, "00000000-0000-0000-0000-000000000000").Expect().Status(card.SessionNoVerify)

	// C. AdminOut
	// C1. Should logout without error
	e.POST("/logout").WithCookie(card.SourceSession, cookie.Raw().Value).Expect().Status(iris.StatusOK)

	// C2. Check the session is not existed
	e.GET("/check").WithCookie(card.SourceSession, cookie.Raw().Value).Expect().Status(card.SessionNoVerify)
}

func TestAdminOther(t *testing.T) {
	tmpPath := MakeTempPath()

	sess := card.MakeCardAdmin("", tmpPath)
	if sess.Hostname != "" {
		t.Errorf(`Should not have Hostname when empty name passed`)
	}

	sess.CloseDB()
	os.RemoveAll(tmpPath)
}

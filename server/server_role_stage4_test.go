package server

import (
	"os"
	"testing"

	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"

	"github.com/golang/mock/gomock"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

// stage4App wires the real multi-user CardAdmin and the role-gated
// checkAdminVerify middleware, exposing a protected GUI route and a protected
// API route for assertions.
func stage4App(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *iris.Application {
	t.Helper()
	app := iris.New()
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(userDB)

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	app.Get("/verify/admin/security", func(ctx iris.Context) {
		ctx.StatusCode(iris.StatusOK)
	}).Use(checkAdminVerify)
	app.Get("/verify/api/cardwrite", func(ctx iris.Context) {
		ctx.StatusCode(iris.StatusOK)
	}).Use(checkAdminVerify)

	return app
}

// login returns a fresh httptest client (its own cookie jar) for the user, so
// the follow-up requests from that client carry the session cookie.
func login(t *testing.T, app *iris.Application, username, pw string) *httptest.Expect {
	t.Helper()
	e := httptest.New(t, app, httptest.URL("http://test.com"))
	e.POST("/login").WithForm(iris.Map{"inid": username, "inpw": pw}).
		Expect().Status(card.SessionPassed)
	return e
}

// loginToken logs in with a throwaway client and returns the minted session
// token (cardAdmin.LastLoginToken after AdminIn).
func loginToken(t *testing.T, app *iris.Application, username, pw string) string {
	t.Helper()
	e := httptest.New(t, app, httptest.URL("http://test.com"))
	e.POST("/login").WithForm(iris.Map{"inid": username, "inpw": pw}).
		Expect().Status(card.SessionPassed)
	return cardAdmin.LastLoginToken()
}

// TestStage4RoleAuthz verifies that admins reach GUI admin pages while operators
// are restricted to the /verify/api/... data endpoints (Stage 4).
func TestStage4RoleAuthz(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	// Store is seeded non-empty, so the legacy db path is never reached.
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "stage4-userdb")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()

	// Seed an admin and an operator.
	for _, u := range []struct {
		user, pw, role string
	}{
		{"admin1", "adminpw", card.RoleAdmin},
		{"op1", "oppass", card.RoleOperator},
	} {
		id, _ := card.UserNewID()
		salt, _ := card.UserNewID()
		rec := card.GenUserRec(u.user, u.pw, []byte(salt))
		if err := udb.UserAdd(id, u.user, rec, salt, u.role); err != nil {
			t.Fatalf("seed %s: %v", u.user, err)
		}
	}

	app := stage4App(t, cardMock, udb)

	// Cookie-based GUI clients (each its own jar).
	adminCli := login(t, app, "admin1", "adminpw")
	opCli := login(t, app, "op1", "oppass")

	// API-token-based clients (X-token header).
	adminTok := loginToken(t, app, "admin1", "adminpw")
	opTok := loginToken(t, app, "op1", "oppass")

	// Admin: GUI page allowed.
	adminCli.GET("/verify/admin/security").Expect().Status(iris.StatusOK)
	// Admin: API endpoint allowed.
	apiAdmin := httptest.New(t, app, httptest.URL("http://test.com"))
	apiAdmin.GET("/verify/api/cardwrite").
		WithHeader("X-token", adminTok).Expect().Status(iris.StatusOK)

	// Operator: GUI page denied (404).
	opCli.GET("/verify/admin/security").Expect().Status(iris.StatusNotFound)
	// Operator: API endpoint allowed.
	apiOp := httptest.New(t, app, httptest.URL("http://test.com"))
	apiOp.GET("/verify/api/cardwrite").
		WithHeader("X-token", opTok).Expect().Status(iris.StatusOK)

	// Unauthenticated / bogus API token is denied.
	apiBogus := httptest.New(t, app, httptest.URL("http://test.com"))
	apiBogus.GET("/verify/api/cardwrite").
		WithHeader("X-token", "bogus").Expect().Status(iris.StatusNotFound)
}

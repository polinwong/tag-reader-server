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

// stage7App wires the Stage 7 endpoints: logout invalidation + login history.
func stage7App(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *iris.Application {
	t.Helper()
	app := iris.New()
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(userDB)

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	// API login returns the minted token (used for the X-token path test).
	app.Post("/verify/api/login", verifyApiLogin)
	// A minimal endpoint behind checkAdminVerify to prove token validity for
	// the API (X-token) path without pulling in cardWrite internals.
	app.Get("/verify/api/cardwrite", func(ctx iris.Context) {
		ctx.WriteString("ok")
	}).Use(checkAdminVerify)
	app.Get("/verify/api/loginrec", getLoginRecord).Use(checkAdminVerify)
	app.Delete("/verify/api/logindel/{id}", removeLoginRecord).Use(checkAdminVerify)
	app.Post("/verify/admin/logout", adminLogout).Use(checkAdminVerify)
	return app
}

// TestStage7LogoutAndHistory verifies:
//   - a session token is rejected by the API path after logout (server-side
//     invalidation of sessions[token]), and the GUI cookie is also cleared;
//   - the login-history view lists every login with username + time.
func TestStage7LogoutAndHistory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "stage7-userdb")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()

	seedUser(t, udb, "admin1", "adminpw", card.RoleAdmin)
	seedUser(t, udb, "op1", "oppass", card.RoleOperator)

	app := stage7App(t, cardMock, udb)

	// --- GUI login mints token T (carried in the admin cookie) ---
	admin := httptest.New(t, app, httptest.URL("http://test.com"))
	admin.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "adminpw"}).
		Expect().Status(card.SessionPassed)
	token := cardAdmin.LastLoginToken()
	if token == "" {
		t.Fatalf("expected a session token after login")
	}

	// --- Token is valid for the API (X-token) path before logout ---
	api := httptest.New(t, app, httptest.URL("http://test.com"))
	api.GET("/verify/api/cardwrite").WithHeader("X-token", token).
		Expect().Status(iris.StatusOK)

	// --- Login history lists the login with username + time ---
	rec := admin.GET("/verify/api/loginrec").Expect().JSON().Object()
	rec.Value("msg").Equal("OK")
	data := rec.Value("data").Array()
	data.Length().Gt(0)
	for _, v := range data.Raw() {
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("history entry is not an object: %v", v)
		}
		if m["username"] == nil || m["time"] == nil {
			t.Fatalf("history entry missing username/time: %v", m)
		}
		if _, isNum := m["time"].(float64); !isNum {
			t.Fatalf("history time should be a unix number: %v", m["time"])
		}
	}

	// --- Logout invalidates the token server-side ---
	admin.POST("/verify/admin/logout").Expect()

	// Token is now rejected for the API path (sessions[token] deleted).
	api.GET("/verify/api/cardwrite").WithHeader("X-token", token).
		Expect().Status(iris.StatusNotFound)

	// The old GUI cookie no longer authenticates either.
	admin.GET("/verify/api/loginrec").Expect().Status(iris.StatusNotFound)

	// --- A fresh login after logout re-issues a working token ---
	admin.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "adminpw"}).
		Expect().Status(card.SessionPassed)
	token2 := cardAdmin.LastLoginToken()
	if token2 == "" || token2 == token {
		t.Fatalf("expected a fresh token after re-login (got %q)", token2)
	}
	api2 := httptest.New(t, app, httptest.URL("http://test.com"))
	api2.GET("/verify/api/cardwrite").WithHeader("X-token", token2).
		Expect().Status(iris.StatusOK)

	// History now contains both logins.
	rec2 := admin2History(t, app)
	if float64(rec2) <= data.Length().Raw() {
		t.Fatalf("expected history to grow after re-login (before=%v, after=%d)",
			data.Length().Raw(), rec2)
	}
}

// admin2History reads the login history length for an authenticated admin. It
// logs in a fresh admin client (the previous cookie was destroyed by logout).
func admin2History(t *testing.T, app *iris.Application) int {
	t.Helper()
	admin := httptest.New(t, app, httptest.URL("http://test.com"))
	admin.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "adminpw"}).
		Expect().Status(card.SessionPassed)
	obj := admin.GET("/verify/api/loginrec").Expect().JSON().Object()
	return int(obj.Value("data").Array().Length().Raw())
}

// TestOperatorLoadsAdminJSAndLogout regresses the bug where the js/admin dir
// was guarded by checkAdminVerify, so operators 404'd on admin-common.js and
// logout()/change-pw were undefined on their security page (Sign Out did
// nothing). It also verifies the unified /verify/logout works for operators.
func TestOperatorLoadsAdminJSAndLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "op-js")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()
	seedUser(t, udb, "op1", "oppass", card.RoleOperator)

	app := iris.New()
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(udb)
	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	// Mirror server_admin.go: js/admin is guarded by checkOperatorVerify and the
	// unified logout by the same.
	app.Get("/js/admin/admin-common.js", func(ctx iris.Context) {
		ctx.WriteString("// js")
	}).Use(checkOperatorVerify)
	app.Post("/verify/logout", adminLogout).Use(checkOperatorVerify)

	op := httptest.New(t, app, httptest.URL("http://test.com"))
	op.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).
		Expect().Status(card.SessionPassed)

	// The operator must be able to load the admin JS (was 404 before the fix).
	op.GET("/js/admin/admin-common.js").Expect().Status(iris.StatusOK)

	// The unified logout works for operators and clears the session.
	op.POST("/verify/logout").Expect()
	op.GET("/js/admin/admin-common.js").Expect().Status(iris.StatusNotFound)
}

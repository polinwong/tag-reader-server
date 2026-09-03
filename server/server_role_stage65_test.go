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

// stage65App wires the Stage 6.5 admin user-management endpoints.
func stage65App(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *iris.Application {
	t.Helper()
	app := iris.New()
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(userDB)

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	app.Post("/verify/admin/createuser", adminCreateUser).Use(checkAdminVerify)
	app.Post("/verify/admin/resetpw", adminResetPW).Use(checkAdminVerify)
	app.Get("/verify/admin/userlist", adminUserList).Use(checkAdminVerify)

	return app
}

// TestStage65UserManagement verifies admin can create users and reset passwords
// (rotation + forced change), and that operators are blocked from both.
func TestStage65UserManagement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "stage65-userdb")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()

	seedUser(t, udb, "admin1", "adminpw", card.RoleAdmin)
	seedUser(t, udb, "op1", "oppass", card.RoleOperator)

	app := stage65App(t, cardMock, udb)

	admin := httptest.New(t, app, httptest.URL("http://test.com"))
	admin.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "adminpw"}).
		Expect().Status(card.SessionPassed)

	// --- Create a new operator ---
	admin.POST("/verify/admin/createuser").WithForm(iris.Map{
		"username": "newop", "newpw": "newpw1", "role": card.RoleOperator,
	}).Expect().JSON().Object().Value("msg").Equal("OK")

	// New user can log in with the password and is flagged MustChange.
	newCli := httptest.New(t, app, httptest.URL("http://test.com"))
	newCli.POST("/login").WithForm(iris.Map{"inid": "newop", "inpw": "newpw1"}).
		Expect().Status(card.SessionPassed)
	if info, e := udb.UserGetByUsername("newop"); e != nil || !info.MustChange {
		t.Fatalf("new user should have MustChange=true (got %v, err %v)", info.MustChange, e)
	}

	// Duplicate username rejected.
	admin.POST("/verify/admin/createuser").WithForm(iris.Map{
		"username": "newop", "newpw": "x", "role": card.RoleOperator,
	}).Expect().JSON().Object().Value("msg").Equal("FAIL")

	// --- Admin resets op1's password ---
	op1, _ := udb.UserGetByUsername("op1")
	admin.POST("/verify/admin/resetpw").WithForm(iris.Map{
		"userid": op1.ID, "newpw": "resetpw1",
	}).Expect().JSON().Object().Value("msg").Equal("OK")
	if !udb.UserVerify("op1", "resetpw1") {
		t.Fatalf("op1 should verify with reset password")
	}
	if udb.UserVerify("op1", "oppass") {
		t.Fatalf("op1 old password should NOT verify after reset")
	}
	if info, _ := udb.UserGetByID(op1.ID); !info.MustChange {
		t.Fatalf("op1 should be flagged MustChange after reset")
	}

	// --- Operators are blocked from these admin-only endpoints (404) ---
	// Use op1's NEW password (it was reset above); the old password no longer
	// verifies, which is exactly what we asserted earlier.
	op := httptest.New(t, app, httptest.URL("http://test.com"))
	op.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "resetpw1"}).
		Expect().Status(card.SessionPassed)
	op.POST("/verify/admin/createuser").WithForm(iris.Map{
		"username": "x", "newpw": "y", "role": card.RoleOperator,
	}).Expect().Status(iris.StatusNotFound)
	op.POST("/verify/admin/resetpw").WithForm(iris.Map{
		"userid": op1.ID, "newpw": "z",
	}).Expect().Status(iris.StatusNotFound)
}

// TestOperatorChangeOwnPassword regresses the bug where the security page
// POSTed the change-password form to /verify/admin/changepw (admin-only), so
// operators always failed with "Change fail" even when every field was filled.
// Operators must be able to change their own password via the unified
// /verify/changepw endpoint.
func TestOperatorChangeOwnPassword(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "op-changepw")
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
	app.Post("/verify/changepw", adminChangePW).Use(checkOperatorVerify)

	op := httptest.New(t, app, httptest.URL("http://test.com"))
	op.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).
		Expect().Status(card.SessionPassed)

	// Operator changes own password via the unified endpoint.
	op.POST("/verify/changepw").WithForm(iris.Map{
		"orgpw": "oppass", "changepw": "newpass1", "changepw2": "newpass1",
	}).Expect().JSON().Object().Value("msg").Equal("OK")

	// New password verifies; the old one does not.
	if !udb.UserVerify("op1", "newpass1") {
		t.Fatalf("op1 should verify with the new password")
	}
	if udb.UserVerify("op1", "oppass") {
		t.Fatalf("op1 old password should NOT verify after change")
	}
}

// 3D Artefact Exhibition backend
// File: server_card_verify.go, NFC tag verify for NTAG 424 DNA, webn
// Creater: Kevin Mak, Sep 2021
// (c)Marvel Digital Ltd. 2021

package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	// "marveldigital/sourceserver/card"
	// "marveldigital/sourceserver/dbwork"
	"marveldigital/tag-reader-server/card"
	"strings"

	"github.com/kataras/iris/v12"
)

var (
	cardBase *card.VerifyCard
	cardDB   *card.CardDatabase

	MakeVerifyCardLocal = card.MakeVerifyCard
	CreateCardDBLocal   = card.CreateCardDB

	LogFatalf = log.Fatalf

	// basicImagePath is the directory where model images are stored/served.
	// It is resolved at init: an explicit IMG_DIR env var wins; otherwise it
	// is anchored to the executable's directory so the server is independent
	// of the current working directory (safe for production/systemd/Docker).
	basicImagePath string
)

const (
	appsName = "com.mdl.arttagscanner"
	ListMax  = 50
)

func init() {
	if dir := os.Getenv("IMG_DIR"); dir != "" {
		basicImagePath = dir
	} else if ex, err := os.Executable(); err == nil {
		basicImagePath = filepath.Join(filepath.Dir(ex), "source", "img")
	} else {
		// Fallback: CWD-relative (dev, e.g. `go run`).
		basicImagePath = filepath.Join("source", "img")
	}
}

func MakeCardPage(app *iris.Application) {
	// User view
	app.Get("/verify/sun", verifySUN)
	app.Post("/verify/sun", verifySUN)
	// app.Get("/verify/api", verifyUID) //debug
	app.Post("/verify/api", verifyUID)
	app.Post("/verify/api/login", verifyApiLogin)
	app.Get("/verify/api/linkmodel/{id}", getLinkModelDetail)

	// Admin view
	app.Get("/verify/admin", verifyAdmin).Use(checkAdminVerify)
	app.Get("/verify/api/loginrec", getLoginRecord).Use(checkAdminVerify)
	app.Delete("/verify/api/logindel/{id}", removeLoginRecord).Use(checkAdminVerify)

	app.Options("/verify/sun", corsPreflight)
	app.Options("/verify/api/cardwrite", corsPreflight)
	app.Options("/verify/api/cardsearch", corsPreflight)
	app.Options("/verify/api/modellist", corsPreflight)

	app.Get("/verify/api/carddata", cardData).Use(checkAdminVerify)
	app.Post("/verify/api/cardwrite", cardWrite).Use(checkAdminVerify)
	// app.Get("/verify/api/cardwrite", writeTag).Use(checkAdminVerify) //debug
	app.Post("/verify/api/cardchecked", cardEdit).Use(checkAdminVerify)
	app.Post("/verify/api/cardpwupdate", cardEdit).Use(checkAdminVerify)
	app.Post("/verify/api/carddel", cardEdit).Use(checkAdminVerify)
	app.Post("/verify/api/cardlinkset", cardEdit).Use(checkAdminVerify)
	app.Get("/verify/api/cardsearch", cardSearch).Use(checkAdminVerify)

	app.Get("/verify/api/modellist", modelList).Use(checkAdminVerify)
	app.Post("/verify/api/modelimgclean", modelImgClean).Use(checkAdminVerify)
	app.Post("/verify/api/modelwrite", modelWrite).Use(checkAdminVerify)
	app.Get("/verify/api/modelsearch", modelSearch).Use(checkAdminVerify)

	app.HandleDir("/verify/js", dbPath+"/js/verify")
	app.HandleDir("/verify/source/img", basicImagePath)
	app.HandleDir("/verify/css", dbPath+"/css")

	if cardBase = MakeVerifyCardLocal(dbPath); cardBase == nil {
		LogFatalf("Error when create card verify library\n")
	}

	if d, err := CreateCardDBLocal(dbPath); err == nil {
		cardDB = d
	} else {
		LogFatalf("Error create card database, server closed in reason \"%s\"\n", err.Error())
	}

	if f, err := os.Stat(basicImagePath); err == nil {
		if !f.IsDir() {
			LogFatalf("Cannot create image cache folder, please remove path \"%s\" and restart\n", basicImagePath)
		}
	} else {
		if err := os.MkdirAll(basicImagePath, 0750); err != nil {
			LogFatalf("Create folder error, %s\n", err)
		}
	}
}

func checkHeader(ctx iris.Context) bool {
	return ctx.GetHeader("X-Requested-With") == appsName
}

func praseFunc(ctx iris.Context, value string) string {
	if ctx.Method() == iris.MethodPost {
		return ctx.FormValue(value)
	} else {
		return ctx.URLParamEscape(value)
	}
}

// API login for app
func verifyApiLogin(ctx iris.Context) {
	defer func() {
		if perr := recover(); perr != nil {
			log.Printf("login panic, %s", perr)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.WriteString("500 Internal Server Error")
		}
	}()
	if !checkHeader(ctx) {
		ctx.NotFound()
		return
	}

	if cardAdmin.AdminIn(ctx, cardDB) == card.SessionPassed {
		if id, err := cardDB.OnLoginUser(); err != nil {
			ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error()})
		} else {
			appCORSHandle(ctx)
			ctx.JSON(iris.Map{"msg": "OK", "token": id})
		}
	} else {
		loginFailLog(ctx.Request().RemoteAddr)
		ctx.JSON(iris.Map{"msg": "FAIL"})
	}
}

func getLoginRecord(ctx iris.Context) {
	ctx.JSON(iris.Map{"msg": "OK", "data": cardDB.GetLoginRecord()})
}

func removeLoginRecord(ctx iris.Context) {
	id := ctx.Params().Get("id")
	if err := cardDB.RemoveLoginRecord(id); err != nil {
		ctx.NotFound()
		ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error()})
	}
	ctx.JSON(iris.Map{"msg": "OK"})
}

func verifyAdmin(ctx iris.Context) {
	ctx.ViewLayout("page-layout.html")
	ctx.ViewData("title", "Card verify")
	ctx.ViewData("addon", `<script src="https://cdn.jsdelivr.net/npm/bs-custom-file-input/dist/bs-custom-file-input.min.js"></script>
<script src="/js/admin/card-verify.js"></script>`)
	ctx.ViewData("message", "")
	ctx.ViewData("navActiveL1", " active")
	ctx.View("card-verify.html")
}

func corsPreflight(ctx iris.Context) {
	if ctx.Method() == "OPTIONS" {
		if !checkHeader(ctx) {
			ctx.NotFound()
			return
		}
		origin := ctx.Request().Header.Get("origin")
		ctx.Header("Access-Control-Allow-Origin", origin)
		ctx.Header("Access-Control-Allow-Credentials", "true")
		ctx.Header("Access-Control-Allow-Headers", "X-token, X-Requested-With")
		ctx.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
}

func modelList(ctx iris.Context) {
	starttime := time.Now()
	var startPage int
	if p, err := ctx.URLParamInt("page"); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.WriteString("Bad request")
		return
	} else {
		startPage = p
	}

	dataset, maxitem := cardDB.ModelLinkListJSON(startPage, ListMax)
	if dataset != nil {
		for i := range dataset {
			if dataset[i]["image"] != nil {
				img := dataset[i]["image"].(string)
				dataset[i]["img"] = "/verify/source/img/" + img
			}
		}
		appCORSHandle(ctx)
		numPage := math.Ceil(float64(maxitem) / float64(ListMax))
		ctx.JSON(iris.Map{"msg": "OK", "items": dataset, "maxpage": numPage, "maxitems": ListMax, "usedtime": time.Since(starttime).String()})
	} else {
		ctx.JSON(iris.Map{"msg": "FAIL"})
	}
}

func modelWrite(ctx iris.Context) {
	retFail := func(err error) {
		msg := fmt.Sprintf("Error: %s", err)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": msg})
		ctx.StatusCode(iris.StatusBadRequest)
	}

	defer func() {
		if err := recover(); err != nil {
			retFail(errors.New(fmt.Sprint(err)))
		}
	}()

	if ctx.FormValue("modelDel") == "1" {
		id := ctx.FormValue("modelId")
		if len(id) == 0 {
			panic(errors.New("missed model id"))
		}
		info := card.ModelInfo{}
		info.SetID(id)
		if err := cardDB.ModelDelLink(info); err != nil {
			if err != card.ErrCardModelDelLinking {
				panic(err)
			} else {
				ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error() + " (" + id + ")"})
				return
			}
		}
		ctx.JSON(iris.Map{"msg": "OK", "info": id + " removed"})
		return
	}

	name := ctx.FormValue("modelIdName")
	desc := ctx.FormValue("modelDesc")
	id := ctx.FormValue("modelId")

	var imageID string
	if file, h, err := ctx.FormFile("modelImg"); err == nil {
		if h.Size > 0 {
			buf := make([]byte, h.Size)
			if n, err := file.Read(buf); err != nil && n < 0 {
				panic(errors.New("error read file, " + err.Error()))
			}
			hashRet := sha256.Sum256(buf)
			if len(hashRet) == 32 {
				imageID = hex.EncodeToString(hashRet[:])
			}

			if err := os.WriteFile(filepath.Join(basicImagePath, imageID), buf, 0640); err != nil {
				panic(errors.New("error write image, " + err.Error()))
			}
		}
	}

	if id == "" && len(name) == 0 {
		panic(errors.New("name is empty"))
	}

	var err error
	var model card.ModelInfo
	if model, err = card.NewModelInfo(name, desc, imageID, id); err == nil {
		if id != "" {
			if rec, err := cardDB.ModelGetLink(id); err == nil {
				if name == "" {
					model.Name = rec.Name
				}
				if desc == "" {
					model.Desc = rec.Desc
				}
				if imageID == "" {
					model.Image = rec.Image
				}
			}
		} else if _, err = cardDB.ModelGetLink(model.GetID()); err == nil {
			panic(errors.New("database error, please resubmit"))
		}
		if err = cardDB.ModelAddLink(model); err == nil {
			ctx.JSON(iris.Map{"msg": "OK", "retid": model.GetID()})
			return
		}
	}
	retFail(err)
}

func modelImgClean(ctx iris.Context) {
	filesList, err := os.ReadDir(basicImagePath)
	if err != nil {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Image path reading error"})
		return
	}

	m := map[string]bool{}
	for _, v := range filesList {
		m[v.Name()] = true
	}

	if err := cardDB.ModelGetRemainImages(m); err != nil {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Read record error"})
		return
	}

	for k := range m {
		os.Remove(basicImagePath + "/" + k)
	}

	ctx.JSON(iris.Map{"msg": "OK"})
}

func cardWrite(ctx iris.Context) {
	id := praseFunc(ctx, "id")
	sign := praseFunc(ctx, "sign")
	link := praseFunc(ctx, "link")

	bId, err := base64.URLEncoding.DecodeString(id)
	if err != nil {
		ctx.JSON(iris.Map{"msg": "FAIL"})
		return
	}
	bSign, err := base64.URLEncoding.DecodeString(sign)
	if err != nil {
		ctx.JSON(iris.Map{"msg": "FAIL"})
		return
	}
	if !cardBase.VerifyUIDBase64(sign, id) {
		ctx.JSON(iris.Map{"msg": "FAIL"})
		return
	}

	var fkey string
	if fkey, err = cardDB.WriteCardData(bId, make([]byte, 3), bSign, link); err != nil {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error()})
		return
	}
	appCORSHandle(ctx)
	ctx.JSON(iris.Map{
		"msg":          "OK",
		"id":           id,
		"fkey":         fkey,
		"metakey":      hex.EncodeToString(card.SdmMetaKey),
		"appmasterkey": hex.EncodeToString(card.AppMasterKey),
	})
}

func cardData(ctx iris.Context) {
	var startPage int
	if p, err := ctx.URLParamInt("page"); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.WriteString("Bad request")
		return
	} else {
		startPage = p
	}

	l, maxitem := cardDB.GetCardList(startPage, ListMax)
	if len(l) == 0 {
		ctx.JSON(iris.Map{"msg": "OK", "data": "empty"})
		return
	}
	for i := range l {
		// When record is invalid, mark it no record
		if data, err := cardDB.ModelGetLink(l[i]["link"].(string)); err == nil {
			l[i]["name"] = data.Name
		} else {
			l[i]["name"] = "No record"
		}
	}
	numPage := math.Ceil(float64(maxitem) / float64(ListMax))
	ctx.JSON(iris.Map{"msg": "OK", "maxpage": numPage, "maxitems": ListMax, "data": l})
}

func cardEdit(ctx iris.Context) {
	var err error
	defer func() {
		if err != nil {
			logio.Printf("Access error, %s", err)
			ctx.JSON(iris.Map{"msg": "FAIL", "info": "error happen"})
			ctx.StatusCode(iris.StatusBadRequest)
		}
	}()

	const (
		workChecked  = "CHECKED"
		workPWUpdate = "PWUPDATE"
		workDel      = "DEL"
		workLinkSet  = "LINKSET"
	)
	var work string
	path := ctx.RequestPath(true)
	if strings.Contains(path, "cardchecked") {
		work = workChecked
	} else if strings.Contains(path, "cardpwupdate") {
		work = workPWUpdate
	} else if strings.Contains(path, "carddel") {
		work = workDel
	} else if strings.Contains(path, "cardlinkset") {
		work = workLinkSet
	}

	var bId []byte
	if id := ctx.PostValue("id"); len(id) != 14 {
		err = errors.New("id is not valid")
		return
	} else if bId, err = hex.DecodeString(id); err != nil {
		return
	} else {
		if work == workChecked {
			if err = cardDB.CheckedCardStatus(bId); err == nil {
				ctx.JSON(iris.Map{"msg": "OK", "id": id})
				return
			}
		}
		if work == workPWUpdate {
			if err = cardDB.UpdateCardPW(bId, ctx.PostValue("pw")); err == nil {
				ctx.JSON(iris.Map{"msg": "OK", "id": id})
				return
			}
		}
		if work == workDel {
			if err = cardDB.DelUpdateCard(bId, ctx.PostValue("pw")); err == nil {
				ctx.JSON(iris.Map{"msg": "OK", "id": id})
				return
			}
		}
		if work == workLinkSet {
			if err = cardDB.UpdateCardLink(bId, ctx.PostValue("link")); err == nil {
				ctx.JSON(iris.Map{"msg": "OK", "id": id})
				return
			}
		}
	}
}

func appCORSHandle(ctx iris.Context) {
	if checkHeader(ctx) {
		origin := ctx.Request().Header.Get("origin")
		ctx.Header("Access-Control-Allow-Origin", origin)
		ctx.Header("Access-Control-Allow-Credentials", "true")
	}
}

func verifySUN(ctx iris.Context) {
	var uid, ctr, cmac, picc, bin string

	method := ctx.Method()

	// uid = praseFunc("uid")
	// ctr = praseFunc("ctr")
	cmac = praseFunc(ctx, "cmac")
	picc = praseFunc(ctx, "picc_data")
	bin = praseFunc(ctx, "d")

	doMethodAction := func() {
		if method == iris.MethodGet {
			ctx.View("verifysrc/verify-page.html")
		} else {
			ctx.NotFound()
		}
	}

	if len(bin) == 0 {
		// Raw data input for testing
		if len(picc) < 32 && len(uid) < 14 && len(ctr) < 6 && len(cmac) < 16 {
			doMethodAction()
			return
		}
	} else {
		// Packed and encrypted data
		if len(bin) == 64 { // LRP encryption
			picc = bin[:48]
			cmac = bin[48:]
		} else if len(bin) == 48 { // AES encryption
			picc = bin[:32]
			cmac = bin[32:]
		} else {
			ctx.ViewData("dataresult", iris.Map{"msg": "FAIL", "info": "data invalid"})
			doMethodAction()
			return
		}
	}

	bCmac, err3 := hex.DecodeString(cmac)
	bPicc, err4 := hex.DecodeString(picc)

	if err3 != nil || err4 != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"msg": "read data empty error"})
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Verify panic, %s", err)
			ctx.JSON(iris.Map{"msg": "FAIL", "info": "data invalid"})
		}
	}()
	var finJson iris.Map
	if cid, err := cardBase.VerifyPICCData(bPicc, bCmac, cardDB, appdebug); err == nil {
		finJson = iris.Map{"msg": "OK", "info": "data valid"}
		if _, link, cderr := cardDB.ReadCardData(cid); cderr == nil {
			finJson["link"] = link
		}
	} else {
		finJson = iris.Map{"msg": "FAIL", "info": "data invalid"}
		if err == card.ErrVerifySunCtrRepeated {
			logio.Printf("Card id: %s, %s", card.LastErrorID, err)
		}

		// debug only
		// if err == card.ErrVerifySunCtrRepeated {
		// 	finJson = iris.Map{"msg": "OK", "info": "data valid"}
		// 	if _, link, cderr := cardDB.ReadCardData(cid); cderr == nil {
		// 		finJson["link"] = link
		// 	}
		// }
	}
	if method == iris.MethodGet {
		ctx.ViewData("dataresult", finJson["msg"])
		if finJson["link"] != nil {
			ctx.ViewData("dataid", finJson["link"].(string))
		}
		ctx.View("verifysrc/verify-page.html")
	} else {
		appCORSHandle(ctx)
		ctx.JSON(finJson)
	}

	// plain text verify is debug only
	// bUid, err1 := hex.DecodeString(uid)
	// bCtr, err2 := hex.DecodeString(ctr)
	// if err1 != nil || err2 != nil || err3 != nil {
	// 	ctx.StatusCode(iris.StatusBadRequest)
	// 	ctx.JSON(iris.Map{"msg": "read data empty error"})
	// 	return
	// }

	// if err := cardBase.VerifyPlainSUN(bUid, bCtr, bCmac); err == nil {
	// 	ctx.JSON(iris.Map{"msg": "verify OK"})
	// } else {
	// 	ctx.JSON(iris.Map{"msg": "verify FAIL"})
	// }
}

func getLinkModelDetail(ctx iris.Context) {
	if p, err := cardDB.ModelGetLink(ctx.Params().Get("id")); err == nil {
		appCORSHandle(ctx)
		if len(p.Image) > 0 {
			ctx.JSON(iris.Map{"msg": "OK", "name": p.Name, "img": "/verify/source/img/" + p.Image, "desc": p.Desc})
			return
		} else {
			ctx.JSON(iris.Map{"msg": "OK", "name": p.Name, "img": "", "desc": p.Desc})
			return
		}
	}
	ctx.NotFound()
}

func verifyUID(ctx iris.Context) {
	sign := praseFunc(ctx, "s")
	uid := praseFunc(ctx, "u")
	if len(sign) == 0 && len(uid) == 0 {
		ctx.NotFound()
		return
	}

	// ret := cardBase.VerifyUIDBase64("0ZQNF8_tpL_4A1mrl1-fZRQxPo-QwdPKr1lBrXRKHN-ag_iDyv4P6V0ZObG35HETmTMkRzt4XSE=", "BFGN-qlhgA==") // test data
	ret := cardBase.VerifyUIDBase64(sign, uid)
	if bUid, err := base64.URLEncoding.DecodeString(uid); err == nil {
		// Debug for capture signatrue
		// bsign, err2 := base64.URLEncoding.DecodeString(sign)
		// if err2 != nil {
		// 	ctx.JSON(iris.Map{"msg": "data read error"})
		// 	return
		// }
		// log.Printf("verfiy id: %s, sign: %s, result: %t\n", hex.EncodeToString(bUid), hex.EncodeToString(bsign), ret)

		log.Printf("verfiy id: %s, result: %t\n", hex.EncodeToString(bUid), ret)

		appCORSHandle(ctx)
		if d, l, err := cardDB.ReadCardData(bUid); err == nil && d != nil {
			ctx.JSON(iris.Map{"msg": "key verify", "result": ret, "link": l})
			return
		} else {
			// ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"msg": "access error"})
			return
		}
	} else {
		log.Printf("verify failed")
		ctx.JSON(iris.Map{"msg": "key verify", "result": ret})
		return
	}
}

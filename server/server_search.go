// 3D Artefact Exhibition backend
// File: server_search.go, NFC tag verify for NTAG 424 DNA, model and card search
// Creater: Kevin Mak, Dec 2021
// (c)Marvel Digital Ltd. 2021

package server

import (
	"encoding/base64"
	"time"

	"github.com/kataras/iris/v12"
)

func modelSearch(ctx iris.Context) {
	starttime := time.Now()
	if k := ctx.URLParamEscape("key"); len(k) == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.WriteString("Bad request")
	} else {
		dataset := cardDB.ModelSearchJSON(k)
		ctx.JSON(iris.Map{"msg": "OK", "items": dataset, "usedtime": time.Since(starttime).String()})
	}
}

func cardSearch(ctx iris.Context) {
	badReq := func() {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.WriteString("Bad request")
	}

	starttime := time.Now()
	k := ctx.URLParamEscape("key")
	id := ctx.URLParamEscape("id")
	if len(k) == 0 && len(id) == 0 {
		badReq()
		return
	}

	if len(k) > 0 {
		dataset := cardDB.CardSearchJSON(k)
		for i := range dataset {
			data, err := cardDB.ModelGetLink(dataset[i]["link"].(string))
			if err != nil {
				ctx.JSON(iris.Map{"msg": "FAIL", "info": "access error, " + err.Error()})
				return
			}
			dataset[i]["name"] = data.Name
		}
		ctx.JSON(iris.Map{"msg": "OK", "items": dataset, "usedtime": time.Since(starttime).String()})
	}

	if len(id) > 0 {
		buf, err := base64.URLEncoding.DecodeString(id)
		if err != nil {
			badReq()
			return
		}
		appCORSHandle(ctx)
		if _, _, err := cardDB.ReadCardData(buf); err == nil {
			ctx.JSON(iris.Map{"msg": "EXISTED", "usedtime": time.Since(starttime).String()})
		} else {
			ctx.JSON(iris.Map{"msg": "NOTEXIST", "usedtime": time.Since(starttime).String()})
		}
	}
}

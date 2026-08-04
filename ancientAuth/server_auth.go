// 3D Artefact Exhibition backend
// File: server_auth.go, Artefacts Authentication main program
// Creater: Kevin Mak, March 2021
// (c)Marvel Digital Ltd. 2021

// Package package ancientauth for ancient art authentication
package ancientauth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	// "marveldigital/climgt" // [Commented out] climgt not available for local build
	"os"
	"path/filepath"
	"strconv"

	uuid "github.com/iris-contrib/go.uuid"
	"github.com/kataras/iris/v12"
)

const authProgramName = "Ancient Artefacts Authentication Platform"

var (
	authdb *authDB
	logio  *log.Logger
)

type histogramData [16][4]float64

func MakeAncientAuthServer(app *iris.Application) {
	// [Commented out] climgt not available for local build
	// logio = climgt.Logio
	// [Replacement] Use local logger writing to stdout instead of climgt.Logio
	logio = log.New(io.MultiWriter(os.Stdout), "", log.LstdFlags)
	if ent, err := os.ReadDir("./uploadtmp"); err == nil {
		for _, v := range ent {
			os.RemoveAll("./uploadtmp/" + v.Name())
		}
	}

	authdb = MakeAuthDB()

	app.Get("/auth", authBasic)
	app.Post("/auth/scan", authScan)
	app.Post("/auth/api/chk", authCheck)
	app.Post("/auth/api/ref", authRef)
	app.Post("/auth/api/submit", authSubmit)

	app.HandleDir("/js/auth", "./local/js/auth")
}

func authBasic(ctx iris.Context) {
	ctx.ViewLayout("page-layout.html")
	ctx.ViewData("title", authProgramName)
	ctx.ViewData("addon", `<script src="/js/auth/auth.js"></script>
<script src="/js/auth/html5-qrcode.min.js"></script>`)
	ctx.ViewData("message", "")

	// The simple qrcode reader only for debug
	debugData := ""
	if ctx.FormValue("debug") == "1" {
		debugData = `<button class="btn btn-primary" id="qrSimpleBtn">Simple QR reader</button>`
	}
	ctx.ViewData("debug", debugData)

	ctx.View("auth/auth-index.html")
}

func authScan(ctx iris.Context) {
	if curid := ctx.PostValue("id"); curid != "" {
		ctx.ViewLayout("page-layout.html")
		ctx.ViewData("title", authProgramName)
		ctx.ViewData("addon", `<script src="https://cdn.jsdelivr.net/npm/bs-custom-file-input/dist/bs-custom-file-input.min.js"></script>
<script src="/js/auth/auth-scan.js"></script>`)
		ctx.ViewData("message", `<script>var objId = "`+curid+`";</script>`)
		ctx.View("auth/auth-scan.html")
		return
	}
	ctx.NotFound()
}

func authCheck(ctx iris.Context) {
	if curid := authdb.CheckID(ctx.PostValue("id")); curid != "" {
		// TODO: This path only for testing
		if f, err := os.Open("./source/authdata/overview/" + curid); err == nil {
			st, _ := f.Stat()
			b := make([]byte, st.Size())

			if _, err := f.Read(b); err != nil {
				ctx.StatusCode(iris.StatusInternalServerError)
				ctx.JSON(iris.Map{"msg": "FAIL", "error": "read data error"})
				return
			}

			ctx.JSON(iris.Map{"msg": "OK", "picb64": base64.RawStdEncoding.EncodeToString(b)})
		}
		return
	}
	ctx.NotFound()
}

func authRef(ctx iris.Context) {
	if id := ctx.PostValue("id"); id != "" {
		if ret, err := authdb.GetIdItem(id); err == nil {
			if j, err := ret.GetImgJSON(); err == nil {
				ctx.JSON(iris.Map{"msg": "OK", "dataset": j})
				return
			}
		}
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"msg": "FAIL", "error": "Input error"})
	} else {
		ctx.NotFound()
	}
}

func authSubmit(ctx iris.Context) {
	// TODO: follow process are demo only
	id := ctx.FormValue("id")
	if id == "" || authdb.CheckID(id) == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"msg": "FAIL", "error": "missed infomation"})
		return
	}

	v, err := strconv.Atoi(ctx.FormValue("num"))
	if err != nil || v == 0 {
		ctx.JSON(iris.Map{"msg": "FAIL", "error": "no file uploaded"})
		return
	}
	ferr := func() {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"msg": "FAIL", "error": "server file read error"})
	}
	var strid string
	if id, err := uuid.NewV4(); err == nil {
		strid = id.String()
	} else {
		ferr()
		return
	}

	totalScore := []float64{}
	for i := 0; i < v; i++ {
		imgstr := "img" + strconv.Itoa(i)
		f, fh, err := ctx.FormFile(imgstr)
		if err != nil {
			ferr()
			return
		}
		defer f.Close()

		fpath := "./uploadtmp/" + strid
		os.MkdirAll(fpath, 0750)

		out, err := os.OpenFile(fpath+"/"+imgstr+filepath.Ext(fh.Filename), os.O_RDWR|os.O_CREATE, 0640)
		if err != nil {
			return
		}
		defer out.Close()
		if _, err := io.Copy(out, f); err != nil {
			logio.Printf("file write error: %s", err)
			ferr()
			return
		}
		out.Seek(0, 0)
		if s, err := testResult(id, i, out); err != nil {
			fmt.Println(err)
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"msg": "FAIL", "error": "input data error: " + err.Error()})
			return
		} else {
			totalScore = append(totalScore, s)
		}
		out.Close()
	}

	final := 0.0
	if len(totalScore) > 0 {
		for _, v := range totalScore {
			final += v
		}
		final = 1.0 - (final / float64(len(totalScore)))
		if final < 0 {
			final = 0
		}
		if final > 1 {
			final = 1
		}
	}

	ctx.JSON(iris.Map{"msg": "OK", "score": fmt.Sprintf("%.2f%%", final*100)})
	if err := os.RemoveAll("./uploadtmp/" + strid); err != nil {
		log.Printf("remove error: %s\n", err)
	}
}

// TODO: demo only
func testResult(id string, num int, f *os.File) (score float64, err error) {
	img, ierr := jpeg.Decode(f)
	// img, ierr := png.Decode(&f)
	if ierr != nil {
		err = errors.New("read image error: " + ierr.Error())
		return
	}
	bounds := img.Bounds()

	histogram := MakeHistogram(bounds, img)
	// fmt.Printf("%-14s %6s %6s %6s %6s\n", "bin", "red", "green", "blue", "alpha")
	// for i, x := range histogram {
	// 	fmt.Printf("0x%04x-0x%04x: %6f %6f %6f %6f\n", i<<12, (i+1)<<12-1, x[0], x[1], x[2], x[3])
	// }

	score = authdb.AuthDemo(id, num, histogram)

	return
}

// TODO: demo only
func MakeHistogram(bounds image.Rectangle, img image.Image) (finHistogram histogramData) {
	var histogram [16][4]int
	var total float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// A color's RGBA method returns values in the range [0, 65535].
			// Shifting by 12 reduces this to the range [0, 15].
			histogram[r>>12][0]++
			histogram[g>>12][1]++
			histogram[b>>12][2]++
			histogram[a>>12][3]++
			total++
		}
	}

	// Print the results.
	// fmt.Printf("%-14s %6s %6s %6s %6s\n", "bin", "red", "green", "blue", "alpha")
	// for i, x := range histogram {
	// 	fmt.Printf("0x%04x-0x%04x: %6f %6f %6f %6f\n", i<<12, (i+1)<<12-1,
	// 		float64(x[0])/total, float64(x[1])/total, float64(x[2])/total, float64(x[3])/total)
	// }

	for i, x := range histogram {
		finHistogram[i][0] = float64(x[0]) / total
		finHistogram[i][1] = float64(x[1]) / total
		finHistogram[i][2] = float64(x[2]) / total
		finHistogram[i][3] = float64(x[3]) / total
	}

	return finHistogram
}

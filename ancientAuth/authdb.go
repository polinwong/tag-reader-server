package ancientauth

import (
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"log"
	"math"
	"os"

	"github.com/kataras/iris/v12"
)

type authDB struct {
	list     map[string]string
	listitem map[string]authItems
}

type authItems struct {
	num  int
	data []string
}

func (a *authItems) AddedData(imgpath string) (err error) {
	if f, e := os.Stat(imgpath); e == nil && !f.IsDir() {
		a.data = append(a.data, imgpath)
		a.num++
	} else {
		err = e
	}

	return
}

func (a *authItems) GetImgJSON() (jobj []iris.Map, err error) {
	for _, v := range a.data {
		if f, e := os.Open(v); e == nil {
			s, _ := f.Stat()
			b := make([]byte, s.Size())
			if _, e := f.Read(b); e == nil {
				jobj = append(jobj, iris.Map{
					"type": "jpg",
					"pic":  base64.RawStdEncoding.EncodeToString(b),
				})
			} else {
				err = e
			}
		} else {
			err = e
		}
	}

	return
}

func MakeAuthDB() (adb *authDB) {
	adb = new(authDB)
	// TODO: debug use list, will change return server list
	adb.list = map[string]string{
		"642326b4-dd3d-4dca-9675-1465d64ffb65": "Model01.jpg", //Code01
		"feb6d230-7b5d-459c-ba0a-b989858be323": "Model02.jpg", //Code02
		"1a5132c3-7432-488a-98ef-bec436ca018f": "Model03.jpg", //Code03
		"0bbe0827-bed0-4c8d-afd2-3fdd356c8781": "Model04.jpg", //Code04
	}
	adb.listitem = make(map[string]authItems)
	{
		i1id := []string{
			"642326b4-dd3d-4dca-9675-1465d64ffb65",
			"feb6d230-7b5d-459c-ba0a-b989858be323",
			"1a5132c3-7432-488a-98ef-bec436ca018f",
		}
		for _, v := range i1id {
			i1 := authItems{}
			flist := []string{
				"./source/authdata/set/" + v + "/img01.jpg",
				"./source/authdata/set/" + v + "/img02.jpg",
			}
			for _, v := range flist {
				if err := i1.AddedData(v); err != nil {
					log.Printf("add data error: %s", err)
				}
			}
			adb.listitem[v] = i1
		}
	}

	return
}

func (adb *authDB) CheckID(id string) (ret string) {
	if f := adb.list[id]; f != "" {
		ret = f
	}
	return
}

func (adb *authDB) GetIdItem(id string) (ret authItems, err error) {
	if curid := adb.listitem[id]; curid.num != 0 {
		ret = curid
	} else {
		err = errors.New("item list is empty")
	}

	return
}

// TODO: demo only
func (adb *authDB) AuthDemo(id string, imgnum int, hisin histogramData) (score float64) {
	score = -1
	if adb.list[id] == "" {
		return
	}

	i := adb.listitem[id]
	var img image.Image
	if f, err := os.Open(i.data[imgnum]); err == nil {
		if img, err = jpeg.Decode(f); err != nil {
			return
		}
		f.Close()
	}

	bounds := img.Bounds()
	histogramData := MakeHistogram(bounds, img)

	var s1, s2, s3, s4 float64
	for i := 0; i < len(histogramData); i++ {
		s1 += math.Abs(histogramData[i][0] - hisin[i][0])
		s2 += math.Abs(histogramData[i][1] - hisin[i][1])
		s3 += math.Abs(histogramData[i][2] - hisin[i][2])
		s4 += math.Abs(histogramData[i][3] - hisin[i][3])
	}
	log.Printf("      Red       green     blue      alpha\n")
	log.Printf("Score %f, %f, %f, %f\n", s1, s2, s3, s4)

	const scorebase = 2.5
	score = (s1 + s2 + s3 + s4) - scorebase

	return
}

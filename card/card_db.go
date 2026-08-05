// 3D Artefact Exhibition backend
// File: verify_base.go, NFC tag record database
// Created by: Kevin Mak, Sep 2021
// (c)Marvel Digital Ltd. 2021

package card

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	uuid "github.com/iris-contrib/go.uuid"
	"github.com/kataras/iris/v12"
	"go.etcd.io/bbolt"
)

const (
	DB_CARD          = "card-data"
	DB_LINK          = "link-data"
	API_SESSION_TIME = time.Hour * 24 * 7 // In hour

	dbAdmin = "admin-record"

	dbAdminHash   = "admin-hash"
	dbAdminHashID = "rec"

	StatusCtrNormal   = "NORMAL"
	StatusCtrJump     = "JUMP"
	StatusCtrRepeated = "REPEATED"

	jsonSign   = "sign"
	jsonCtr    = "ctr"
	jsonLink   = "link"
	jsonFkey   = "filekey"
	jsonStatus = "status"
)

var (
	// ErrCardDBNotExists is card database not existed
	ErrCardDBNotExists = errors.New("bucket not existed")

	// ErrCardIdSignWrong is card input data is not correct format
	ErrCardIdSignWrong = errors.New("card input data is not correct format")

	// ErrCardDBRead is read database error
	ErrCardDBRead = errors.New("read database error")

	// ErrCardDBWrite is write database error
	ErrCardDBWrite = errors.New("write database error")

	// ErrCardDBDel is want to delete id not existed
	ErrCardDBDel = errors.New("want to delete id not existed")

	// ErrCardModelAdd is add model error
	ErrCardModelAdd = errors.New("add model error")

	// ErrCardSessionExpired is session expired
	ErrCardSessionExpired = errors.New("session expired")

	// ErrCardInput is wrong input data
	ErrCardInput = errors.New("wrong input data")

	// ErrCardAdminFail is admin login fail
	ErrCardAdminFail = errors.New("admin login fail")

	// ErrCardModelDelLinking is delete model have record
	ErrCardModelDelLinking = errors.New("cannot delete this model because have linked record, please remove linked tag first")
	// ErrCardModelNotFound is link target model not existed
	ErrCardModelNotFound = errors.New("target model not found, please create the model first")
)

var (
	DbOpen  = BboltOpen
	DbClock Clock
	DbUUID  UUID
)

// ModelInfo the database record entry
type ModelInfo struct {
	id    string
	Name  string
	Desc  string
	Image string
}

type cardData struct {
	sign   string
	link   string
	ctr    []byte
	fkey   string
	status string
}

func init() {
	DbClock = ClockImpl{}
	DbUUID = UUIDImpl{}
}

// NewModelInfo make the new model linking record
func NewModelInfo(name, nft, image, idsrc string) (m ModelInfo, err error) {
	var id uuid.UUID
	if len(idsrc) != 36 {
		id, err = DbUUID.NewV4()
		if err != nil {
			return ModelInfo{}, err
		}
	} else {
		if id, err = uuid.FromString(idsrc); err != nil {
			return
		}
	}
	m = ModelInfo{
		id:    id.String(),
		Name:  name,
		Desc:  nft,
		Image: image,
	}
	return
}

func (m *ModelInfo) GetID() string {
	return m.id
}

func (m *ModelInfo) SetID(id string) {
	m.id = id
}

// CardDatabase database structure for manage NFC tag record
type CardDatabase struct {
	recDb  IDB
	cardDb IDB
}

// CreateCardDB make the CardDatabase entry for read write record in runtime
func CreateCardDB(dbpath string) (d *CardDatabase, err error) {
	if dbpath == "" {
		return nil, errors.New("path cannot empty")
	}
	d = new(CardDatabase)

	// This DB holds admin credentials (admin-hash) and admin login records (admin-record).
	// It can be removed without losing card/model data, but admin password would need re-setup.
	if d.recDb, err = makeDB(path.Join(dbpath, "admin.db"), dbAdmin, dbAdminHash); err != nil {
		return nil, err
	}

	// This DB is keeping the linking of model record and card info, must carefully handle
	if d.cardDb, err = makeDB(path.Join(dbpath, "carddata.db"), DB_CARD, DB_LINK); err != nil {
		return nil, err
	}

	return d, err
}

func CreateCardDBRaw(recDb, cardDb IDB) (d *CardDatabase, err error) {
	d = new(CardDatabase)
	if recDb == nil || cardDb == nil {
		return nil, ErrCardDBRead
	}

	d.recDb = recDb
	d.cardDb = cardDb
	return
}

func makeDB(path string, list ...string) (IDB, error) {
	db, err := DbOpen(path, 0640, &bbolt.Options{})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx ITx) error {
		for _, v := range list {
			_, err := tx.CreateBucketIfNotExists([]byte(v))
			if err != nil {
				return err
			}
		}
		return nil
	})

	return db, err
}

func (b *CardDatabase) Close() {
	if b.cardDb != nil {
		b.cardDb.Close()
	}
	if b.recDb != nil {
		b.recDb.Close()
	}
}

//------------------For model record------------------

func (b *CardDatabase) ModelGetRemainImages(m map[string]bool) (err error) {
	err = b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_LINK))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.ForEach(func(k, v []byte) error {
			data := iris.Map{}
			if err := json.Unmarshal(v, &data); err != nil {
				return err
			}

			var img string
			if data["image"] != nil {
				img = data["image"].(string)
			}

			if m[img] {
				delete(m, img)
			}
			return nil
		})
	})
	return
}

// ModelSearchJSON return a model list filtered by search keyword
func (b *CardDatabase) ModelSearchJSON(key string) (ret []iris.Map) {
	err := b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_LINK))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.ForEach(func(k, v []byte) error {
			if len(ret) >= 50 {
				return nil
			}
			data := iris.Map{}
			if err := json.Unmarshal(v, &data); err != nil {
				return err
			}
			name := strings.ToLower(data["name"].(string))
			if strings.Contains(name, strings.ToLower(key)) {
				ret = append(ret, data)
			}
			return nil
		})
	})
	if err != nil {
		ret = nil
	}
	return
}

// ModelLinkListJSON return to model linking list in JSON format for api
func (b *CardDatabase) ModelLinkListJSON(page, pageMax int) (ret []iris.Map, size int) {
	startNum := page * pageMax
	err := b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_LINK))
		if buk == nil {
			return ErrCardDBNotExists
		}
		count := 0
		size = buk.Stats().KeyN

		return buk.ForEach(func(k, v []byte) error {
			if count < startNum || count > startNum+pageMax-1 {
				count++
				return nil
			}
			data := iris.Map{}
			if err := json.Unmarshal(v, &data); err != nil {
				return err
			}

			data["id"] = string(k)
			ret = append(ret, data)
			count++
			return nil
		})
	})
	if err != nil {
		ret = nil
	}
	return
}

// ModelGetLink return a linked model record
func (b *CardDatabase) ModelGetLink(id string) (data ModelInfo, err error) {
	err = b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_LINK))
		if buk == nil {
			return ErrCardDBNotExists
		}
		ret := buk.Get([]byte(id))
		jsonBuf := iris.Map{}
		if err := json.Unmarshal(ret, &jsonBuf); err != nil {
			return err
		}
		data.id = string(id)
		if jsonBuf["name"] != nil {
			data.Name = jsonBuf["name"].(string)
		}
		if jsonBuf["desc"] != nil {
			data.Desc = jsonBuf["desc"].(string)
		}
		if jsonBuf["image"] != nil {
			data.Image = jsonBuf["image"].(string)
		}
		return nil
	})
	return
}

// ModelAddLink the data entry for add and delete record
func (b *CardDatabase) ModelAddLink(data ModelInfo) error {
	return b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_LINK))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if len(data.id) == 0 {
			return ErrCardModelAdd
		}

		modelId := []byte(data.id)
		info := iris.Map{
			"name":  data.Name,
			"desc":  data.Desc,
			"image": data.Image,
		}
		data, err := json.Marshal(info)
		if err != nil {
			return err
		}

		return buk.Put(modelId, data)
	})
}

// ModelDelLink delete model data without link
func (b *CardDatabase) ModelDelLink(data ModelInfo) error {
	return b.cardDb.Batch(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_LINK))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if buk.Get([]byte(data.id)) == nil {
			return ErrCardDBDel
		}

		bukCard := tx.Bucket([]byte(DB_CARD))
		if bukCard == nil {
			return ErrCardDBNotExists
		}

		err := bukCard.ForEach(func(k, v []byte) error {
			if o, err := b.getUnmarshalJSON(v); err != nil {
				return err
			} else {
				if strings.Compare(data.id, o.link) == 0 {
					return ErrCardModelDelLinking
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		return buk.Delete([]byte(data.id))
	})
}

//-----------------For login verify-------------------

func (b *CardDatabase) GetLoginRecord() (mapData []iris.Map) {
	mapData = make([]iris.Map, 0)
	err := b.recDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdmin))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.ForEach(func(k, v []byte) error {
			time, e := binary.Varint(v)
			if e <= 0 {
				return ErrCardDBRead
			}
			m := iris.Map{}
			m["id"] = string(k)
			m["time"] = time
			mapData = append(mapData, m)

			return nil
		})
	})

	if err != nil {
		return nil
	} else {
		return
	}
}

func (b *CardDatabase) RemoveLoginRecord(id string) error {
	return b.recDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdmin))
		if buk == nil {
			return ErrCardDBNotExists
		}

		return buk.Delete([]byte(id))
	})
}

func (b *CardDatabase) CheckLoginSession(id string) (timeout int64, err error) {
	err = b.recDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdmin))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if v := buk.Get([]byte(id)); v != nil {
			iTime, e := binary.Varint(v)
			if e <= 0 {
				return ErrCardDBRead
			}
			if DbClock.Since(time.Unix(iTime, 0)) > API_SESSION_TIME {
				log.Printf("Expired")
				return ErrCardSessionExpired
			}
			timeout = time.Now().Unix()
			return nil
		} else {
			return ErrCardDBRead
		}
	})

	return
}

func (b *CardDatabase) OnLoginUpdate(input map[string]int64) (err error) {
	return b.recDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdmin))
		if buk == nil {
			return ErrCardDBNotExists
		}
		for i, v := range input {
			timebuf := make([]byte, 8)
			binary.PutVarint(timebuf, v)
			if err = buk.Put([]byte(i), timebuf); err != nil {
				break
			}
		}
		return err
	})
}

func (b *CardDatabase) OnLoginUser() (id string, err error) {
	err = b.recDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdmin))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if uid, err := DbUUID.NewV4(); err != nil {
			return err
		} else {
			id = uid.String()
		}

		now := time.Now().Unix()
		timebuf := make([]byte, 8)
		binary.PutVarint(timebuf, now)
		return buk.Put([]byte(id), timebuf)
	})
	return
}

func (b *CardDatabase) ChangeAdminPW(id, pw, orgid, orgpw string) (err error) {
	return b.recDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdminHash))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if uid, err := DbUUID.NewV4(); err != nil {
			return err
		} else {
			if !b.CheckAdminLogin(orgid, orgpw) {
				return ErrCardAdminFail
			}

			salt := uid.String()
			hash := genPWHashWithSalt(id, pw, []byte(salt))

			if err := buk.Put([]byte("salt"), []byte(salt)); err != nil {
				return ErrCardDBWrite
			}
			if err := buk.Put([]byte(dbAdminHashID), []byte(hash)); err != nil {
				return ErrCardDBWrite
			}
			return nil
		}
	})
}

func (b *CardDatabase) CheckAdminLogin(id, pw string) (chk bool) {
	ret := b.recDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(dbAdminHash))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if rec := buk.Get([]byte(dbAdminHashID)); rec != nil {
			salt := buk.Get([]byte("salt"))
			if salt == nil {
				return ErrCardDBRead
			}

			if strings.Compare(genPWHashWithSalt(id, pw, salt), string(rec)) == 0 {
				return nil
			} else {
				return ErrCardAdminFail
			}
		} else {
			if strings.Compare(GenPWHash(id, pw), password) == 0 {
				return nil
			} else {
				return ErrCardAdminFail
			}
		}
	})

	if ret == nil {
		chk = true
	}
	return
}

func genPWHashWithSalt(id, pw string, salt []byte) (hash string) {
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(id + pw))
	outMAC := mac.Sum(nil)
	hash = fmt.Sprintf("%x", outMAC)
	return
}

//-----------------For card data-------------------

// CardSearchJSON return a card list filtered by search keyword
func (b *CardDatabase) CardSearchJSON(key string) (ret []iris.Map) {
	// The Key search function
	searchFunc := func(k, v []byte) error {
		if len(ret) >= 50 {
			return nil
		}
		hId := hex.EncodeToString(k)
		if !strings.Contains(strings.ToLower(hId), strings.ToLower(key)) {
			return nil
		}
		if o, err := b.getUnmarshalJSON(v); err != nil {
			return err
		} else {
			hSign := o.sign
			link := o.link
			fkey := o.fkey
			ctr, _ := binary.Uvarint(o.ctr)
			status := o.status
			ret = append(ret, iris.Map{
				"id":     hId,
				"sign":   hSign,
				"link":   link,
				"ctr":    ctr,
				"fkey":   fkey,
				"status": status,
			})
		}
		return nil
	}

	err := b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.ForEach(searchFunc)
	})
	if err != nil {
		ret = nil
	}
	return
}

func (b *CardDatabase) getUnmarshalJSON(d []byte) (o cardData, err error) {
	v := iris.Map{}
	if err = json.Unmarshal(d, &v); err != nil {
		return
	} else {
		o.fkey = v[jsonFkey].(string)
		o.sign = v[jsonSign].(string)
		if v[jsonStatus] != nil {
			o.status = v[jsonStatus].(string)
		} else {
			o.status = StatusCtrNormal
		}
		o.link = v[jsonLink].(string)
		var bCtr []byte
		if bCtr, err = base64.StdEncoding.DecodeString(v[jsonCtr].(string)); err != nil {
			return
		}
		o.ctr = bCtr
	}
	return
}

func (b *CardDatabase) GetCardList(page, pageMax int) (ret []iris.Map, size int) {
	startNum := page * pageMax
	err := b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}
		count := 0
		size = buk.Stats().KeyN

		return buk.ForEach(func(k, v []byte) error {
			if count < startNum || count > startNum+pageMax-1 {
				count++
				return nil
			}
			if o, err := b.getUnmarshalJSON(v); err != nil {
				return err
			} else {
				hId := hex.EncodeToString(k)
				hSign := o.sign
				link := o.link
				fkey := o.fkey
				ctr, _ := binary.Uvarint(o.ctr)
				status := o.status
				ret = append(ret, iris.Map{
					"id":     hId,
					"sign":   hSign,
					"link":   link,
					"ctr":    ctr,
					"fkey":   fkey,
					"status": status,
				})
			}
			count++
			return nil
		})
	})

	if err != nil {
		ret = nil
	}
	return
}

func (b *CardDatabase) WriteCardData(cardId, ctr, cardSign []byte, link string) (fkey string, err error) {
	err = b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if len(cardId) != 7 || len(cardSign) != 56 {
			return ErrCardIdSignWrong
		}

		if data := buk.Get(cardId); data != nil {
			jsonBuf := iris.Map{}
			if err := json.Unmarshal(data, &jsonBuf); err != nil {
				return ErrCardDBRead
			}
			jsonBuf[jsonLink] = link
			jsonBuf[jsonSign] = cardSign
			fkey = jsonBuf[jsonFkey].(string)
			if m, err := json.Marshal(jsonBuf); err != nil {
				return err
			} else {
				return buk.Put(cardId, m)
			}
		} else {
			id, err := DbUUID.NewV4()
			if err != nil {
				return err
			}
			fkey = hex.EncodeToString(id.Bytes())
			if m, err := json.Marshal(iris.Map{
				jsonSign:   cardSign,
				jsonCtr:    ctr,
				jsonLink:   link,
				jsonFkey:   fkey,
				jsonStatus: StatusCtrNormal,
			}); err != nil {
				return err
			} else {
				return buk.Put(cardId, m)
			}
		}
	})

	return
}

func (b *CardDatabase) ReadCardData(cardId []byte) (cardSign []byte, link string, err error) {
	err = b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if len(cardId) != 7 {
			return ErrCardIdSignWrong
		}

		if o, err := b.getUnmarshalJSON(buk.Get(cardId)); err != nil {
			return err
		} else {
			cardSign, err = base64.StdEncoding.DecodeString(o.sign)
			if err != nil {
				return err
			}
			link = o.link
			return nil
		}
	})
	return
}

func (b *CardDatabase) ReadCardFilekey(cardId []byte) (fkey, ctr []byte, err error) {
	err = b.cardDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if o, err := b.getUnmarshalJSON(buk.Get(cardId)); err != nil {
			return err
		} else {
			fkey, err = hex.DecodeString(o.fkey)
			if err != nil {
				return err
			}
			ctr = o.ctr
			return nil
		}
	})
	return
}

func (b *CardDatabase) CheckedCardStatus(cardId []byte) (err error) {
	return b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if data := buk.Get(cardId); data != nil {
			jsonBuf := iris.Map{}
			if err := json.Unmarshal(data, &jsonBuf); err != nil {
				return ErrCardDBRead
			}
			jsonBuf[jsonStatus] = StatusCtrNormal
			if m, err := json.Marshal(jsonBuf); err != nil {
				return err
			} else {
				return buk.Put(cardId, m)
			}
		}
		return ErrCardDBNotExists
	})
}

func (b *CardDatabase) UpdateCardPW(cardId []byte, pw string) (err error) {
	return b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if _, err := hex.DecodeString(pw); err != nil || len(pw) != 32 {
			return ErrCardInput
		}

		if data := buk.Get(cardId); data != nil {
			jsonBuf := iris.Map{}
			if err := json.Unmarshal(data, &jsonBuf); err != nil {
				return ErrCardDBRead
			}
			jsonBuf[jsonFkey] = pw
			if m, err := json.Marshal(jsonBuf); err != nil {
				return err
			} else {
				return buk.Put(cardId, m)
			}
		}
		return ErrCardDBNotExists
	})
}

func (b *CardDatabase) UpdateCardLink(cardId []byte, link string) (err error) {
	return b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if link == "" {
			return ErrCardInput
		}
		// Reject if the target model does not exist (keeps FK consistent)
		if _, err := b.ModelGetLink(link); err != nil {
			return ErrCardModelNotFound
		}

		if data := buk.Get(cardId); data != nil {
			jsonBuf := iris.Map{}
			if err := json.Unmarshal(data, &jsonBuf); err != nil {
				return ErrCardDBRead
			}
			jsonBuf[jsonLink] = link
			if m, err := json.Marshal(jsonBuf); err != nil {
				return err
			} else {
				return buk.Put(cardId, m)
			}
		}
		return ErrCardDBNotExists
	})
}

func (b *CardDatabase) UpdateCardCTR(cardId, ctr []byte, status string) (err error) {
	err = b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if data := buk.Get(cardId); data != nil {
			jsonBuf := iris.Map{}
			if err := json.Unmarshal(data, &jsonBuf); err != nil {
				return ErrCardDBRead
			}
			jsonBuf[jsonCtr] = ctr
			if jsonBuf[jsonStatus] != StatusCtrRepeated {
				jsonBuf[jsonStatus] = status
			}
			if m, err := json.Marshal(jsonBuf); err != nil {
				return err
			} else {
				return buk.Put(cardId, m)
			}
		}
		return ErrCardDBNotExists
	})

	return
}

func (b *CardDatabase) DelUpdateCard(cardId []byte, pw string) (err error) {
	return b.cardDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_CARD))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if data := buk.Get(cardId); data != nil {
			jsonBuf := iris.Map{}
			if err := json.Unmarshal(data, &jsonBuf); err != nil {
				return ErrCardDBRead
			}

			fkey := jsonBuf[jsonFkey].(string)
			if strings.Compare(pw, fkey) == 0 {
				return buk.Delete(cardId)
			}
		}
		return ErrCardDBRead
	})
}

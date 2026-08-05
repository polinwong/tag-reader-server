package card_test

import (
	"marveldigital/tag-reader-server/card"
	"os"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

func MakeTempPath() string {
	d, err := os.MkdirTemp("", "tag-server")
	if err != nil {
		panic(d)
	}
	return d
}

func TestTypeUUID(t *testing.T) {
	u := card.UUIDImpl{}
	if _, err := u.NewV4(); err != nil {
		t.Errorf(`Should not error run UUID`)
	}
}

func TestTypeTime(t *testing.T) {
	c := card.ClockImpl{}
	pass := time.Unix(1659283200, 0)
	if d := c.Since(pass); d < 0 {
		t.Error(`Should run Until success`)
	}
}

func TestTypeBBolt(t *testing.T) {
	temppath := MakeTempPath()
	db, err := card.BboltOpen(temppath+"/test.db", 0600, &bbolt.Options{})
	if err != nil {
		t.Error(`Should run BboltOpen success`)
	}

	db.Update(func(i card.ITx) error {
		testName := []byte("test")
		_, err := i.CreateBucketIfNotExists(testName)
		if err != nil {
			t.Errorf(`Should create the name of bucket`)
		}

		buk := i.Bucket(testName)
		if buk == nil {
			t.Errorf(`Should get bucket`)
		}
		return nil
	})

	db.View(func(i card.ITx) error {
		if i == nil {
			t.Errorf(`Should call view and get bbolt Tx`)
		}
		return nil
	})

	db.Batch(func(i card.ITx) error {
		if i == nil {
			t.Errorf(`Should call batch and get bbolt Tx`)
		}
		return nil
	})

	db.Close()
	os.RemoveAll(temppath)
}

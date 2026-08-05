// The type interface for makeing testable code

package card

import (
	"os"
	"time"

	uuid "github.com/iris-contrib/go.uuid"
	"go.etcd.io/bbolt"
)

type ICardDB interface {
	CheckAdminLogin(id, pw string) (chk bool)
	ReadCardFilekey(cardId []byte) (fkey, ctr []byte, err error)
	UpdateCardCTR(cardId, ctr []byte, status string) (err error)
}

type IBucket interface {
	ForEach(fn func(k, v []byte) error) error
	Get(key []byte) []byte
	Put(key []byte, value []byte) error
	Delete(key []byte) error
	Stats() bbolt.BucketStats
}

type ITx interface {
	CreateBucketIfNotExists(name []byte) (IBucket, error)
	Bucket(name []byte) IBucket
}

type IDB interface {
	Close() error
	Update(fn func(ITx) error) error
	View(fn func(ITx) error) error
	Batch(fn func(ITx) error) error
}

// Implement the interface for working on database

type TxImpl struct {
	base *bbolt.Tx
}

func (bb *TxImpl) CreateBucketIfNotExists(name []byte) (IBucket, error) {
	buk, err := bb.base.CreateBucketIfNotExists(name)
	if err != nil {
		return nil, err
	}
	return buk, err
}
func (bb *TxImpl) Bucket(name []byte) IBucket {
	return bb.base.Bucket(name)
}

type DBImpl struct {
	base *bbolt.DB
}

func (bb *DBImpl) Close() error {
	return bb.base.Close()
}

func (bb *DBImpl) Update(fn func(ITx) error) error {
	return bb.base.Update(func(tx *bbolt.Tx) error {
		return fn(&TxImpl{
			base: tx,
		})
	})
}

func (bb *DBImpl) View(fn func(ITx) error) error {
	return bb.base.View(func(tx *bbolt.Tx) error {
		return fn(&TxImpl{
			base: tx,
		})
	})
}

func (bb *DBImpl) Batch(fn func(ITx) error) error {
	return bb.base.Batch(func(tx *bbolt.Tx) error {
		return fn(&TxImpl{
			base: tx,
		})
	})
}

func BboltOpen(path string, mode os.FileMode, options *bbolt.Options) (IDB, error) {
	base, err := bbolt.Open(path, mode, options)
	if err != nil {
		return nil, err
	}
	bb := &DBImpl{
		base: base,
	}
	return bb, err
}

// Clock is time interface for session
type Clock interface {
	Since(t time.Time) time.Duration
}

type ClockImpl struct{}

func (ClockImpl) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// UUID the interface for gen
type UUID interface {
	NewV4() (uuid.UUID, error)
}

type UUIDImpl struct{}

func (UUIDImpl) NewV4() (uuid.UUID, error) {
	return uuid.NewV4()
}

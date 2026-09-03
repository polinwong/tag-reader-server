package card

import (
	"fmt"
	"os"
	"path"
	"testing"

	"go.etcd.io/bbolt"
)

// TestUserDBFileCreated is an inspectable test: it writes userdb.db to a
// persistent location (/tmp/inspect-userdb) instead of a temp dir, so the
// generated file and its buckets can be examined. It does NOT auto-clean.
func TestUserDBFileCreated(t *testing.T) {
	dir := "/tmp/inspect-userdb"
	os.MkdirAll(dir, 0750)

	db, err := CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB failed: %v", err)
	}
	defer db.Close()

	f, err := os.Stat(path.Join(dir, "userdb.db"))
	if err != nil {
		t.Fatalf("userdb.db not created: %v", err)
	}
	fmt.Printf("INSPECT: userdb.db created at %s, size=%d bytes\n", path.Join(dir, "userdb.db"), f.Size())

	b, err := bbolt.Open(path.Join(dir, "userdb.db"), 0640, &bbolt.Options{})
	if err != nil {
		t.Fatalf("bolt open failed: %v", err)
	}
	defer b.Close()
	b.View(func(tx *bbolt.Tx) error {
		for _, n := range []string{DB_USER, DB_USER_SESS, DB_USER_LOG} {
			if tx.Bucket([]byte(n)) != nil {
				fmt.Printf("INSPECT: bucket exists: %s\n", n)
			} else {
				t.Errorf("INSPECT: MISSING bucket: %s", n)
			}
		}
		return nil
	})
}

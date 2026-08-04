package server_test

import (
	"marveldigital/tag-reader-server/server"
	"os"
	"testing"
)

func TestWorkFunc(t *testing.T) {
	pathTest := "somePath"
	t.Run("1.SetDbPath_WrongPath", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fail()
			}
		}()
		server.SetDbPath(pathTest)
	})

	t.Run("2.SetDbPath_WrongFile", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fail()
			}
		}()
		os.WriteFile(pathTest, []byte{}, 0750)
		defer os.Remove(pathTest)
		server.SetDbPath(pathTest)
	})

	t.Run("3.UpdateLogio_CheckNil", func(t *testing.T) {
		if server.UpdateLogio(nil) != server.ErrUpdateLogger {
			t.Fail()
		}
	})
}

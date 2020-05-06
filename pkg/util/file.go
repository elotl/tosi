package util

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func AtomicWriteFile(filename string, buf []byte, mode os.FileMode) error {
	dir := filepath.Dir(filename)
	base := filepath.Base(filename)
	tmpdir, err := ioutil.TempDir(dir, "tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)
	tmpname := filepath.Join(tmpdir, base)
	err = ioutil.WriteFile(tmpname, buf, mode)
	if err != nil {
		return err
	}
	err = os.Rename(tmpname, filename)
	if err != nil {
		return err
	}
	return nil
}

func IsEmptyDir(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true
	}
	return false
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	// We might still have an error, but that's probably related to
	// permissions. Assume the path exists.
	return true
}

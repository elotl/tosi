package util

import (
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

package util

import (
	"io"
	"os"
	"path/filepath"
)

func AtomicWriteFile(filename string, buf []byte, mode os.FileMode) error {
	dir := filepath.Dir(filename)
	base := filepath.Base(filename)
	tmpname := filepath.Join(dir, "."+base)
	f, err := os.OpenFile(tmpname, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(tmpname)
	_, err = f.Write(buf)
	if err != nil {
		return err
	}
	err = RenameFile(tmpname, filename)
	if err != nil {
		return err
	}
	return nil
}

func RenameFile(oldpath, newpath string) error {
	err := os.Link(oldpath, newpath)
	if err != nil {
		return err
	}
	return os.Remove(oldpath)
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

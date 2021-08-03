package state

import (
	"os"
	"time"
)

type fakeFileInfo struct {
	size int64
}

func (fi *fakeFileInfo) Name() string {
	return ""
}

func (fi *fakeFileInfo) Size() int64 {
	return fi.size
}

func (fi *fakeFileInfo) Mode() os.FileMode {
	return os.ModePerm
}

func (fi *fakeFileInfo) ModTime() time.Time {
	return time.Unix(0, 0)
}

func (fi *fakeFileInfo) IsDir() bool {
	return false
}

func (fi *fakeFileInfo) Sys() interface{} {
	return nil
}

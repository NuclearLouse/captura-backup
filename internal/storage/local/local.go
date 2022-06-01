package local

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"captura-backup/internal/storage"
)

type producer struct{}

func NewProducer() storage.Producer {
	return new(producer)
}

func (p *producer) Ping() error {
	return nil
}

func (*producer) Close() error {
	return nil
}

func (*producer) MakedirAll(path string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(path, 0666)
	}
	return nil
}

func (*producer) ReadFile(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (p *producer) SaveFile(path string, reader io.ReadCloser) error {
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	// file, err := os.OpenFile()
	return os.WriteFile(path, bytes, 0777)
}

func (*producer) ReadDir(path string) ([]fs.FileInfo, error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return dir.Readdir(-1)
}

func (*producer) Remove(path string) error {
	return os.Remove(path)
}

func (*producer) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (*producer) Rename(oldname, newname string) error {
	return os.Rename(oldname, newname)
}

func (*producer) DeleteFile(path string) error {
	return os.Remove(path)
}

func (*producer) MakeDir(path string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return os.Mkdir(path, 0700)
	}
	return nil
}

func (*producer) DeleteDir(path string) error {
	return os.Remove(path)
}

func (p *producer) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

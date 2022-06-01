package ftp

import (
	"bytes"
	"captura-backup/internal/storage"
	"errors"
	"fmt"
	"time"

	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"

	"github.com/secsy/goftp"
)

type producer struct {
	c *goftp.Client
}

func NewProducer(client *goftp.Client) storage.Producer {
	return &producer{client}
}

func NewClient(c *storage.RemoteConfig) (*goftp.Client, error) {
	if c.Port == "" {
		c.Port = "21"
	}
	cfg := goftp.Config{
		User:     c.User,
		Password: c.Password,
		Timeout:  time.Duration(c.Timeout) * time.Second,
		Logger:   c.DebugLoger,
	}
	return goftp.DialConfig(cfg, c.Host+":"+c.Port)
}

func (p *producer) Ping() error {
	rawConn, err := p.c.OpenRawConn()
	if err != nil {
		return err
	}
	defer rawConn.Close()

	code, msg, err := rawConn.SendCommand("FEAT")
	if err != nil {
		return err
	}
	if code != 211 || !strings.Contains(msg, "REST") {
		return fmt.Errorf("%d :%s: %w", code, msg, storage.ErrUnsupportedServer)
	}
	return nil
}

func (p *producer) Close() error {
	return p.c.Close()
}

func (p *producer) Stat(path string) (fs.FileInfo, error) {
	return p.c.Stat(path)
}

func (p *producer) ReadFile(path string) (io.ReadCloser, error) {

	pipeReader, pipeWriter := io.Pipe()

	var err error
	go func() {
		err = func() error {
			defer pipeWriter.Close()
			if err := p.c.Retrieve(path, pipeWriter); err != nil {
				return err
			}
			return nil
		}()
	}()

	return pipeReader, err
}

func (p *producer) SaveFile(path string, reader io.ReadCloser) error {
	if reader == nil {
		reader = io.NopCloser(bytes.NewReader([]byte{}))
	}
	return p.c.Store(path, reader)
}

func (p *producer) ReadDir(path string) ([]fs.FileInfo, error) {
	return p.c.ReadDir(path)
}

func (p *producer) Remove(path string) error {
	return p.RemoveAny(path)
}

func (p *producer) RemoveAll(path string) error {
	return p.RemoveAllRecursive(path)
}

func (p *producer) Rename(oldname, newname string) error {
	return p.c.Rename(oldname, newname)
}

func (p *producer) DeleteFile(path string) error {
	return p.c.Delete(path)
}

func (p *producer) MakeDir(path string) error {
	_, err := p.c.Mkdir(path)
	return err
}

func (p *producer) DeleteDir(path string) error {
	return p.c.Rmdir(path)
}

func (p *producer) MakedirAll(path string) error {
	return p.MkdirAll(path)
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error.
// If path is already a directory, MkdirAll does nothing and returns nil.
// If path contains a regular file, an error is returned
func (p *producer) MkdirAll(path string) error {
	// Most of this code mimics https://golang.org/src/os/path.go?s=514:561#L13
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := p.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && path[i-1] == '/' { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && path[j-1] != '/' { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		err = p.MkdirAll(path[0 : j-1])
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	if err = p.MakeDir(path); err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := p.Stat(path)
		// dir, err1 := c.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}

	return nil
}

// Remove removes the specified file or directory. An error will be returned if no
// file or directory with the specified path exists, or if the specified directory
// is not empty.
func (p *producer) RemoveAny(path string) error {

	dir, err := p.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", path, err.Error(), fs.ErrNotExist)
	}
	if dir.IsDir() {
		if err := p.DeleteDir(path); err != nil {
			return fmt.Errorf("%s: %s: Directory is not empty: %w", path, err.Error(), fs.ErrPermission)

		}
		return nil
	}
	if err := p.DeleteFile(path); err != nil {
		return fmt.Errorf("%s: %s: %w", path, err.Error(), fs.ErrInvalid)
	}
	return nil
}

// RemoveAll removes path and any children it contains.
// It removes everything it can but returns the first error
// it encounters. If the path does not exist, RemoveAll
// returns nil (no error).
func (p *producer) RemoveAllRecursive(path string) error {

	if path == "" {
		// fail silently to retain compatibility with previous behavior
		// of RemoveAll. See issue 28830.
		return nil
	}

	// Simple case: if RemoveAny works, we're done.
	err := p.RemoveAny(path)
	switch {
	case err == nil:
		return nil
	case err != nil:
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if !strings.Contains(err.Error(), "Directory is not empty") {
			return err
		}
	}

DIR:
	for {
		infos, err := p.ReadDir(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}

		entities := len(infos)
		if entities == 0 {
			break
		}
		var names []string
		for {
			numErr := 0
			names := func() []string {
				for _, f := range infos {
					names = append(names, f.Name())
				}
				return names
			}()

			if len(names) == 0 {
				break DIR
			}

			for _, name := range names {
				err1 := p.RemoveAllRecursive(path + "/" + name)
				if err == nil {
					err = err1
				}
				if err1 != nil {
					numErr++
				}
			}
			// If we can delete any entry, break to start new iteration.
			// Otherwise, we discard current names, get next entries and try deleting them.
			if numErr != entities {
				break
			}

			if len(names) == 0 {
				break
			}
			if len(names) < entities {
				err1 := p.RemoveAny(path)
				if err1 == nil || (err1 != nil && errors.Is(err1, fs.ErrNotExist)) {
					return nil
				}
				if err != nil {
					return err
				}
			}
		}
	}
	// Remove directory.
	err1 := p.RemoveAny(path)
	if err1 == nil || (err1 != nil && errors.Is(err1, fs.ErrNotExist)) {
		return nil
	}

	return err1
}

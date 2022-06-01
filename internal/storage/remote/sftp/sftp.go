package sftp

import (
	"captura-backup/internal/storage"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"errors"

	gosftp "github.com/pkg/sftp"
)

type producer struct {
	c *gosftp.Client
}

func NewProducer(client *gosftp.Client) storage.Producer {
	return &producer{client}
}

func NewClient(c *storage.RemoteConfig) (*gosftp.Client, error) {
	if c.Port == "" {
		c.Port = "22"
	}
	cfg := &ssh.ClientConfig{
		User:            c.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(c.Timeout) * time.Second,
	}

	switch c.AuthMethod {
	case "key":
		privateKey, err := ioutil.ReadFile(c.PrivateKeyFile)
		if err != nil {
			return nil, err
		}
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		cfg.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		}
	case "password":
		cfg.Auth = []ssh.AuthMethod{
			ssh.Password(c.Password),
		}
	case "keyboard":
		cfg.Auth = []ssh.AuthMethod{
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				// Just sends the password back for all questions
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = c.Password
				}
				return answers, nil
			}),
		}
	default:
		err := errors.New("[" + c.AuthMethod + "] unsupported authentication method")
		return nil, err
	}

	sshClient, err := ssh.Dial("tcp", c.Host+":"+c.Port, cfg)
	if err != nil {
		return nil, err
	}

	return gosftp.NewClient(sshClient, c.ClientOptionsSFTP...)
}

func (p *producer) Ping() error {
	info, err := p.c.Stat("")
	if err != nil {
		return err
	}
	if info == nil {
		return storage.ErrUnsupportedServer
	}
	return nil
}

func (p *producer) Close() error {
	return p.c.Close()
}

func (p *producer) MakedirAll(path string) error {
	return p.c.MkdirAll(path)
}

func (p *producer) ReadFile(path string) (io.ReadCloser, error) {
	return p.c.Open(path)
}

func (p *producer) SaveFile(path string, reader io.ReadCloser) error {
	file, err := p.c.OpenFile(path, os.O_RDWR|os.O_TRUNC|os.O_CREATE)
	// file, err := p.c.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	srcBytes, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	dstBytes, err := file.Write(srcBytes)
	if err != nil {
		return err
	}
	if len(srcBytes) != dstBytes {
		return errors.New("data sizes do not match")
	}
	return nil
}

func (p *producer) ReadDir(path string) ([]fs.FileInfo, error) {
	return p.c.ReadDir(path)
}

func (p *producer) Remove(path string) error {
	err := p.c.Remove(path)
	if err != nil && err == fs.ErrPermission {
		return p.c.RemoveDirectory(path)
	}
	return err
}

func (p *producer) Rename(oldname, newname string) error {
	return p.c.Rename(oldname, newname)
}

func (p *producer) DeleteFile(path string) error {
	return p.c.Remove(path)
}

func (p *producer) MakeDir(path string) error {
	return p.c.Mkdir(path)
}

func (p *producer) DeleteDir(path string) error {
	return p.c.RemoveDirectory(path)
}

func (p *producer) RemoveAll(path string) error {
	return p.RemoveAllRecursive(path)
}

func (p *producer) Stat(path string) (fs.FileInfo, error) {
	return p.c.Stat(path)
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
	// Simple case: if Remove works, we're done.
	err := p.Remove(path)
	switch {
	case err == nil:
		return nil
	case err != nil:
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if status, ok := err.(*gosftp.StatusError); ok {
			if !strings.Contains(status.Error(), "Directory is not empty") {
				return err
			}
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
				for _, info := range infos {
					names = append(names, info.Name())
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
		}

		if len(names) < entities {
			err1 := p.DeleteDir(path)
			if err1 == nil || (err1 != nil && errors.Is(err1, fs.ErrNotExist)) {
				return nil
			}
			if err != nil {
				return err
			}
		}
	}

	// Remove directory.
	err1 := p.DeleteDir(path)
	if err1 == nil || (err1 != nil && errors.Is(err1, fs.ErrNotExist)) {
		return nil
	}

	return err1
}

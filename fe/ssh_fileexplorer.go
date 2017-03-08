package fe

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"

	models "../models"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const DefaultTimeout = 30 * time.Second

type SSHFileExplorer struct {
	FileExplorer
	Host     string
	User     string
	Password string
	client   *ssh.Client
}

func NewSSHFileExplorer(host string, user string, password string) *SSHFileExplorer {
	return &SSHFileExplorer{Host: host, User: user, Password: password}
}

func (fe *SSHFileExplorer) Init() error {
	sshConfig := &ssh.ClientConfig{
		User: fe.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(fe.Password),
		},
	}

	conn, err := net.DialTimeout("tcp", fe.Host, DefaultTimeout)
	if err != nil {
		return err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, fe.Host, sshConfig)
	if err != nil {
		return err
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	fe.client = client

	return nil
}

func normalizePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return "'" + path + "'"
}

func (fe *SSHFileExplorer) ReadFile(path string) ([]byte, error) {
	c, err := sftp.NewClient(fe.client, sftp.MaxPacket(1<<15))
	if err != nil {
		return nil, err
	}
	defer c.Close()

	r, err := c.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// Open file on  local for write
	w, err := ioutil.TempFile("", "gofe-readfile-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(w.Name()) // clean up

	_, err = io.Copy(w, r)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadFile(w.Name())
}

func (fe *SSHFileExplorer) Save(path string, data []byte) error {
	tmpfile, err := ioutil.TempFile("", "gofe-edit-")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name()) // clean up
	_, err = tmpfile.Write(data)
	if err != nil {
		return err
	}

	err = tmpfile.Close()
	if err != nil {
		return err
	}
	f, err := os.Open(tmpfile.Name())
	if err != nil {
		return err
	}

	c, err := sftp.NewClient(fe.client, sftp.MaxPacket(1<<15))
	if err != nil {
		return err
	}
	defer c.Close()

	w, err := c.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = io.Copy(w, f)
	if err != nil {
		return err
	}
	return nil
}

func (fe *SSHFileExplorer) Mkdir(path string, name string) error {
	return fe.ExecOnly("mkdir -p " + normalizePath(path) + "/" + name)
}

func (fe *SSHFileExplorer) ListDir(path string) ([]models.ListDirEntry, error) {
	ls, err := fe.Exec("ls --time-style=long-iso -l " + normalizePath(path))
	if err != nil {
		return nil, err
	}
	return parseLsOutput(string(ls)), nil
}

func (fe *SSHFileExplorer) Move(path string, newPath string) error {
	return fe.ExecOnly("mv " + normalizePath(path) + " " + normalizePath(newPath))
}

func (fe *SSHFileExplorer) Copy(path string, newPath string) error {
	return fe.ExecOnly("cp -r " + normalizePath(path) + " " + normalizePath(newPath))
}

func (fe *SSHFileExplorer) Delete(path string) error {
	return fe.ExecOnly("rm -r " + normalizePath(path))
}

func (fe *SSHFileExplorer) Chmod(path, perms, permsCode string, recusive bool) error {
	if !recusive {
		return fe.ExecOnly("chmod " + permsCode + " " + normalizePath(path))
	}
	return fe.ExecOnly("chmod -r " + permsCode + " " + normalizePath(path))
}

func (fe *SSHFileExplorer) Close() error {
	return fe.client.Close()
}

// Execute cmd on the remote host and return stderr and stdout
func (fe *SSHFileExplorer) Exec(cmd string) ([]byte, error) {
	log.Println(cmd)
	session, err := fe.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CombinedOutput(cmd)
}

func (fe *SSHFileExplorer) ExecOnly(cmd string) error {
	log.Println(cmd)
	session, err := fe.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	_, err1 := session.CombinedOutput(cmd)
	if err1 != nil {
		return err // + " - " + string(out)
	}
	return nil
}

func parseLsOutput(lsout string) []models.ListDirEntry {
	lines := strings.Split(lsout, "\n")
	results := []models.ListDirEntry{}
	for _, line := range lines {
		//fmt.Println(idx, line)
		if len(line) != 0 && !strings.HasPrefix(line, "total") {
			tokens := strings.Fields(line)
			if len(tokens) >= 8 {
				ftype := "file"
				if strings.HasPrefix(tokens[0], "d") {
					ftype = "dir"
				}
				// file name has spaces
				i := strings.Index(line, tokens[7])
				results = append(results, models.ListDirEntry{line[i:], tokens[0], tokens[4], tokens[5] + " " + tokens[6] + ":00", ftype})
			}
		}
	}
	return results
}

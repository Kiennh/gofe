package fe

import (
	models "../models"
)

type FileExplorer interface {
	Init() error
	ListDir(path string) ([]models.ListDirEntry, error)
	Move(path string, newPath string) error
	Copy(path string, newPath string) error
	Delete(path string) error
	Chmod(path, perms, permsCode string, recusive bool) error
	Mkdir(path string, name string) error
	Close() error
	Save(path string, data []byte) error
	ReadFile(path string) ([]byte, error)
}

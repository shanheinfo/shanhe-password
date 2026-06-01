package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

type Extractor interface {
	VerifyPassword(password string) error
	Extract(outputDir string) error
	Close() error
}

type ArchiveType int

const (
	ArchiveZIP ArchiveType = iota
	ArchiveRAR
	Archive7Z
)

func getArchiveType(path string) ArchiveType {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip":
		return ArchiveZIP
	case ".rar":
		return ArchiveRAR
	case ".7z":
		return Archive7Z
	default:
		return -1
	}
}

func NewExtractor(path string, password string) (Extractor, error) {
	switch getArchiveType(path) {
	case ArchiveZIP:
		return NewZipExtractor(path, password)
	case ArchiveRAR:
		return NewRarExtractor(path, password)
	case Archive7Z:
		return New7zExtractor(path, password)
	default:
		return nil, fmt.Errorf("不支持的文件格式: %s", filepath.Ext(path))
	}
}

func isArchiveFile(filename string) bool {
	return getArchiveType(filename) >= 0
}

func extractFileEntry(rc io.Reader, info os.FileInfo, outputDir, name string) error {
	path := filepath.Join(outputDir, name)
	if info.IsDir() {
		return os.MkdirAll(path, os.ModePerm)
	}
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}
	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer outFile.Close()
	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}
	return nil
}

func isPasswordError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "password") || strings.Contains(msg, "需要密码")
}

func isEncryptedError(err error) bool {
	var re *sevenzip.ReadError
	if errors.As(err, &re) && re.Encrypted {
		return true
	}
	return isPasswordError(err)
}

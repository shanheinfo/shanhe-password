package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yeka/zip"
)

type ZipExtractor struct {
	reader   *zip.ReadCloser
	password string
}

func NewZipExtractor(path string, password string) (*ZipExtractor, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("打开ZIP文件失败: %v", err)
	}
	return &ZipExtractor{reader: reader, password: password}, nil
}

func (z *ZipExtractor) VerifyPassword(password string) error {
	for _, file := range z.reader.File {
		if file.IsEncrypted() {
			if password == "" {
				return fmt.Errorf("需要密码")
			}
			file.SetPassword(password)
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("密码错误")
			}
			rc.Close()
			return nil
		}
	}
	return nil
}

func (z *ZipExtractor) Extract(outputDir string) error {
	for _, file := range z.reader.File {
		if file.IsEncrypted() && z.password != "" {
			file.SetPassword(z.password)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(outputDir, file.Name), os.ModePerm); err != nil {
				return err
			}
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("打开压缩文件失败: %v", err)
		}

		err = extractFileEntry(rc, file.FileInfo(), outputDir, file.Name)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (z *ZipExtractor) Close() error {
	return z.reader.Close()
}

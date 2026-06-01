package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nwaples/rardecode"
)

type RarExtractor struct {
	reader   *rardecode.ReadCloser
	password string
	path     string
}

func NewRarExtractor(path string, password string) (*RarExtractor, error) {
	rr, err := rardecode.OpenReader(path, password)
	if err != nil {
		if isPasswordError(err) {
			return nil, fmt.Errorf("密码错误")
		}
		return nil, fmt.Errorf("打开RAR文件失败: %v", err)
	}
	return &RarExtractor{reader: rr, password: password, path: path}, nil
}

func (r *RarExtractor) VerifyPassword(password string) error {
	if r.password == password && r.reader != nil {
		return nil
	}
	oldReader := r.reader
	rr, err := rardecode.OpenReader(r.path, password)
	if err != nil {
		if isPasswordError(err) {
			return fmt.Errorf("密码错误")
		}
		return fmt.Errorf("打开RAR文件失败: %v", err)
	}
	if oldReader != nil {
		oldReader.Close()
	}
	r.reader = rr
	r.password = password
	return nil
}

func (r *RarExtractor) Extract(outputDir string) error {
	for {
		header, err := r.reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取RAR文件失败: %v", err)
		}

		path := filepath.Join(outputDir, header.Name)
		if header.IsDir {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				return fmt.Errorf("创建目录失败: %v", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return fmt.Errorf("创建目录失败: %v", err)
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("创建文件失败: %v", err)
		}
		if _, err := io.Copy(outFile, r.reader); err != nil {
			outFile.Close()
			return fmt.Errorf("写入文件失败: %v", err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("关闭文件失败: %v", err)
		}
	}
	return nil
}

func (r *RarExtractor) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	return nil
}

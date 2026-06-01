package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bodgit/sevenzip"
)

type SevenZExtractor struct {
	reader   *sevenzip.ReadCloser
	password string
	path     string
}

func New7zExtractor(path string, password string) (*SevenZExtractor, error) {
	var r *sevenzip.ReadCloser
	var err error

	if password != "" {
		r, err = sevenzip.OpenReaderWithPassword(path, password)
	} else {
		r, err = sevenzip.OpenReader(path)
	}

	if err != nil {
		var re *sevenzip.ReadError
		if errors.As(err, &re) && re.Encrypted {
			return nil, fmt.Errorf("需要密码")
		}
		if isPasswordError(err) {
			return nil, fmt.Errorf("密码错误")
		}
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf("文件不存在或无法访问: %v", statErr)
		}
		return nil, fmt.Errorf("打开7z文件失败: %v", err)
	}

	return &SevenZExtractor{reader: r, password: password, path: path}, nil
}

func (s *SevenZExtractor) VerifyPassword(password string) error {
	oldReader := s.reader

	var r *sevenzip.ReadCloser
	var err error

	if password != "" {
		r, err = sevenzip.OpenReaderWithPassword(s.path, password)
	} else {
		r, err = sevenzip.OpenReader(s.path)
	}

	if err != nil {
		var re *sevenzip.ReadError
		if errors.As(err, &re) && re.Encrypted {
			return fmt.Errorf("密码错误")
		}
		if isPasswordError(err) {
			return fmt.Errorf("密码错误")
		}
		return fmt.Errorf("打开7z文件失败: %v", err)
	}

	if oldReader != nil {
		oldReader.Close()
	}
	s.reader = r
	s.password = password
	return nil
}

func (s *SevenZExtractor) Extract(outputDir string) error {
	for _, f := range s.reader.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("打开文件失败: %v", err)
		}

		err = extractFileEntry(rc, f.FileInfo(), outputDir, f.Name)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SevenZExtractor) Close() error {
	if s.reader != nil {
		return s.reader.Close()
	}
	return nil
}

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type App struct {
	ctx             context.Context
	selectedArchive string
	outputDir       string
	passwordList    string
	logMessages     []string
	mu              sync.Mutex
	silentMode      int32
}

const maxLogMessages = 200

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) SelectArchive() string {
	selection, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择压缩包",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "压缩文件 (*.zip, *.rar, *.7z)",
				Pattern:     "*.zip;*.rar;*.7z",
			},
		},
	})
	if err != nil {
		log.Println("选择文件时出错:", err)
		a.addLogMessage(fmt.Sprintf("选择文件时出错: %s", err.Error()))
		return ""
	}
	if selection != "" {
		a.selectedArchive = selection
		a.addLogMessage(fmt.Sprintf("已选择压缩包: %s", selection))
		return fmt.Sprintf("已选择: %s", selection)
	}
	return ""
}

func (a *App) CancelArchive() {
	a.selectedArchive = ""
	a.addLogMessage("已取消选择压缩包")
}

func (a *App) UploadPasswordList() string {
	selection, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择密码本",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "文本文件 (.txt)",
				Pattern:     "*.txt",
			},
		},
	})
	if err != nil {
		a.addLogMessage(fmt.Sprintf("选择密码本时出错: %s", err.Error()))
		return ""
	}
	if selection != "" {
		a.passwordList = selection
		a.addLogMessage(fmt.Sprintf("已选择密码本: %s", selection))
		return selection
	}
	return ""
}

func (a *App) SelectOutputDir() string {
	selection, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择解压目录",
	})
	if err != nil {
		log.Println("选择目录时出错:", err)
		a.addLogMessage(fmt.Sprintf("选择解压目录时出错: %s", err.Error()))
		return ""
	}
	if selection != "" {
		a.outputDir = selection
		a.addLogMessage(fmt.Sprintf("已选择解压目录: %s", selection))
		return selection
	}
	return ""
}

func (a *App) addLogMessage(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logMessages = append(a.logMessages, message)
	if len(a.logMessages) > maxLogMessages {
		a.logMessages = a.logMessages[len(a.logMessages)-maxLogMessages:]
	}
	wailsRuntime.EventsEmit(a.ctx, "logUpdate", message)
}

func (a *App) GetLogMessages() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.logMessages
}

func (a *App) readPasswordList() ([]string, error) {
	if a.passwordList == "" {
		a.addLogMessage("未设置密码本路径")
		return nil, nil
	}
	a.addLogMessage(fmt.Sprintf("正在读取密码本: %s", a.passwordList))
	content, err := os.ReadFile(a.passwordList)
	if err != nil {
		a.addLogMessage(fmt.Sprintf("读取密码本文件失败: %v", err))
		return nil, fmt.Errorf("读取密码本失败: %v", err)
	}
	utf8Content, err := simplifiedchinese.GBK.NewDecoder().Bytes(content)
	if err != nil {
		a.addLogMessage("GBK转换失败，尝试直接使用原内容")
		utf8Content = content
	}
	var passwords []string
	scanner := bufio.NewScanner(bytes.NewReader(utf8Content))
	for scanner.Scan() {
		pwd := strings.TrimSpace(scanner.Text())
		if pwd != "" {
			passwords = append(passwords, pwd)
		}
	}
	if err := scanner.Err(); err != nil {
		a.addLogMessage(fmt.Sprintf("解析密码本失败: %v", err))
		return nil, fmt.Errorf("解析密码本失败: %v", err)
	}
	a.addLogMessage(fmt.Sprintf("成功读取密码本，共 %d 个密码", len(passwords)))
	return passwords, nil
}

func (a *App) appendPasswordToPasswordList(password string) error {
	if a.passwordList == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户目录失败: %v", err)
		}
		a.passwordList = filepath.Join(homeDir, "passwords.txt")
		a.addLogMessage(fmt.Sprintf("创建新密码本: %s", a.passwordList))
	}
	fileInfo, err := os.Stat(a.passwordList)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查密码本文件失败: %v", err)
	}
	file, err := os.OpenFile(a.passwordList, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开密码本文件失败: %v", err)
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			a.addLogMessage(fmt.Sprintf("关闭密码本文件失败: %v", err))
		}
	}(file)
	if fileInfo != nil && fileInfo.Size() > 0 {
		if _, err := file.WriteString("\n"); err != nil {
			return fmt.Errorf("写入换行符失败: %v", err)
		}
	}
	if _, err := file.WriteString(password); err != nil {
		return fmt.Errorf("写入密码失败: %v", err)
	}
	a.addLogMessage(fmt.Sprintf("已将密码 [%s] 添加到密码本: %s", password, a.passwordList))
	return nil
}

type VersionInfo struct {
	CurrentVersion string
	LatestVersion  string
	UpdateURL      string
	IsLatest       bool
	Error          string
}

func (a *App) GetVersionInfo() VersionInfo {
	currentVersion := "v1.0.2"
	repoURL := "https://github.com/shanheinfo/shanhe-password"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/shanheinfo/shanhe-password/releases/latest")
	if err != nil {
		return VersionInfo{
			CurrentVersion: currentVersion,
			UpdateURL:      repoURL + "/releases",
			Error:          "无法连接到服务器，请检查网络连接",
		}
	}
	defer func(Body io.ReadCloser) {
		Body.Close()
	}(resp.Body)
	if resp.StatusCode == 404 {
		return VersionInfo{
			CurrentVersion: currentVersion,
			UpdateURL:      repoURL + "/releases",
			Error:          "暂无发布版本",
			IsLatest:       true,
		}
	}
	if resp.StatusCode != 200 {
		return VersionInfo{
			CurrentVersion: currentVersion,
			UpdateURL:      repoURL + "/releases",
			Error:          "获取版本信息失败，服务器响应异常",
		}
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return VersionInfo{
			CurrentVersion: currentVersion,
			UpdateURL:      repoURL + "/releases",
			Error:          "解析版本信息失败",
		}
	}
	return VersionInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		UpdateURL:      release.HTMLURL,
		IsLatest:       strings.TrimPrefix(release.TagName, "v") == strings.TrimPrefix(currentVersion, "v"),
	}
}
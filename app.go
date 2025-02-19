package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"strings"
	"sync"

	"bufio"
	"bytes"
	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yeka/zip"
	"golang.org/x/text/encoding/simplifiedchinese"
)


type App struct {
	ctx             context.Context
	selectedArchive string     // 选中的压缩包路径
	outputDir       string     // 输出目录
	passwordList    string     // 密码本路径
	logMessages     []string   // 存储日志消息
	mu              sync.Mutex // 用于保护日志消息的互斥锁
}


func NewApp() *App {
	return &App{}
}


func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}


func (a *App) SelectArchive() string {
	// 打开文件选择对话框
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
		archiveInfo := fmt.Sprintf("已选择: %s", selection)
		a.addLogMessage(fmt.Sprintf("已选择压缩包: %s", selection))
		return archiveInfo
	}

	return ""
}

// CancelArchive 用于处理取消按钮的点击事件
func (a *App) CancelArchive() {
	a.selectedArchive = ""      // 清空选中的压缩包路径
	a.addLogMessage("已取消选择压缩包")
}

// UploadPasswordList 用于处理上传密码本的按钮点击事件
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

// SelectOutputDir 用于处理选择解压目录的按钮点击事件
func (a *App) SelectOutputDir() string {
	// 打开目录选择对话框
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

// addLogMessage 添加一条日志消息
func (a *App) addLogMessage(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logMessages = append(a.logMessages, message)
	wailsRuntime.EventsEmit(a.ctx, "logUpdate", message) //触发前端日志更新
}

// GetLogMessages 获取所有日志消息
func (a *App) GetLogMessages() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.logMessages
}

// readPasswordList 从密码本文件中读取密码列表
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

	// 尝试将 GBK 转换为 UTF-8
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

// appendPasswordToPasswordList 将密码添加到密码本
func (a *App) appendPasswordToPasswordList(password string) error {
	if a.passwordList == "" {
		// 如果没有密码本，创建一个新的
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户目录失败: %v", err)
		}
		a.passwordList = filepath.Join(homeDir, "passwords.txt")
		a.addLogMessage(fmt.Sprintf("创建新密码本: %s", a.passwordList))
	}

	// 检查文件是否存在
	fileInfo, err := os.Stat(a.passwordList)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查密码本文件失败: %v", err)
	}

	// 打开文件用于追加
	file, err := os.OpenFile(a.passwordList, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开密码本文件失败: %v", err)
	}
	defer file.Close()

	// 如果文件存在且不为空，先写入换行符
	if fileInfo != nil && fileInfo.Size() > 0 {
		if _, err := file.WriteString("\n"); err != nil {
			return fmt.Errorf("写入换行符失败: %v", err)
		}
	}

	// 写入密码
	if _, err := file.WriteString(password); err != nil {
		return fmt.Errorf("写入密码失败: %v", err)
	}

	a.addLogMessage(fmt.Sprintf("已将密码 [%s] 添加到密码本: %s", password, a.passwordList))
	return nil
}

// tryPassword 尝试使用密码解压文件
func (a *App) tryPassword(archivePath, password, outputDir string) bool {
	a.addLogMessage(fmt.Sprintf("正在尝试解压: %s", filepath.Base(archivePath)))
	if password != "" {
		a.addLogMessage(fmt.Sprintf("尝试密码: %s", password))
	} else {
		a.addLogMessage("尝试无密码解压...")
	}

	ext := strings.ToLower(filepath.Ext(archivePath))
	var success bool
	var err error

	switch ext {
	case ".zip":
		success, err = a.handleZip(archivePath, password, outputDir)
	case ".rar":
		success, err = a.handleRar(archivePath, password, outputDir)
	case ".7z":
		success, err = a.handle7z(archivePath, password, outputDir)
	default:
		a.addLogMessage("不支持的文件格式")
		return false
	}

	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "password") {
			a.addLogMessage("密码错误")
			return false
		}
		a.addLogMessage(fmt.Sprintf("解压失败: %v", err))
		return false
	}

	return success
}

// handleZip 处理ZIP文件
func (a *App) handleZip(archivePath, password, outputDir string) (bool, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return false, fmt.Errorf("打开ZIP文件失败: %v", err)
	}
	defer reader.Close()

	// 先尝试列出文件，检查密码是否正确
	for _, file := range reader.File {
		if file.IsEncrypted() {
			if password == "" {
				return false, fmt.Errorf("需要密码")
			}
			file.SetPassword(password)
			// 尝试打开文件验证密码
			_, err := file.Open()
			if err != nil {
				return false, fmt.Errorf("密码错误")
			}
			break
		}
	}

	// 密码验证通过，开始解压
	for _, file := range reader.File {
		if file.IsEncrypted() && password != "" {
			file.SetPassword(password)
		}

		filePath := filepath.Join(outputDir, file.Name)
		if file.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return false, fmt.Errorf("创建目录失败: %v", err)
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return false, fmt.Errorf("创建文件失败: %v", err)
		}

		srcFile, err := file.Open()
		if err != nil {
			dstFile.Close()
			return false, fmt.Errorf("打开压缩文件失败: %v", err)
		}

		_, err = io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()

		if err != nil {
			return false, fmt.Errorf("解压文件失败: %v", err)
		}
	}

	return true, nil
}

// handle7z 处理7Z文件
func (a *App) handle7z(archivePath, password, outputDir string) (bool, error) {
	r, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		// 检查文件是否存在且可读
		if _, statErr := os.Stat(archivePath); statErr != nil {
			return false, fmt.Errorf("文件不存在或无法访问: %v", statErr)
		}
		return false, fmt.Errorf("打开7z文件失败: %v", err)
	}
	defer r.Close()

	// 尝试解压
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return false, fmt.Errorf("打开文件失败: %v", err)
		}

		path := filepath.Join(outputDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, os.ModePerm)
			rc.Close()
			continue
		}

		err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
		if err != nil {
			rc.Close()
			return false, fmt.Errorf("创建目录失败: %v", err)
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return false, fmt.Errorf("创建文件失败: %v", err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return false, fmt.Errorf("写入文件失败: %v", err)
		}
	}

	return true, nil
}

// handleRar 处理RAR文件
func (a *App) handleRar(archivePath, password, outputDir string) (bool, error) {
	rr, err := rardecode.OpenReader(archivePath, password)
	if err != nil {
		if strings.Contains(err.Error(), "password") {
			return false, nil 
		}
		return false, fmt.Errorf("打开RAR文件失败: %v", err)
	}
	defer rr.Close()

	for {
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("读取RAR文件失败: %v", err)
		}

		path := filepath.Join(outputDir, header.Name)
		if header.IsDir {
			os.MkdirAll(path, os.ModePerm)
			continue
		}

		err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
		if err != nil {
			return false, fmt.Errorf("创建目录失败: %v", err)
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return false, fmt.Errorf("创建文件失败: %v", err)
		}

		_, err = io.Copy(outFile, rr)
		outFile.Close()
		if err != nil {
			return false, fmt.Errorf("写入文件失败: %v", err)
		}
	}

	return true, nil
}

// processNestedArchives 递归处理嵌套压缩包
func (a *App) processNestedArchives(dir string) {
	a.addLogMessage(fmt.Sprintf("开始扫描嵌套压缩包: %s", dir))

	var wg sync.WaitGroup
	var mu sync.Mutex
	processedFiles := make(map[string]bool)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 检查是否是压缩文件
		if !isArchiveFile(path) {
			return nil
		}

		// 使用互斥锁检查文件是否已处理
		mu.Lock()
		if processedFiles[path] {
			mu.Unlock()
			return nil
		}
		processedFiles[path] = true
		mu.Unlock()

		wg.Add(1)
		go func(archivePath string) {
			defer wg.Done()

			// 构建新的输出目录
			baseName := strings.TrimSuffix(filepath.Base(archivePath), filepath.Ext(archivePath))
			nestedOutputDir := filepath.Join(filepath.Dir(archivePath), "extracted_"+baseName)

			a.addLogMessage(fmt.Sprintf("处理嵌套压缩包: %s", filepath.Base(archivePath)))

			// 创建输出目录
			if err := os.MkdirAll(nestedOutputDir, 0755); err != nil {
				a.addLogMessage(fmt.Sprintf("创建目录失败: %v", err))
				return
			}

			// 尝试解压
			success := false

			// 1. 尝试无密码解压
			a.addLogMessage(fmt.Sprintf("尝试无密码解压嵌套文件: %s", filepath.Base(archivePath)))
			if a.tryPassword(archivePath, "", nestedOutputDir) {
				a.addLogMessage(fmt.Sprintf("成功解压嵌套文件（无密码）: %s", filepath.Base(archivePath)))
				success = true
			} else {
				a.addLogMessage("无密码解压失败，尝试使用已知密码...")
				// 2. 尝试使用已知密码
				if passwords, err := a.readPasswordList(); err == nil && len(passwords) > 0 {
					a.addLogMessage(fmt.Sprintf("开始尝试密码本中的密码，共 %d 个", len(passwords)))
					for i, pwd := range passwords {
						a.addLogMessage(fmt.Sprintf("尝试第 %d/%d 个密码: %s", i+1, len(passwords), pwd))
						if a.tryPassword(archivePath, pwd, nestedOutputDir) {
							a.addLogMessage(fmt.Sprintf("成功解压嵌套文件: %s，密码: %s",
								filepath.Base(archivePath), pwd))
							success = true
							break
						}
					}
				} else {
					a.addLogMessage("未找到可用的密码本或密码本为空")
				}
			}

			if success {
				// 递归处理新解压出来的目录
				a.processNestedArchives(nestedOutputDir)
			} else {
				a.addLogMessage(fmt.Sprintf("无法自动解压嵌套文件: %s，需要手动处理", filepath.Base(archivePath)))
				// 保存当前文件路径和输出目录，供后续处理
				a.selectedArchive = archivePath
				a.outputDir = nestedOutputDir
				// 触发密码输入框
				wailsRuntime.EventsEmit(a.ctx, "needPassword", true)
			}
		}(path)

		return nil
	})

	if err != nil {
		a.addLogMessage(fmt.Sprintf("扫描目录出错: %v", err))
	}

	wg.Wait()
	a.addLogMessage("所有嵌套压缩包处理完成")
}

// isArchiveFile 判断是否是支持的压缩文件格式
func isArchiveFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".zip" || ext == ".rar" || ext == ".7z"
}

// StartExtraction 开始解压过程
func (a *App) StartExtraction(performanceMode string) {
	defer func() {
		if r := recover(); r != nil {
			a.addLogMessage(fmt.Sprintf("程序发生错误: %v", r))
		}
	}()

	if a.selectedArchive == "" {
		a.addLogMessage("请先选择压缩包")
		return
	}

	if a.outputDir == "" {
		a.addLogMessage("请先选择解压目录")
		return
	}

	a.addLogMessage("开始解压流程...")
	a.addLogMessage(fmt.Sprintf("压缩包: %s", a.selectedArchive))
	a.addLogMessage(fmt.Sprintf("输出目录: %s", a.outputDir))

	// 首先尝试无密码解压
	a.addLogMessage("第一步：尝试无密码解压...")
	if a.tryPassword(a.selectedArchive, "", a.outputDir) {
		a.addLogMessage("无密码解压成功！")
		go a.processNestedArchives(a.outputDir)
		return
	}
	a.addLogMessage("无密码解压失败")

	// 尝试密码本中的密码
	if a.passwordList != "" {
		a.addLogMessage("第二步：尝试密码本...")
		passwords, err := a.readPasswordList()
		if err != nil {
			a.addLogMessage(fmt.Sprintf("读取密码本失败: %s", err.Error()))
		} else {
			a.addLogMessage(fmt.Sprintf("成功读取密码本，共 %d 个密码", len(passwords)))
			for i, pwd := range passwords {
				a.addLogMessage(fmt.Sprintf("正在尝试第 %d/%d 个密码: %s", i+1, len(passwords), pwd))
				if a.tryPassword(a.selectedArchive, pwd, a.outputDir) {
					a.addLogMessage(fmt.Sprintf("密码正确: %s", pwd))
					go a.processNestedArchives(a.outputDir)
					return
				}
			}
			a.addLogMessage("密码本中所有密码尝试失败")
		}
	} else {
		a.addLogMessage("未上传密码本")
	}

	a.addLogMessage("第三步：询问是否需要暴力破解...")
	// 直接触发前端显示密码输入框
	wailsRuntime.EventsEmit(a.ctx, "needPassword", true)
}

// CancelPasswordInput 处理取消密码输入
func (a *App) CancelPasswordInput() {
	a.addLogMessage("取消输入密码，开始暴力破解...")
	numWorkers := runtime.NumCPU()
	go a.bruteForce(a.selectedArchive, a.outputDir, numWorkers)
}

// HandleManualPassword 处理手动输入的密码
func (a *App) HandleManualPassword(password string) {
	a.addLogMessage(fmt.Sprintf("尝试输入的密码: %s", password))
	if a.tryPassword(a.selectedArchive, password, a.outputDir) {
		a.addLogMessage(fmt.Sprintf("密码正确: %s", password))
		// 将正确的密码添加到密码本
		if err := a.appendPasswordToPasswordList(password); err != nil {
			a.addLogMessage(fmt.Sprintf("添加密码到密码本失败: %v", err))
		} else {
			a.addLogMessage(fmt.Sprintf("已将密码 [%s] 添加到密码本", password))
		}
		// 处理嵌套压缩包
		go a.processNestedArchives(a.outputDir)
	} else {
		a.addLogMessage("密码错误，重新尝试...")
		// 重新显示密码输入框
		wailsRuntime.EventsEmit(a.ctx, "needPassword", true)
	}
}

// bruteForce 暴力破解
func (a *App) bruteForce(archivePath, outputDir string, numWorkers int) {
	// 用于通知所有 goroutine 停止的通道
	stopChan := make(chan struct{})
	// 用于接收成功找到的密码
	resultChan := make(chan string)
	// 用于等待所有工作协程完成
	var wg sync.WaitGroup

	// 定义字符集
	digits := "0123456789"
	lowerLetters := "abcdefghijklmnopqrstuvwxyz"
	upperLetters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	specialChars := "!@#$%^&()_+-=[]{}|;:,.<>?"

	charSets := []struct {
		chars string
		desc  string
	}{
		{digits, "纯数字"},
		{lowerLetters, "小写字母"},
		{upperLetters, "大写字母"},
		{digits + lowerLetters, "数字+小写字母"},
		{digits + upperLetters, "数字+大写字母"},
		{digits + lowerLetters + upperLetters, "数字+字母"},
		{digits + lowerLetters + upperLetters + specialChars, "全部字符"},
	}

	// 启动工作协程
	jobChan := make(chan string, numWorkers)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for password := range jobChan {
				select {
				case <-stopChan:
					return
				default:
					if a.tryPassword(archivePath, password, outputDir) {
						resultChan <- password
						close(stopChan) 
						return
					}
				}
			}
		}()
	}

	// 启动密码生成协程
	go func() {
		defer close(jobChan)
		for _, set := range charSets {
			select {
			case <-stopChan:
				return
			default:
				a.addLogMessage(fmt.Sprintf("开始尝试%s密码", set.desc))
				// 从1位到8位逐步尝试
				for length := 1; length <= 8; length++ {
					a.generatePasswords(set.chars, length, jobChan, stopChan)
				}
			}
		}
	}()

	// 等待结果
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果
	password := <-resultChan
	if password != "" {
		a.addLogMessage(fmt.Sprintf("暴力破解成功，密码是: %s", password))
		// 将密码添加到密码本
		if err := a.appendPasswordToPasswordList(password); err != nil {
			a.addLogMessage(fmt.Sprintf("添加密码到密码本失败: %v", err))
		}
		// 处理嵌套压缩包
		go a.processNestedArchives(outputDir)
	} else {
		a.addLogMessage("暴力破解失败，未找到正确密码")
	}
}

// generatePasswords 生成密码的辅助函数
func (a *App) generatePasswords(chars string, length int, jobs chan<- string, stop <-chan struct{}) {
	var generate func(prefix string, length int)
	generate = func(prefix string, length int) {
		if length == 0 {
			select {
			case <-stop:
				return
			case jobs <- prefix:
				a.addLogMessage(fmt.Sprintf("正在尝试密码: %s", prefix))
			}
			return
		}
		for _, char := range chars {
			select {
			case <-stop:
				return
			default:
				generate(prefix+string(char), length-1)
			}
		}
	}
	generate("", length)
}

// 定义版本信息结构
type VersionInfo struct {
	CurrentVersion string
	LatestVersion  string
	UpdateURL      string
	IsLatest       bool
	Error          string
}

// 获取版本信息
func (a *App) GetVersionInfo() VersionInfo {
	currentVersion := "1.0.0" 
	
	// 获取 GitHub 最新版本
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/shanhe/shanhe-password/releases/latest")
	
	if err != nil {
		return VersionInfo{
			CurrentVersion: currentVersion,
			Error:         "无法连接到服务器，请检查网络连接",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return VersionInfo{
			CurrentVersion: currentVersion,
			Error:         "获取版本信息失败，服务器响应异常",
		}
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return VersionInfo{
			CurrentVersion: currentVersion,
			Error:         "解析版本信息失败",
		}
	}

	return VersionInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		UpdateURL:      release.HTMLURL,
		IsLatest:       release.TagName == currentVersion,
	}
}

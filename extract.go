package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) tryPassword(archivePath, password, outputDir string) bool {
	if atomic.LoadInt32(&a.silentMode) == 0 {
		a.addLogMessage(fmt.Sprintf("正在尝试解压: %s", filepath.Base(archivePath)))
		if password != "" {
			a.addLogMessage(fmt.Sprintf("尝试密码: %s", password))
		} else {
			a.addLogMessage("尝试无密码解压...")
		}
	}

	ext, err := NewExtractor(archivePath, password)
	if err != nil {
		if isEncryptedError(err) {
			if atomic.LoadInt32(&a.silentMode) == 0 {
				a.addLogMessage("密码错误")
			}
			return false
		}
		a.addLogMessage(fmt.Sprintf("解压失败: %v", err))
		return false
	}
	defer ext.Close()

	if err := ext.VerifyPassword(password); err != nil {
		if isEncryptedError(err) {
			if atomic.LoadInt32(&a.silentMode) == 0 {
				a.addLogMessage("密码错误")
			}
			return false
		}
		a.addLogMessage(fmt.Sprintf("密码验证失败: %v", err))
		return false
	}

	if err := ext.Extract(outputDir); err != nil {
		if isEncryptedError(err) {
			if atomic.LoadInt32(&a.silentMode) == 0 {
				a.addLogMessage("密码错误")
			}
			return false
		}
		a.addLogMessage(fmt.Sprintf("解压失败: %v", err))
		return false
	}

	return true
}

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

	a.addLogMessage("第一步：尝试无密码解压...")
	if a.tryPassword(a.selectedArchive, "", a.outputDir) {
		a.addLogMessage("无密码解压成功！")
		go a.processNestedArchives(a.outputDir)
		return
	}
	a.addLogMessage("无密码解压失败")

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
	wailsRuntime.EventsEmit(a.ctx, "needPassword", true)
}

func (a *App) CancelPasswordInput() {
	a.addLogMessage("取消输入密码，开始暴力破解...")
	numWorkers := runtime.NumCPU()
	go a.bruteForce(a.selectedArchive, a.outputDir, numWorkers)
}

func (a *App) HandleManualPassword(password string) {
	a.addLogMessage(fmt.Sprintf("尝试输入的密码: %s", password))
	if a.tryPassword(a.selectedArchive, password, a.outputDir) {
		a.addLogMessage(fmt.Sprintf("密码正确: %s", password))
		if err := a.appendPasswordToPasswordList(password); err != nil {
			a.addLogMessage(fmt.Sprintf("添加密码到密码本失败: %v", err))
		} else {
			a.addLogMessage(fmt.Sprintf("已将密码 [%s] 添加到密码本", password))
		}
		go a.processNestedArchives(a.outputDir)
	} else {
		a.addLogMessage("密码错误，重新尝试...")
		wailsRuntime.EventsEmit(a.ctx, "needPassword", true)
	}
}

const maxNestedDepth = 10

func (a *App) processNestedArchives(dir string) {
	a.processNestedArchivesWithDepth(dir, 0)
}

func (a *App) processNestedArchivesWithDepth(dir string, depth int) {
	if depth >= maxNestedDepth {
		a.addLogMessage(fmt.Sprintf("嵌套深度达到上限 %d，停止递归: %s", maxNestedDepth, dir))
		return
	}

	a.addLogMessage(fmt.Sprintf("开始扫描嵌套压缩包: %s", dir))

	var wg sync.WaitGroup
	var mu sync.Mutex
	processedFiles := make(map[string]bool)
	sem := make(chan struct{}, 4)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !isArchiveFile(path) {
			return nil
		}

		mu.Lock()
		if processedFiles[path] {
			mu.Unlock()
			return nil
		}
		processedFiles[path] = true
		mu.Unlock()

		wg.Add(1)
		sem <- struct{}{}
		go func(archivePath string) {
			defer func() { <-sem }()
			defer wg.Done()

			baseName := strings.TrimSuffix(filepath.Base(archivePath), filepath.Ext(archivePath))
			nestedOutputDir := filepath.Join(filepath.Dir(archivePath), "extracted_"+baseName)

			a.addLogMessage(fmt.Sprintf("处理嵌套压缩包: %s", filepath.Base(archivePath)))

			if err := os.MkdirAll(nestedOutputDir, 0755); err != nil {
				a.addLogMessage(fmt.Sprintf("创建目录失败: %v", err))
				return
			}

			success := false

			a.addLogMessage(fmt.Sprintf("尝试无密码解压嵌套文件: %s", filepath.Base(archivePath)))
			if a.tryPassword(archivePath, "", nestedOutputDir) {
				a.addLogMessage(fmt.Sprintf("成功解压嵌套文件（无密码）: %s", filepath.Base(archivePath)))
				success = true
			} else {
				a.addLogMessage("无密码解压失败，尝试使用已知密码...")
				if passwords, err := a.readPasswordList(); err == nil && len(passwords) > 0 {
					a.addLogMessage(fmt.Sprintf("开始尝试密码本中的密码，共 %d 个", len(passwords)))
					for i, pwd := range passwords {
						a.addLogMessage(fmt.Sprintf("尝试第 %d/%d 个密码: %s", i+1, len(passwords), pwd))
						if a.tryPassword(archivePath, pwd, nestedOutputDir) {
							a.addLogMessage(fmt.Sprintf("成功解压嵌套文件: %s，密码: %s", filepath.Base(archivePath), pwd))
							success = true
							break
						}
					}
				} else {
					a.addLogMessage("未找到可用的密码本或密码本为空")
				}
			}

			if success {
				a.processNestedArchivesWithDepth(nestedOutputDir, depth+1)
			} else {
				a.addLogMessage(fmt.Sprintf("无法自动解压嵌套文件: %s，需要手动处理", filepath.Base(archivePath)))
				mu.Lock()
				a.selectedArchive = archivePath
				a.outputDir = nestedOutputDir
				mu.Unlock()
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

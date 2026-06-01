package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type BruteForcer struct {
	app        *App
	count      int64
	silentMode int32
	stopOnce   sync.Once
	stopChan   chan struct{}
}

func newBruteForcer(app *App) *BruteForcer {
	return &BruteForcer{
		app:      app,
		stopChan: make(chan struct{}),
	}
}

func (bf *BruteForcer) stop() {
	bf.stopOnce.Do(func() {
		close(bf.stopChan)
	})
}

func (bf *BruteForcer) isStopped() bool {
	select {
	case <-bf.stopChan:
		return true
	default:
		return false
	}
}

func (a *App) bruteForce(archivePath, outputDir string, numWorkers int) {
	bf := newBruteForcer(a)
	atomic.StoreInt64(&bf.count, 0)
	atomic.StoreInt32(&a.silentMode, 1)
	defer atomic.StoreInt32(&a.silentMode, 0)

	resultChan := make(chan string, 1)
	var wg sync.WaitGroup

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

	jobChan := make(chan string, numWorkers)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for password := range jobChan {
				if bf.isStopped() {
					return
				}
				if a.tryPassword(archivePath, password, outputDir) {
					select {
					case resultChan <- password:
					default:
					}
					bf.stop()
					return
				}
			}
		}()
	}

	go func() {
		defer close(jobChan)
		for _, set := range charSets {
			if bf.isStopped() {
				return
			}
			a.addLogMessage(fmt.Sprintf("开始尝试%s密码", set.desc))
			for length := 1; length <= 8; length++ {
				if bf.isStopped() {
					return
				}
				bf.generatePasswords(set.chars, length, jobChan)
			}
		}
	}()

	go func() {
		wg.Wait()
		bf.stop()
		close(resultChan)
	}()

	password, ok := <-resultChan
	if ok && password != "" {
		a.addLogMessage(fmt.Sprintf("暴力破解成功，密码是: %s", password))
		if err := a.appendPasswordToPasswordList(password); err != nil {
			a.addLogMessage(fmt.Sprintf("添加密码到密码本失败: %v", err))
		}
		go a.processNestedArchives(outputDir)
	} else {
		a.addLogMessage("暴力破解失败，未找到正确密码")
	}
}

func (bf *BruteForcer) generatePasswords(chars string, length int, jobs chan<- string) {
	var generate func(prefix string, length int)
	generate = func(prefix string, length int) {
		if length == 0 {
			select {
			case <-bf.stopChan:
				return
			case jobs <- prefix:
				count := atomic.AddInt64(&bf.count, 1)
				if count%100 == 0 {
					bf.app.addLogMessage(fmt.Sprintf("已尝试 %d 个密码...", count))
				}
			}
			return
		}
		for _, char := range chars {
			select {
			case <-bf.stopChan:
				return
			default:
				generate(prefix+string(char), length-1)
			}
		}
	}
	generate("", length)
}

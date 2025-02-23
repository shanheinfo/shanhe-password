# 山河压缩包密码批量解压和破解工具

本项目源自群友的需求，正好我也有对应需求，随即制作而成

一个用于批量解压和破解加密压缩包的工具，自动支持密码本解压，无密码解压，询问用户密码式解压，暴力破解功能，目前支持 ZIP、RAR、7Z 压缩包格式
目前 7z 格式暂不支持有密码解压，zip 和 rar 完全支持所有功能

发行版程序下载：https://github.com/shanheinfo/shanhe-password/releases/latest

项目采用 MIT 协议，请随意使用，而并不需要署名作者信息

PS：不要忘记给本项目加上star，如果有问题可以提交issue
## 程序预览
<img src="frontend/src/assets/docs.png">

## 功能特点

- 支持 ZIP、RAR、7Z 格式的压缩包
- 支持密码本导入
- 支持暴力破解
- 支持嵌套压缩包自动解压

## 使用说明

1. 选择压缩包：点击"选择压缩包"按钮选择要解压的文件
2. 上传密码本（可选）：如果有密码本，可以点击"上传密码本"导入
3. 选择解压目录：选择解压后文件的保存位置
4. 开始解压：点击"开始解压"按钮开始处理

## 开发环境

- Go 1.21+
- Wails v2
- Node.js 16+

## 调试
wails dev

## 构建说明
 wails build -platform windows/amd64 -webview2 embed -o "山河压缩包密码批量解压和破解工具.exe"

## 更新日志

### 1.0.0
- 初始版本发布
- 支持 ZIP、RAR、7Z 格式压缩包解压
- 支持密码本导入和暴力破解  

### 1.0.1
- 修复了 rar 密码压缩的问题
- 修复了 版本号 更新和显示的问题
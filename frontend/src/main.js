// 获取按钮元素
const selectArchiveBtn = document.getElementById('selectArchive');
const cancelArchiveBtn = document.getElementById('cancelArchive');
const uploadPasswordListBtn = document.getElementById('uploadPasswordList');
const selectOutputDirBtn = document.getElementById('selectOutputDir');
const startExtractionBtn = document.createElement('button'); 
const archiveInfoDiv = document.getElementById('archiveInfo');
const logContainer = document.getElementById('logContainer');
const cancelExtractionBtn = document.getElementById('cancelExtraction');

startExtractionBtn.textContent = '开始解压';
startExtractionBtn.id = 'startExtraction';
startExtractionBtn.className = 'button';
startExtractionBtn.innerHTML = '<i class="fas fa-play"></i> 开始解压';

// 获取性能模式选项和密码对话框元素
const performanceOptions = document.getElementsByName('performance');
const passwordDialog = document.getElementById('password-dialog');
const passwordInput = document.getElementById('password-input');
const submitPasswordBtn = document.getElementById('submit-password');
const cancelPasswordBtn = document.getElementById('cancel-password');

// 添加日志的函数
function addLog(message) {
    const logEntry = document.createElement('div');
    logEntry.className = 'log-entry';
    logEntry.textContent = message;
    logContainer.appendChild(logEntry);
    logContainer.scrollTop = logContainer.scrollHeight;
}

// 选择压缩包按钮
selectArchiveBtn.addEventListener('click', async () => {
    try {
        const archiveInfo = await window.go.main.App.SelectArchive();
        if (archiveInfo) {
            selectArchiveBtn.style.display = 'none';
            cancelArchiveBtn.style.display = 'inline-block';
            // 添加开始解压按钮
            selectArchiveBtn.parentNode.insertBefore(startExtractionBtn, selectArchiveBtn.nextSibling);
            // 确保取消解压按钮是隐藏的
            cancelExtractionBtn.style.display = 'none';
        }
    } catch (error) {
        console.error('选择压缩包失败:', error);
    }
});

// 取消按钮
cancelArchiveBtn.addEventListener('click', async () => {
    try {
        await window.go.main.App.CancelArchive();
        selectArchiveBtn.style.display = 'inline-block'; 
        cancelArchiveBtn.style.display = 'none';     
        // 移除开始解压按钮
        if (startExtractionBtn.parentNode) {
            startExtractionBtn.parentNode.removeChild(startExtractionBtn);
        }
    } catch (error) {
        console.error('取消选择失败:', error);
        addLog(`取消选择失败: ${error}`);
    }
});

// 上传密码本按钮
uploadPasswordListBtn.addEventListener('click', async () => {
    try {
        await window.go.main.App.UploadPasswordList();
    } catch (error) {
        console.error('选择密码本失败:', error);
    }
});

// 选择解压目录按钮
selectOutputDirBtn.addEventListener('click', async () => {
    try {
        await window.go.main.App.SelectOutputDir();
    } catch (error) {
        console.error('选择解压目录失败:', error);
    }
});

// 开始解压按钮
startExtractionBtn.addEventListener('click', async () => {
    try {
        // 获取性能模式
        let performanceMode = 'default';
        for (const option of performanceOptions) {
            if (option.checked) {
                performanceMode = option.value;
                break;
            }
        }
        await window.go.main.App.StartExtraction(performanceMode);
    } catch (error) {
        console.error('解压出错:', error);
        addLog(`解压出错: ${error}`);
    }
});

// 监听后端发送的需要密码
window.runtime.EventsOn("needPassword", () => {
    passwordDialog.style.display = 'block';
});

// 提交密码按钮点击
submitPasswordBtn.addEventListener('click', async () => {
    const password = passwordInput.value;
    if (password) {
        passwordDialog.style.display = 'none';
        try {
            await window.go.main.App.HandleManualPassword(password);
        } catch (error) {
            console.error('密码验证失败:', error);
            addLog(`密码验证失败: ${error}`);
        }
    }
    passwordInput.value = '';
});

// 取消密码输入
cancelPasswordBtn.addEventListener('click', () => {
    passwordDialog.style.display = 'none';
    window.go.main.App.CancelPasswordInput();
});

// 监听来自后端的日志更新
window.runtime.EventsOn('logUpdate', (message) => {
    if (message) {
        addLog(message);
    }
});

cancelExtractionBtn.addEventListener('click', async () => {
    try {
        await window.go.main.App.CancelExtraction();
        cancelExtractionBtn.style.display = 'none';
    } catch (error) {
        console.error('取消解压失败:', error);
    }
});

// 检查版本更新
async function checkVersion() {
    const versionSpan = document.getElementById('currentVersion');
    const versionLink = document.getElementById('versionLink');
    
    try {
        const versionInfo = await window.go.main.App.GetVersionInfo();
        
        // 设置链接
        versionLink.href = versionInfo.UpdateURL;
        
        if (versionInfo.Error) {
            versionSpan.textContent = `v${versionInfo.CurrentVersion}`;
            versionSpan.classList.add('version-error');
            versionLink.title = versionInfo.Error;
            versionLink.classList.add('version-error-icon');
            return;
        }

        if (versionInfo.IsLatest) {
            versionSpan.textContent = `v${versionInfo.CurrentVersion}`;
            versionLink.title = "当前已是最新版本";
            versionLink.classList.add('version-latest');
        } else {
            versionSpan.textContent = `发现新版本: v${versionInfo.LatestVersion}`;
            versionSpan.classList.add('version-update');
            versionLink.title = "点击更新";
            versionLink.classList.add('version-update-icon');
        }
    } catch (error) {
        versionSpan.textContent = `v${versionInfo.CurrentVersion}`;
        versionSpan.classList.add('version-error');
        versionLink.title = "版本检查失败";
        versionLink.classList.add('version-error-icon');
    }
}

// 在页面加载时检查版本
document.addEventListener('DOMContentLoaded', checkVersion);
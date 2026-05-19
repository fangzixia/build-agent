// API Adapter - 桌面模式，直接调用 Go Bridge

/** @param {any} error */
function formatWailsError(error) {
    if (error == null) return '任务执行失败';
    if (typeof error === 'string') return error;
    const m = error.message || error.Message;
    if (m) return String(m);
    if (error.error) return String(error.error);
    try {
        const s = String(error);
        if (s && s !== '[object Object]') return s;
    } catch (_) { /* ignore */ }
    try {
        return JSON.stringify(error);
    } catch (_) {
        return '任务执行失败';
    }
}

const WailsAPI = {
    getMode: () => 'desktop',

    // 工作�?
    async getWorkspace() {
        return window.go.desktop.Bridge.GetWorkspace();
    },
    async setWorkspace(req) {
        await window.go.desktop.Bridge.SetWorkspace(req.path || req);
        return { path: req.path || req };
    },
    async openFolderDialog() {
        return window.go.desktop.Bridge.OpenFolderDialog();
    },

    // 配置
    async getSettings() {
        return window.go.desktop.Bridge.GetSettings();
    },
    async getMCPServersJSON() {
        return window.go.desktop.Bridge.GetMCPServersJSON();
    },
    async saveSettings(settings) {
        await window.go.desktop.Bridge.SaveSettings(settings);
        return { success: true };
    },
    /** @param {Record<string, object>} servers mcpServers 映射，与 settings.json 一致 */
    async probeMCP(servers) {
        return window.go.desktop.Bridge.ProbeMCP(servers || {});
    },

    // 文件操作
    async listFiles(path) {
        const files = await window.go.desktop.Bridge.ListFiles(path);
        return { files };
    },
    async readFile(path) {
        const content = await window.go.desktop.Bridge.ReadFile(path);
        return { path, content };
    },
    async saveFile(path, content) {
        await window.go.desktop.Bridge.SaveFile(path, content);
        return { success: true, message: '保存成功' };
    },

    // 需求和评测（从文件系统读取，由 Bridge 提供�?
    async getRequirements() {
        const result = await window.go.desktop.Bridge.GetRequirements();
        return { requirements: result };
    },
    async getEvaluations() {
        const result = await window.go.desktop.Bridge.GetEvaluations();
        return { evaluations: result };
    },

    // 任务执行（非流式�?
    async runTask(agentName, task, filePath = '') {
        return window.go.desktop.Bridge.RunTask(agentName, task);
    },

    // 任务执行（流式）
    async runTaskStreaming(agentName, task, filePath = '', onLog, onDone, onError) {
        const eventName = 'task:progress';

        if (onLog) {
            window.runtime.EventsOn(eventName, (log) => {
                onLog({ type: log.type || 'log', ...log });
            });
        }

        try {
            const result = await window.go.desktop.Bridge.RunTaskWithProgress(agentName, task);
            window.runtime.EventsOff(eventName);
            if (onDone) onDone(result);
        } catch (error) {
            window.runtime.EventsOff(eventName);
            if (onError) {
                onError({ error: formatWailsError(error) });
            } else {
                throw error;
            }
        }
    },
};

window.WailsAPI = WailsAPI;

console.log('[API Adapter] Desktop mode');

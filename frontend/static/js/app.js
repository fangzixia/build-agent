// API 基础 URL
const API_BASE = window.location.origin;

// 全局状态
const state = {
    requirements: [],
    evaluations: [],
    currentAction: null,
    currentRequirement: null,
    executionStartTime: null,
    /** 流式执行所属任务类型（切换卡片时 currentAction 会变，日志须归到发起执行的任务） */
    streamingAction: null,
    /** 每个任务卡片独立的执行日志与结果，互不影响 */
    executionByAction: {},
};

// 工具函数
const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => document.querySelectorAll(selector);

const formatDate = (dateStr) => {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleString('zh-CN');
};

function createEmptyExecutionSlot() {
    return {
        logs: [],
        result: null,
        hasError: false,
        running: false,
        logStartTime: null,
    };
}

function getExecutionSlot(action) {
    if (!action) return createEmptyExecutionSlot();
    if (!state.executionByAction[action]) {
        state.executionByAction[action] = createEmptyExecutionSlot();
    }
    return state.executionByAction[action];
}

/** 切换「执行任务」页内不同卡片时，恢复该卡片对应的进度/结果 UI */
function applyExecutionViewForAction(action) {
    const slot = getExecutionSlot(action);
    const progressEl = $('#execution-progress');
    const resultEl = $('#execution-result');

    if (slot.running) {
        resultEl.style.display = 'none';
        progressEl.style.display = 'block';
        $('#progress-status').textContent = '执行中';
        const w = slot.logs.length > 0 ? Math.min(10 + slot.logs.length * 5, 90) : 5;
        $('#progress-fill').style.width = `${w}%`;
        renderLogsFromSlot(slot);
    } else if (slot.result || slot.logs.length > 0) {
        // 任务已完成：保留日志区域，同时展示结果
        progressEl.style.display = 'block';
        $('#progress-status').textContent = slot.hasError ? '执行失败' : '执行完成';
        $('#progress-fill').style.width = '100%';
        renderLogsFromSlot(slot);
        if (slot.result) {
            renderResultToDOM(slot);
            resultEl.style.display = 'block';
        } else {
            resultEl.style.display = 'none';
        }
    } else {
        progressEl.style.display = 'none';
        resultEl.style.display = 'none';
    }
}

function renderLogsFromSlot(slot) {
    const container = $('#progress-logs');
    container.innerHTML = '';
    if (!slot.logs.length) {
        const row = document.createElement('div');
        row.className = 'log-entry';
        row.innerHTML = '<span class="log-time">00:00:00</span><span class="log-message">准备执行...</span>';
        container.appendChild(row);
        return;
    }
    slot.logs.forEach(({ time, message, type }) => {
        const div = document.createElement('div');
        div.className = `log-entry ${type}`;
        const t1 = document.createElement('span');
        t1.className = 'log-time';
        t1.textContent = time;
        const t2 = document.createElement('span');
        t2.className = 'log-message';
        t2.textContent = message;
        div.appendChild(t1);
        div.appendChild(t2);
        container.appendChild(div);
    });
    container.scrollTop = container.scrollHeight;
}

function renderResultToDOM(slot) {
    const statusEl = $('#result-status');
    const contentEl = $('#result-content');
    contentEl.innerHTML = '';

    if (slot.hasError && slot.result) {
        statusEl.textContent = '执行失败';
        statusEl.className = 'result-status error';
        const errText = slot.result.error || '未知错误';
        const pre = document.createElement('pre');
        pre.style.whiteSpace = 'pre-wrap';
        pre.textContent = errText;
        contentEl.appendChild(pre);
        return;
    }

    if (!slot.result) {
        statusEl.textContent = '';
        statusEl.className = 'result-status';
        return;
    }

    const r = slot.result;
    statusEl.textContent = r.has_error ? '执行完成（有错误）' : '执行成功';
    statusEl.className = `result-status ${r.has_error ? 'error' : 'success'}`;
    const pre = document.createElement('pre');
    pre.style.whiteSpace = 'pre-wrap';
    pre.textContent = r.output || '无输出';
    contentEl.appendChild(pre);

    if (!r.has_error && r.output) {
        const fileMatch = r.output.match(/\.spec\/[^\s]+\.md/g);
        if (fileMatch && fileMatch.length > 0) {
            const filePath = fileMatch[fileMatch.length - 1];
            const viewBtn = document.createElement('button');
            viewBtn.className = 'btn btn-secondary';
            viewBtn.innerHTML = '<span class="btn-icon">👁️</span> 查看生成的文件';
            viewBtn.style.marginTop = '12px';
            viewBtn.onclick = async () => {
                try {
                    const response = await api.readFile(filePath);
                    openFileEditor(filePath, response.content);
                } catch (error) {
                    showNotification('加载文件失败: ' + error.message, 'error');
                }
            };
            contentEl.appendChild(viewBtn);
        }
    }
}

const formatDuration = (ms) => {
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    
    if (hours > 0) {
        return `${hours}:${String(minutes % 60).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
    }
    return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
};

// API 调用 - 使用 WailsAPI 适配层
const api = {
    async call(endpoint, options = {}) {
        // 对于通用的 API 调用，根据端点路由到相应的 WailsAPI 方法
        try {
            if (endpoint === '/v1/requirements') {
                return await WailsAPI.getRequirements();
            } else if (endpoint === '/v1/evaluations') {
                return await WailsAPI.getEvaluations();
            } else if (endpoint === '/v1/config') {
                return await WailsAPI.getConfig();
            } else if (endpoint === '/v1/config/env') {
                if (options.method === 'POST') {
                    return await WailsAPI.saveEnvConfig(JSON.parse(options.body));
                } else {
                    return await WailsAPI.getEnvConfig();
                }
            } else if (endpoint.startsWith('/v1/files/list')) {
                const url = new URL(endpoint, 'http://dummy');
                const path = url.searchParams.get('path') || '';
                return await WailsAPI.listFiles(path);
            } else if (endpoint.startsWith('/v1/files/read')) {
                const url = new URL(endpoint, 'http://dummy');
                const path = url.searchParams.get('path') || '';
                return await WailsAPI.readFile(path);
            } else if (endpoint === '/v1/files/save') {
                const body = JSON.parse(options.body);
                return await WailsAPI.saveFile(body.path, body.content);
            } else {
                throw new Error(`Unsupported endpoint: ${endpoint}`);
            }
        } catch (error) {
            console.error('API Error:', error);
            throw error;
        }
    },

    async runAgent(agent, task, filePath = '') {
        return await WailsAPI.runTask(agent, task, filePath);
    },

    async runAgentStreaming(agent, task, filePath = '', onLog, onDone, onError) {
        return await WailsAPI.runTaskStreaming(agent, task, filePath, onLog, onDone, onError);
    },

    async runBuild(task, filePath = '', onLog, onDone, onError) {
        return await WailsAPI.runTaskStreaming('build', task, filePath, onLog, onDone, onError);
    },

    async getFiles(path) {
        return await WailsAPI.listFiles(path);
    },

    async readFile(path) {
        return await WailsAPI.readFile(path);
    },

    async saveFile(path, content) {
        return await WailsAPI.saveFile(path, content);
    },
};

// 页面导航
function initNavigation() {
    $$('.nav-link').forEach(link => {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            const page = link.dataset.page;
            showPage(page);
        });
    });
}

function showPage(pageName) {
    // 更新导航状态
    $$('.nav-link').forEach(link => {
        link.classList.toggle('active', link.dataset.page === pageName);
    });

    // 显示对应页面
    $$('.page').forEach(page => {
        page.classList.toggle('active', page.id === `${pageName}-page`);
    });

    // 加载页面数据
    loadPageData(pageName);
}

async function loadPageData(pageName) {
    switch (pageName) {
        case 'dashboard':
            await loadDashboard();
            break;
        case 'requirements':
            await loadRequirements();
            break;
        case 'execution':
            if (state.currentAction) {
                applyExecutionViewForAction(state.currentAction);
            } else {
                $('#execution-progress').style.display = 'none';
                $('#execution-result').style.display = 'none';
            }
            break;
        case 'history':
            await loadHistory();
            break;
    }
}

// 仪表盘
async function loadDashboard() {
    try {
        // 加载需求数据
        await loadRequirementsData();
        
        // 更新统计
        updateStats();
        
        // 显示最近需求
        renderRecentRequirements();
        
        // 显示失败项统计
        renderFailureStats();
    } catch (error) {
        console.error('加载仪表盘失败:', error);
    }
}

function updateStats() {
    const total = state.requirements.length;
    const passed = state.requirements.filter(r => r.status === 'passed').length;
    const failed = state.requirements.filter(r => r.status === 'failed').length;
    const passRate = total > 0 ? Math.round((passed / total) * 100) : 0;

    $('#total-requirements').textContent = total;
    $('#passed-requirements').textContent = passed;
    $('#failed-requirements').textContent = failed;
    $('#pass-rate').textContent = `${passRate}%`;
}

function renderRecentRequirements() {
    const container = $('#recent-requirements');
    const recent = state.requirements.slice(0, 5);

    if (recent.length === 0) {
        container.innerHTML = '<div class="empty-state">暂无需求数据</div>';
        return;
    }

    container.innerHTML = recent.map(req => `
        <div class="requirement-item" data-id="${req.id}">
            <div class="requirement-header">
                <span class="requirement-id">${req.id}</span>
                <span class="requirement-status ${req.status}">${getStatusText(req.status)}</span>
            </div>
            <div class="requirement-title">${req.title}</div>
            <div class="requirement-meta">
                <span>📅 ${formatDate(req.createdAt)}</span>
                ${req.score ? `<span>📊 ${req.score}/100</span>` : ''}
                ${req.rounds ? `<span>🔄 ${req.rounds} 轮</span>` : ''}
            </div>
        </div>
    `).join('');

    // 绑定点击事件
    container.querySelectorAll('.requirement-item').forEach(item => {
        item.addEventListener('click', () => {
            const id = item.dataset.id;
            showRequirementDetail(id);
        });
    });
}

function renderFailureStats() {
    const container = $('#failure-stats');
    
    // 统计失败项类别
    const failureCategories = {};
    state.evaluations.forEach(eval => {
        if (eval.failedItems) {
            eval.failedItems.forEach(item => {
                const category = item.category || '其他';
                failureCategories[category] = (failureCategories[category] || 0) + 1;
            });
        }
    });

    const categories = Object.entries(failureCategories);
    
    if (categories.length === 0) {
        container.innerHTML = '<div class="empty-state">暂无失败项数据</div>';
        return;
    }

    container.innerHTML = categories.map(([category, count]) => `
        <div class="failure-item">
            <div class="failure-category">${getCategoryText(category)}</div>
            <div class="failure-count">${count} 项</div>
        </div>
    `).join('');
}

// 需求管理
async function loadRequirements() {
    await loadRequirementsData();
    renderRequirementsList();
}

async function loadRequirementsData() {
    try {
        // 加载真实需求数据
        const reqResponse = await api.call('/v1/requirements');
        state.requirements = reqResponse.requirements || [];
        
        // 加载真实验收数据
        const evalResponse = await api.call('/v1/evaluations');
        state.evaluations = evalResponse.evaluations || [];
    } catch (error) {
        console.error('加载需求数据失败:', error);
        state.requirements = [];
        state.evaluations = [];
    }
}

function renderRequirementsList() {
    const container = $('#requirements-list');

    if (state.requirements.length === 0) {
        container.innerHTML = '<div class="empty-state">暂无需求数据</div>';
        return;
    }

    container.innerHTML = state.requirements.map(req => `
        <div class="requirement-item" data-id="${req.id}">
            <div class="requirement-header">
                <span class="requirement-id">${req.id}</span>
                <span class="requirement-status ${req.status}">${getStatusText(req.status)}</span>
            </div>
            <div class="requirement-title">${req.title}</div>
            <div class="requirement-meta">
                <span>📅 ${formatDate(req.createdAt)}</span>
                ${req.score ? `<span>📊 ${req.score}/100</span>` : ''}
                ${req.rounds ? `<span>🔄 ${req.rounds} 轮</span>` : ''}
            </div>
        </div>
    `).join('');

    // 绑定点击事件
    container.querySelectorAll('.requirement-item').forEach(item => {
        item.addEventListener('click', () => {
            const id = item.dataset.id;
            showRequirementDetail(id);
        });
    });
}

// 执行任务
function initExecution() {
    // 快速操作卡片
    $$('.action-card').forEach(card => {
        card.addEventListener('click', () => {
            const action = card.dataset.action;
            showExecutionForm(action);
        });
    });

    // 表单按钮
    $('#close-execution-form').addEventListener('click', hideExecutionForm);
    $('#cancel-execution-btn').addEventListener('click', hideExecutionForm);
    $('#submit-execution-btn').addEventListener('click', submitExecution);
    $('#new-execution-btn').addEventListener('click', () => {
        hideExecutionResult();
        showPage('execution');
    });
}

async function showExecutionForm(action) {
    state.currentAction = action;
    applyExecutionViewForAction(action);

    const titles = {
        analysis: '分析项目',
        requirements: '创建需求',
        code: '编码实现',
        eval: '验收评测',
        build: '完整构建',
    };

    const placeholders = {
        analysis: '（可选）指定分析范围或重点...',
        requirements: '请描述需求...',
        code: '（可选）指定实现重点...',
        eval: '（可选）指定验收重点...',
        build: '（可选）指定构建参数...',
    };

    $('#execution-form-title').textContent = titles[action] || '执行任务';
    $('#task-input').value = '';
    $('#task-input').placeholder = placeholders[action] || '请输入任务描述...';
    
    // analysis 和其他非 requirements 的任务描述可选
    const taskOptional = action !== 'requirements';
    const label = $('#task-input').previousElementSibling;
    if (label && label.tagName === 'LABEL') {
        label.textContent = taskOptional ? '任务描述（可选）' : '任务描述';
    }
    
    // 根据不同的 action 显示不同的表单字段
    await setupFormFields(action);
    
    $('#execution-form').style.display = 'block';
    $('#task-input').focus();

    $$('.action-card').forEach((c) => {
        c.classList.toggle('action-card--active', c.dataset.action === action);
    });
}

function hideExecutionForm() {
    $('#execution-form').style.display = 'none';
}

// 设置表单字段
async function setupFormFields(action) {
    // 清除之前的动态字段
    const existingDynamicFields = $('#execution-form .form-body').querySelectorAll('.dynamic-field');
    existingDynamicFields.forEach(field => field.remove());
    
    const formBody = $('#execution-form .form-body');
    const actionsDiv = formBody.querySelector('.form-actions');
    
    if (action === 'analysis') {
        // 分析项目：显示设计文档查看按钮
        await addAnalysisFields(formBody, actionsDiv);
    } else if (action === 'requirements') {
        // 创建需求：显示已有需求文件选择
        await addRequirementsFields(formBody, actionsDiv);
    } else if (action === 'code') {
        // 编码实现：需求文件下拉框
        await addCodeFields(formBody, actionsDiv);
    } else if (action === 'eval') {
        // 验收评测：需求文件下拉框 + 评测文件下拉框（只读）
        await addEvalFields(formBody, actionsDiv);
    } else if (action === 'build') {
        // 完整构建：需求文件下拉框
        await addBuildFields(formBody, actionsDiv);
    }
}

// 分析项目的字段
async function addAnalysisFields(formBody, actionsDiv) {
    try {
        const config = await api.call('/v1/config');
        const designPath = config.analysisDesignSpecPath || '.spec/design.md';
        
        // 检查文件是否存在
        let fileExists = false;
        try {
            await api.readFile(designPath);
            fileExists = true;
        } catch (e) {
            // 文件不存在
        }
        
        if (fileExists) {
            const fieldDiv = document.createElement('div');
            fieldDiv.className = 'form-group dynamic-field';
            fieldDiv.innerHTML = `
                <label>项目设计文档</label>
                <div class="file-info-box">
                    <span class="file-path">${designPath}</span>
                    <button type="button" class="btn btn-secondary btn-sm" id="view-design-file-btn">
                        <span class="btn-icon">👁️</span>
                        查看/编辑
                    </button>
                </div>
            `;
            formBody.insertBefore(fieldDiv, actionsDiv);
            
            $('#view-design-file-btn').addEventListener('click', async () => {
                try {
                    const response = await api.readFile(designPath);
                    openFileEditor(designPath, response.content, false);
                } catch (error) {
                    showNotification('加载文件失败: ' + error.message, 'error');
                }
            });
        }
    } catch (error) {
        console.error('加载分析配置失败:', error);
    }
}

// 创建需求的字段
async function addRequirementsFields(formBody, actionsDiv) {
    try {
        // 确保加载需求数据
        if (state.requirements.length === 0) {
            await loadRequirementsData();
        }
        
        const fieldDiv = document.createElement('div');
        fieldDiv.className = 'form-group dynamic-field';
        
        if (state.requirements.length > 0) {
            fieldDiv.innerHTML = `
                <label for="existing-req-select">参考已有需求（可选）</label>
                <div class="select-with-action">
                    <select id="existing-req-select" class="form-control">
                        <option value="">-- 不参考 --</option>
                        ${state.requirements.map(req => 
                            `<option value="${req.path}">${req.id} - ${req.title}</option>`
                        ).join('')}
                    </select>
                    <button type="button" class="btn btn-secondary btn-sm" id="view-req-file-btn" style="display: none;">
                        <span class="btn-icon">👁️</span>
                        查看/编辑
                    </button>
                </div>
            `;
        } else {
            fieldDiv.innerHTML = `
                <label>参考已有需求（可选）</label>
                <div class="form-text">暂无已有需求文件</div>
            `;
        }
        
        formBody.insertBefore(fieldDiv, actionsDiv);
        
        if (state.requirements.length > 0) {
            const selectEl = $('#existing-req-select');
            const viewBtn = $('#view-req-file-btn');
            
            selectEl.addEventListener('change', () => {
                viewBtn.style.display = selectEl.value ? 'inline-block' : 'none';
            });
            
            viewBtn.addEventListener('click', async () => {
                const path = selectEl.value;
                if (!path) return;
                
                try {
                    const response = await api.readFile(path);
                    openFileEditor(path, response.content, false);
                } catch (error) {
                    showNotification('加载文件失败: ' + error.message, 'error');
                }
            });
        }
    } catch (error) {
        console.error('加载需求列表失败:', error);
        // 显示错误提示
        const fieldDiv = document.createElement('div');
        fieldDiv.className = 'form-group dynamic-field';
        fieldDiv.innerHTML = `
            <label>参考已有需求（可选）</label>
            <div class="form-text" style="color: #ef4444;">加载需求列表失败</div>
        `;
        formBody.insertBefore(fieldDiv, actionsDiv);
    }
}

// 编码实现的字段
async function addCodeFields(formBody, actionsDiv) {
    try {
        // 确保加载需求数据
        if (state.requirements.length === 0) {
            await loadRequirementsData();
        }
        
        const fieldDiv = document.createElement('div');
        fieldDiv.className = 'form-group dynamic-field';
        fieldDiv.innerHTML = `
            <label for="code-req-select">需求文件路径（可选）</label>
            <div class="select-with-action">
                <select id="code-req-select" class="form-control">
                    <option value="">-- 使用最新需求 --</option>
                    ${state.requirements.map(req => 
                        `<option value="${req.path}">${req.id} - ${req.title}</option>`
                    ).join('')}
                </select>
                <button type="button" class="btn btn-secondary btn-sm" id="view-code-req-btn" style="display: none;">
                    <span class="btn-icon">👁️</span>
                    查看/编辑
                </button>
            </div>
            <small class="form-text">${state.requirements.length > 0 ? '留空则使用最新需求文件' : '暂无需求文件，留空将使用最新生成的需求'}</small>
        `;
        formBody.insertBefore(fieldDiv, actionsDiv);
        
        const selectEl = $('#code-req-select');
        const viewBtn = $('#view-code-req-btn');
        
        selectEl.addEventListener('change', () => {
            viewBtn.style.display = selectEl.value ? 'inline-block' : 'none';
        });
        
        viewBtn.addEventListener('click', async () => {
            const path = selectEl.value;
            if (!path) return;
            
            try {
                const response = await api.readFile(path);
                openFileEditor(path, response.content, false);
            } catch (error) {
                showNotification('加载文件失败: ' + error.message, 'error');
            }
        });
    } catch (error) {
        console.error('加载需求列表失败:', error);
        // 显示错误提示
        const fieldDiv = document.createElement('div');
        fieldDiv.className = 'form-group dynamic-field';
        fieldDiv.innerHTML = `
            <label for="code-req-select">需求文件路径（可选）</label>
            <div class="form-text" style="color: #ef4444;">加载需求列表失败，留空将使用最新需求</div>
        `;
        formBody.insertBefore(fieldDiv, actionsDiv);
    }
}

// 验收评测的字段
async function addEvalFields(formBody, actionsDiv) {
    try {
        // 确保加载需求数据
        if (state.requirements.length === 0) {
            await loadRequirementsData();
        }
        
        // 需求文件下拉框
        const reqFieldDiv = document.createElement('div');
        reqFieldDiv.className = 'form-group dynamic-field';
        reqFieldDiv.innerHTML = `
            <label for="eval-req-select">需求文件路径（可选）</label>
            <div class="select-with-action">
                <select id="eval-req-select" class="form-control">
                    <option value="">-- 使用最新需求 --</option>
                    ${state.requirements.map(req => 
                        `<option value="${req.path}">${req.id} - ${req.title}</option>`
                    ).join('')}
                </select>
                <button type="button" class="btn btn-secondary btn-sm" id="view-eval-req-btn" style="display: none;">
                    <span class="btn-icon">👁️</span>
                    查看
                </button>
            </div>
            <small class="form-text">${state.requirements.length > 0 ? '留空则使用最新需求文件' : '暂无需求文件，留空将使用最新生成的需求'}</small>
        `;
        formBody.insertBefore(reqFieldDiv, actionsDiv);
        
        // 评测文件下拉框
        const evalFieldDiv = document.createElement('div');
        evalFieldDiv.className = 'form-group dynamic-field';
        evalFieldDiv.innerHTML = `
            <label for="eval-file-select">查看历史评测（可选）</label>
            <div class="select-with-action">
                <select id="eval-file-select" class="form-control">
                    <option value="">-- 选择评测文件 --</option>
                    ${state.evaluations.map(ev => 
                        `<option value="${ev.path}">${ev.requirementId} - 第${ev.round}轮 (${ev.score}分)</option>`
                    ).join('')}
                </select>
                <button type="button" class="btn btn-secondary btn-sm" id="view-eval-file-btn" style="display: none;">
                    <span class="btn-icon">👁️</span>
                    查看
                </button>
            </div>
        `;
        formBody.insertBefore(evalFieldDiv, actionsDiv);
        
        // 需求文件查看按钮
        const reqSelectEl = $('#eval-req-select');
        const reqViewBtn = $('#view-eval-req-btn');
        
        reqSelectEl.addEventListener('change', () => {
            reqViewBtn.style.display = reqSelectEl.value ? 'inline-block' : 'none';
        });
        
        reqViewBtn.addEventListener('click', async () => {
            const path = reqSelectEl.value;
            if (!path) return;
            
            try {
                const response = await api.readFile(path);
                openFileEditor(path, response.content, true); // 只读
            } catch (error) {
                showNotification('加载文件失败: ' + error.message, 'error');
            }
        });
        
        // 评测文件查看按钮
        const evalSelectEl = $('#eval-file-select');
        const evalViewBtn = $('#view-eval-file-btn');
        
        evalSelectEl.addEventListener('change', () => {
            evalViewBtn.style.display = evalSelectEl.value ? 'inline-block' : 'none';
        });
        
        evalViewBtn.addEventListener('click', async () => {
            const path = evalSelectEl.value;
            if (!path) return;
            
            try {
                const response = await api.readFile(path);
                openFileEditor(path, response.content, true); // 只读
            } catch (error) {
                showNotification('加载文件失败: ' + error.message, 'error');
            }
        });
    } catch (error) {
        console.error('加载评测列表失败:', error);
    }
}

// 完整构建的字段
async function addBuildFields(formBody, actionsDiv) {
    try {
        // 确保加载需求数据
        if (state.requirements.length === 0) {
            await loadRequirementsData();
        }
        
        const fieldDiv = document.createElement('div');
        fieldDiv.className = 'form-group dynamic-field';
        fieldDiv.innerHTML = `
            <label for="build-req-select">需求文件路径（可选）</label>
            <div class="select-with-action">
                <select id="build-req-select" class="form-control">
                    <option value="">-- 使用最新需求 --</option>
                    ${state.requirements.map(req => 
                        `<option value="${req.path}">${req.id} - ${req.title}</option>`
                    ).join('')}
                </select>
                <button type="button" class="btn btn-secondary btn-sm" id="view-build-req-btn" style="display: none;">
                    <span class="btn-icon">👁️</span>
                    查看/编辑
                </button>
            </div>
            <small class="form-text">${state.requirements.length > 0 ? '留空则使用最新需求文件' : '暂无需求文件，留空将使用最新生成的需求'}</small>
        `;
        formBody.insertBefore(fieldDiv, actionsDiv);
        
        const selectEl = $('#build-req-select');
        const viewBtn = $('#view-build-req-btn');
        
        selectEl.addEventListener('change', () => {
            viewBtn.style.display = selectEl.value ? 'inline-block' : 'none';
        });
        
        viewBtn.addEventListener('click', async () => {
            const path = selectEl.value;
            if (!path) return;
            
            try {
                const response = await api.readFile(path);
                openFileEditor(path, response.content, false);
            } catch (error) {
                showNotification('加载文件失败: ' + error.message, 'error');
            }
        });
    } catch (error) {
        console.error('加载需求列表失败:', error);
        // 显示错误提示
        const fieldDiv = document.createElement('div');
        fieldDiv.className = 'form-group dynamic-field';
        fieldDiv.innerHTML = `
            <label for="build-req-select">需求文件路径（可选）</label>
            <div class="form-text" style="color: #ef4444;">加载需求列表失败，留空将使用最新需求</div>
        `;
        formBody.insertBefore(fieldDiv, actionsDiv);
    }
}

async function submitExecution() {
    const task = $('#task-input').value.trim();
    let filePath = '';
    
    // 根据不同的 action 获取需求路径
    if (state.currentAction === 'requirements') {
        const selectEl = $('#existing-req-select');
        filePath = selectEl ? selectEl.value : '';
    } else if (state.currentAction === 'code') {
        const selectEl = $('#code-req-select');
        filePath = selectEl ? selectEl.value : '';
    } else if (state.currentAction === 'eval') {
        const selectEl = $('#eval-req-select');
        filePath = selectEl ? selectEl.value : '';
    } else if (state.currentAction === 'build') {
        const selectEl = $('#build-req-select');
        filePath = selectEl ? selectEl.value : '';
    }
    
    hideExecutionForm();
    showExecutionProgress();

    try {
        // 使用流式 API
        await api.runAgentStreaming(
            state.currentAction,
            task,
            filePath,
            // onLog
            (log) => {
                if (log.type === 'start') {
                    addLog(`开始执行: ${log.message}`, 'info');
                } else if (log.type === 'log') {
                    handleLogEvent(log);
                }
            },
            // onDone
            (result) => {
                showExecutionResult(result, false);
            },
            // onError
            (error) => {
                showExecutionResult(error, true);
            }
        );
    } catch (error) {
        showExecutionResult({ error: error.message }, true);
    }
}

function handleLogEvent(log) {
    let message = '';
    let type = 'info';

    if (log.error) {
        message = `❌ 错误: ${log.error}`;
        type = 'error';
    } else if (log.output) {
        // 格式化输出
        if (log.agent_name === 'system') {
            message = `⚙️ ${log.output}`;
            type = 'info';
        } else if (log.tool_name) {
            message = `🔧 ${log.tool_name}: ${truncateLog(log.output)}`;
            type = 'info';
        } else if (log.role === 'assistant') {
            message = `🤖 ${truncateLog(log.output)}`;
            type = 'info';
        } else {
            message = truncateLog(log.output);
            type = 'info';
        }
    }

    if (message) {
        addLog(message, type);

        // 仅当仍停留在发起执行的任务卡片上时更新进度条（避免切换卡片后误改 UI）
        if (state.streamingAction && state.currentAction === state.streamingAction) {
            const currentWidth = parseFloat($('#progress-fill').style.width) || 0;
            if (currentWidth < 90) {
                $('#progress-fill').style.width = `${Math.min(currentWidth + 5, 90)}%`;
            }
        }
    }
}

function truncateLog(text, maxLength = 200) {
    if (!text) return '';
    text = text.trim();
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
}

function showExecutionProgress() {
    state.streamingAction = state.currentAction;
    const slot = getExecutionSlot(state.streamingAction);
    slot.running = true;
    slot.result = null;
    slot.hasError = false;
    slot.logStartTime = Date.now();
    state.executionStartTime = slot.logStartTime;

    $('#execution-progress').style.display = 'block';
    $('#execution-result').style.display = 'none';
    $('#progress-status').textContent = '执行中';
    $('#progress-fill').style.width = '0%';
    const _startNow = new Date();
    const _startTime = `${String(_startNow.getHours()).padStart(2, '0')}:${String(_startNow.getMinutes()).padStart(2, '0')}:${String(_startNow.getSeconds()).padStart(2, '0')}`;
    slot.logs = [{ time: _startTime, message: '准备执行...', type: 'info' }];
    if (state.currentAction === state.streamingAction) {
        renderLogsFromSlot(slot);
    }

    // 清除之前的定时器
    if (state.progressInterval) {
        clearInterval(state.progressInterval);
        state.progressInterval = null;
    }
}

function addLog(message, type = 'info') {
    const action = state.streamingAction || state.currentAction;
    const slot = getExecutionSlot(action);
    const now = new Date();
    const time = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`;
    slot.logs.push({ time, message, type });

    if (state.currentAction !== action) {
        return;
    }

    const logEntry = document.createElement('div');
    logEntry.className = `log-entry ${type}`;
    const t1 = document.createElement('span');
    t1.className = 'log-time';
    t1.textContent = time;
    const t2 = document.createElement('span');
    t2.className = 'log-message';
    t2.textContent = message;
    logEntry.appendChild(t1);
    logEntry.appendChild(t2);

    $('#progress-logs').appendChild(logEntry);
    $('#progress-logs').scrollTop = $('#progress-logs').scrollHeight;
}

function hideExecutionProgress() {
    if (state.progressInterval) {
        clearInterval(state.progressInterval);
        state.progressInterval = null;
    }
    $('#execution-progress').style.display = 'none';
}

function showExecutionResult(result, hasError) {
    const action = state.streamingAction || state.currentAction;
    const slot = getExecutionSlot(action);
    slot.running = false;
    state.streamingAction = null;

    if (hasError) {
        slot.hasError = true;
        slot.result = { error: result.error || result.message || '未知错误' };
    } else {
        slot.hasError = false;
        slot.result = {
            output: result.output,
            has_error: result.has_error,
        };
    }

    // 停止进度定时器但保留日志区域可见
    if (state.progressInterval) {
        clearInterval(state.progressInterval);
        state.progressInterval = null;
    }
    $('#progress-fill').style.width = '100%';
    $('#progress-status').textContent = hasError ? '执行失败' : '执行完成';

    if (state.currentAction === action) {
        renderResultToDOM(slot);
        $('#execution-result').style.display = 'block';
    }

    loadRequirementsData();
}

function hideExecutionResult() {
    $('#execution-result').style.display = 'none';
}

// 验收历史
async function loadHistory() {
    await loadRequirementsData();
    renderHistoryList();
    updateHistoryFilters();
}

function updateHistoryFilters() {
    const filterReq = $('#filter-requirement');
    filterReq.innerHTML = '<option value="">全部需求</option>' +
        state.requirements.map(req => `<option value="${req.id}">${req.id} - ${req.title}</option>`).join('');
}

function renderHistoryList() {
    const container = $('#history-list');
    
    if (state.evaluations.length === 0) {
        container.innerHTML = '<div class="empty-state">暂无验收历史</div>';
        return;
    }

    container.innerHTML = state.evaluations.map(eval => `
        <div class="history-item" data-path="${eval.path}">
            <div class="history-header">
                <span class="history-title">${eval.requirementId} - 第 ${eval.round} 轮验收</span>
                <span class="history-score ${eval.passed ? 'passed' : 'failed'}">${eval.score}/100</span>
            </div>
            <div class="history-meta">
                <span>📅 ${formatDate(eval.evaluatedAt)}</span>
                <span>📊 ${eval.passed ? '✅ 已通过' : '❌ 未通过'}</span>
            </div>
            <div class="history-summary">${eval.summary || '无摘要'}</div>
            <div class="history-actions">
                <button class="btn btn-secondary btn-sm view-eval-btn" data-path="${eval.path}">
                    <span class="btn-icon">👁️</span>
                    查看详情
                </button>
            </div>
        </div>
    `).join('');

    // 绑定查看按钮
    container.querySelectorAll('.view-eval-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const path = btn.dataset.path;
            try {
                const response = await api.readFile(path);
                openFileEditor(path, response.content);
            } catch (error) {
                showNotification('加载文件失败: ' + error.message, 'error');
            }
        });
    });
}

// 模态框
function initModals() {
    // 创建需求模态框
    $('#create-requirement-btn').addEventListener('click', () => {
        showModal('create-requirement-modal');
    });
    
    $('#close-create-requirement-modal').addEventListener('click', () => {
        hideModal('create-requirement-modal');
    });
    
    $('#cancel-create-requirement').addEventListener('click', () => {
        hideModal('create-requirement-modal');
    });
    
    $('#submit-create-requirement').addEventListener('click', async () => {
        const description = $('#requirement-description').value.trim();
        if (!description) {
            alert('请输入需求描述');
            return;
        }
        
        hideModal('create-requirement-modal');
        
        // 切换到执行页面并自动填充
        showPage('execution');
        showExecutionForm('requirements');
        $('#task-input').value = description;
    });
    
    // 需求详情模态框
    $('#close-requirement-detail-modal').addEventListener('click', () => {
        hideModal('requirement-detail-modal');
    });
    
    $('#close-requirement-detail').addEventListener('click', () => {
        hideModal('requirement-detail-modal');
    });

    $('#edit-requirement-detail').addEventListener('click', () => {
        if (state.currentRequirement) {
            hideModal('requirement-detail-modal');
            openFileEditor(state.currentRequirement.path, state.currentRequirement.content);
        }
    });
    
    $('#run-build-from-detail').addEventListener('click', () => {
        hideModal('requirement-detail-modal');
        showPage('execution');
        showExecutionForm('build');
        if (state.currentRequirement) {
            $('#requirements-path-input').value = state.currentRequirement.path || '';
        }
    });

    // 文件编辑器模态框
    initFileEditor();
}

function showModal(modalId) {
    $(`#${modalId}`).classList.add('active');
}

function hideModal(modalId) {
    $(`#${modalId}`).classList.remove('active');
}

function showRequirementDetail(reqId) {
    const req = state.requirements.find(r => r.id === reqId);
    if (!req) return;
    
    state.currentRequirement = req;
    
    $('#requirement-detail-title').textContent = `${req.id} - ${req.title}`;
    $('#requirement-detail-content').innerHTML = formatMarkdown(req.content || '暂无内容');
    
    showModal('requirement-detail-modal');
}

// 工具函数
function getStatusText(status) {
    const texts = {
        passed: '已通过',
        failed: '未通过',
        pending: '待验收',
    };
    return texts[status] || status;
}

function getCategoryText(category) {
    const texts = {
        blocking: '🚫 阻塞性失败',
        contract: '🔗 契约不一致',
        ux: '👤 用户体验问题',
        edge_case: '🔍 边缘情况',
        other: '📌 其他',
    };
    return texts[category] || category;
}

function formatMarkdown(text) {
    // 简单的 Markdown 渲染
    return text
        .replace(/^### (.+)$/gm, '<h3>$1</h3>')
        .replace(/^## (.+)$/gm, '<h2>$1</h2>')
        .replace(/^# (.+)$/gm, '<h1>$1</h1>')
        .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.+?)\*/g, '<em>$1</em>')
        .replace(/`(.+?)`/g, '<code>$1</code>')
        .replace(/^- (.+)$/gm, '<li>$1</li>')
        .replace(/(<li>.*<\/li>)/s, '<ul>$1</ul>')
        .replace(/\n\n/g, '</p><p>')
        .replace(/^(.+)$/gm, '<p>$1</p>');
}

// 文件编辑器
function initFileEditor() {
    const editorState = {
        currentPath: '',
        originalContent: '',
        modified: false,
        readOnly: false,
    };

    // 标签页切换
    $$('.editor-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            const mode = tab.dataset.tab;
            switchEditorMode(mode);
        });
    });

    // 关闭编辑器
    $('#close-file-editor-modal').addEventListener('click', () => {
        if (editorState.modified && !editorState.readOnly) {
            if (!confirm('有未保存的更改，确定要关闭吗？')) {
                return;
            }
        }
        hideModal('file-editor-modal');
    });

    $('#cancel-file-editor').addEventListener('click', () => {
        if (editorState.modified && !editorState.readOnly) {
            if (!confirm('有未保存的更改，确定要取消吗？')) {
                return;
            }
        }
        hideModal('file-editor-modal');
    });

    // 保存文件
    $('#save-file-editor').addEventListener('click', async () => {
        if (editorState.readOnly) {
            showNotification('此文件为只读模式', 'error');
            return;
        }
        
        const content = $('#file-editor-textarea').value;
        try {
            await api.saveFile(editorState.currentPath, content);
            editorState.originalContent = content;
            editorState.modified = false;
            updateEditorStatus('saved');
            
            // 刷新数据
            await loadRequirementsData();
            
            // 显示成功提示
            showNotification('保存成功', 'success');
            
            // 关闭文件编辑器窗口
            setTimeout(() => {
                hideModal('file-editor-modal');
            }, 500);
        } catch (error) {
            showNotification('保存失败: ' + error.message, 'error');
        }
    });

    // 监听内容变化
    $('#file-editor-textarea').addEventListener('input', () => {
        if (editorState.readOnly) return;
        
        const content = $('#file-editor-textarea').value;
        editorState.modified = content !== editorState.originalContent;
        updateEditorStatus(editorState.modified ? 'modified' : 'saved');
        
        // 实时更新预览
        updatePreview(content);
    });

    // 暴露打开编辑器的函数
    window.openFileEditor = async (path, content = null, readOnly = false) => {
        editorState.currentPath = path;
        editorState.readOnly = readOnly;
        
        // 如果没有提供内容，从服务器加载
        if (content === null) {
            try {
                const response = await api.readFile(path);
                content = response.content;
            } catch (error) {
                showNotification('加载文件失败: ' + error.message, 'error');
                return;
            }
        }
        
        editorState.originalContent = content;
        editorState.modified = false;
        
        const textarea = $('#file-editor-textarea');
        textarea.value = content;
        textarea.readOnly = readOnly;
        
        $('#editor-file-path').textContent = path + (readOnly ? ' (只读)' : '');
        $('#file-editor-title').textContent = readOnly ? '查看文件' : '文件编辑';
        
        // 只读模式下隐藏保存按钮
        const saveBtn = $('#save-file-editor');
        if (readOnly) {
            saveBtn.style.display = 'none';
            updateEditorStatus('readonly');
        } else {
            saveBtn.style.display = 'inline-block';
            updateEditorStatus('saved');
        }
        
        updatePreview(content);
        
        // 默认显示编辑模式
        switchEditorMode('edit');
        
        showModal('file-editor-modal');
    };

    function switchEditorMode(mode) {
        $$('.editor-tab').forEach(tab => {
            tab.classList.toggle('active', tab.dataset.tab === mode);
        });

        const container = $('.editor-container');
        const editPane = $('.edit-pane');
        const previewPane = $('.preview-pane');

        if (mode === 'edit') {
            container.classList.remove('split-mode');
            editPane.classList.add('active');
            previewPane.classList.remove('active');
        } else if (mode === 'preview') {
            container.classList.remove('split-mode');
            editPane.classList.remove('active');
            previewPane.classList.add('active');
        } else if (mode === 'split') {
            container.classList.add('split-mode');
            editPane.classList.add('active');
            previewPane.classList.add('active');
        }
    }

    function updatePreview(content) {
        $('#file-editor-preview').innerHTML = formatMarkdown(content);
    }

    function updateEditorStatus(status) {
        const statusEl = $('#editor-status');
        if (status === 'saved') {
            statusEl.textContent = '已保存';
            statusEl.className = 'editor-status saved';
        } else if (status === 'modified') {
            statusEl.textContent = '未保存';
            statusEl.className = 'editor-status modified';
        } else if (status === 'readonly') {
            statusEl.textContent = '只读';
            statusEl.className = 'editor-status readonly';
        }
    }
}

function showNotification(message, type = 'info') {
    // 简单的通知实现
    const notification = document.createElement('div');
    notification.className = `notification notification-${type}`;
    notification.textContent = message;
    notification.style.cssText = `
        position: fixed;
        top: 80px;
        right: 20px;
        padding: 12px 20px;
        background: ${type === 'success' ? '#10b981' : type === 'error' ? '#ef4444' : '#3b82f6'};
        color: white;
        border-radius: 8px;
        box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
        z-index: 10000;
        animation: slideIn 0.3s ease-out;
    `;
    
    document.body.appendChild(notification);
    
    setTimeout(() => {
        notification.style.animation = 'slideOut 0.3s ease-out';
        setTimeout(() => notification.remove(), 300);
    }, 3000);
}

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    initNavigation();
    initExecution();
    initModals();
    
    // 加载默认页面
    loadDashboard();
});

// ==================== 配置页面功能 ====================

// 配置项元数据
const CONFIG_METADATA = {
    // Base 配置
    'WORKSPACE_ROOT': {
        label: '工作空间根目录',
        description: '项目的根目录路径',
        type: 'path',
        required: true,
        section: 'Base',
        fullWidth: true,
    },
    'CMD_TIMEOUT_SEC': {
        label: '命令超时时间（秒）',
        description: '执行命令的最大超时时间',
        type: 'number',
        required: true,
        section: 'Base',
    },
    'HTTP_ADDR': {
        label: 'HTTP 服务地址',
        description: 'HTTP 服务监听地址，格式: :端口 或 主机:端口',
        type: 'text',
        required: true,
        section: 'Base',
    },
    
    // Agent 通用配置模板
    '_OPENAI_BASE_URL': {
        label: 'OpenAI API 地址',
        description: 'OpenAI API 的基础 URL',
        type: 'url',
        required: true,
        fullWidth: true,
    },
    '_OPENAI_API_KEY': {
        label: 'OpenAI API 密钥',
        description: 'OpenAI API 的访问密钥',
        type: 'password',
        required: true,
        fullWidth: true,
    },
    '_OPENAI_MODEL': {
        label: '模型名称',
        description: '使用的 AI 模型名称',
        type: 'text',
        required: true,
    },
    '_EXECUTOR_MAX_ITERATIONS': {
        label: '执行器最大迭代次数',
        description: '执行器的最大迭代次数',
        type: 'number',
        required: true,
    },
    '_PLAN_EXECUTE_MAX_ITERATIONS': {
        label: '计划执行最大迭代次数',
        description: '计划执行的最大迭代次数',
        type: 'number',
        required: true,
    },
    '_DESIGN_SPEC_PATH': {
        label: '设计文档路径',
        description: '设计规格文档的路径',
        type: 'path',
        required: false,
    },
    
    // 特殊配置
    'EVAL_REQUIREMENTS_SPEC_PATH': {
        label: '需求文档路径',
        description: 'EVAL Agent 使用的需求文档路径',
        type: 'path',
        required: false,
        section: 'EVAL',
    },
    'EVAL_PASS_SCORE_THRESHOLD': {
        label: '通过分数阈值',
        description: '验收通过的最低分数',
        type: 'number',
        required: false,
        section: 'EVAL',
    },
    'REQUIREMENTS_SPEC_DIR': {
        label: '规格文档目录',
        description: 'REQUIREMENTS Agent 使用的规格文档目录',
        type: 'path',
        required: false,
        section: 'REQUIREMENTS',
    },
    'BUILD_MAX_RETRIES': {
        label: '最大重试次数',
        description: 'BUILD Agent 的最大重试次数',
        type: 'number',
        required: true,
        section: 'BUILD',
    },
};

// 区块配置
const SECTION_CONFIG = {
    'Base': {
        title: '基础配置',
        icon: '⚙️',
        order: 0,
    },
    'CODE': {
        title: 'CODE Agent',
        icon: '💻',
        order: 1,
    },
    'ANALYSIS': {
        title: 'ANALYSIS Agent',
        icon: '🔍',
        order: 2,
    },
    'EVAL': {
        title: 'EVAL Agent',
        icon: '✓',
        order: 3,
    },
    'REQUIREMENTS': {
        title: 'REQUIREMENTS Agent',
        icon: '📝',
        order: 4,
    },
    'BUILD': {
        title: 'BUILD Agent',
        icon: '🔄',
        order: 5,
    },
};

// 配置页面状态
const configPageState = {
    originalConfig: null,
    currentConfig: null,
    modified: false,
    loading: false,
};

// 配置页面控制器 - 简化版（文本编辑）
class ConfigPageController {
    constructor() {
        this.envContent = '';
        this.exampleContent = '';
        this.originalEnvContent = '';
        this.currentFile = 'env'; // 'env' or 'example'
        this.initEventListeners();
    }

    initEventListeners() {
        $('#save-config-btn')?.addEventListener('click', () => this.saveConfig());
        $('#reset-config-btn')?.addEventListener('click', () => this.resetConfig());
        
        // 文件切换标签
        document.querySelectorAll('.config-tab').forEach(tab => {
            tab.addEventListener('click', () => {
                const file = tab.dataset.file;
                this.switchFile(file);
            });
        });
    }

    switchFile(file) {
        if (this.currentFile === file) return;
        
        // 保存当前编辑内容
        const textarea = $('#config-textarea');
        if (this.currentFile === 'env') {
            this.envContent = textarea.value;
        }
        
        // 切换文件
        this.currentFile = file;
        
        // 更新标签状态
        document.querySelectorAll('.config-tab').forEach(tab => {
            tab.classList.toggle('active', tab.dataset.file === file);
        });
        
        // 更新编辑器内容和状态
        if (file === 'env') {
            textarea.value = this.envContent;
            textarea.readOnly = false;
            this.updateHint('.env 文件是实际使用的配置文件，修改后点击保存生效。', '#3b82f6', '#f3f4f6');
            $('#save-config-btn').disabled = false;
            $('#reset-config-btn').disabled = false;
        } else {
            textarea.value = this.exampleContent;
            textarea.readOnly = true;
            this.updateHint('.env.example 是配置模板文件，仅供参考，不可编辑。', '#6b7280', '#f9fafb');
            $('#save-config-btn').disabled = true;
            $('#reset-config-btn').disabled = true;
        }
    }

    updateHint(text, borderColor, bgColor) {
        const hint = $('.config-hint');
        hint.textContent = text;
        hint.style.borderLeftColor = borderColor;
        hint.style.backgroundColor = bgColor;
    }

    async loadConfig() {
        const loadingEl = $('#config-loading');
        const errorEl = $('#config-error');
        const editorContainer = $('#config-editor-container');

        loadingEl.style.display = 'block';
        errorEl.style.display = 'none';
        editorContainer.style.display = 'none';

        try {
            // 加载 .env 文件 - 使用 WailsAPI
            const envConfig = await WailsAPI.getEnvConfig();
            this.envContent = this.configToEnvText(envConfig);
            this.originalEnvContent = this.envContent;

            // 显示在文本编辑器中
            const textarea = $('#config-textarea');
            textarea.value = this.envContent;
            textarea.readOnly = false;

            // 检查是否为空配置（.env 文件不存在）
            if (!this.envContent.trim() || Object.keys(envConfig.sections || {}).length === 0) {
                this.updateHint('⚠️ .env 文件不存在或为空，建议参考 .env.example 模板进行配置。', '#f59e0b', '#fffbeb');
            } else {
                this.updateHint('.env 文件是实际使用的配置文件，修改后点击保存生效。', '#3b82f6', '#f3f4f6');
            }

            loadingEl.style.display = 'none';
            editorContainer.style.display = 'block';
        } catch (error) {
            console.error('加载配置失败:', error);
            loadingEl.style.display = 'none';
            errorEl.textContent = `加载配置失败: ${error.message}`;
            errorEl.style.display = 'block';
        }
    }

    configToEnvText(config) {
        let text = '';
        const sections = config.sections || {};
        const sectionOrder = ['Base', 'CODE', 'ANALYSIS', 'EVAL', 'REQUIREMENTS', 'BUILD'];

        sectionOrder.forEach((sectionName, index) => {
            const entries = sections[sectionName];
            if (!entries || entries.length === 0) return;

            // 添加区块注释
            if (index > 0) {
                text += '\n';
            }
            if (sectionName !== 'Base') {
                text += `# ${sectionName.toLowerCase()} scenario (OPENAI fully isolated)\n`;
            }

            // 添加配置项
            entries.forEach(entry => {
                if (entry.comment && !entry.comment.includes('scenario')) {
                    text += `# ${entry.comment}\n`;
                }
                text += `${entry.key}=${entry.value}\n`;
            });
        });

        return text;
    }

    envTextToConfig(text) {
        const lines = text.split('\n');
        const config = { sections: {} };
        let currentSection = 'Base';
        let currentComment = '';

        lines.forEach(line => {
            line = line.trim();

            // 跳过空行
            if (!line) {
                currentComment = '';
                return;
            }

            // 处理注释
            if (line.startsWith('#')) {
                const comment = line.substring(1).trim();
                if (comment.toLowerCase().includes('scenario') || comment.toLowerCase().includes('agent')) {
                    // 区块注释
                    if (comment.toLowerCase().includes('code')) currentSection = 'CODE';
                    else if (comment.toLowerCase().includes('analysis')) currentSection = 'ANALYSIS';
                    else if (comment.toLowerCase().includes('eval')) currentSection = 'EVAL';
                    else if (comment.toLowerCase().includes('requirements')) currentSection = 'REQUIREMENTS';
                    else if (comment.toLowerCase().includes('build')) currentSection = 'BUILD';
                } else {
                    currentComment = comment;
                }
                return;
            }

            // 解析键值对
            const equalIndex = line.indexOf('=');
            if (equalIndex === -1) return;

            const key = line.substring(0, equalIndex).trim();
            const value = line.substring(equalIndex + 1).trim();

            // 推断区块
            if (currentSection === 'Base') {
                if (key.startsWith('CODE_')) currentSection = 'CODE';
                else if (key.startsWith('ANALYSIS_')) currentSection = 'ANALYSIS';
                else if (key.startsWith('EVAL_')) currentSection = 'EVAL';
                else if (key.startsWith('REQUIREMENTS_')) currentSection = 'REQUIREMENTS';
                else if (key.startsWith('BUILD_')) currentSection = 'BUILD';
            }

            if (!config.sections[currentSection]) {
                config.sections[currentSection] = [];
            }

            config.sections[currentSection].push({
                key,
                value,
                comment: currentComment,
                section: currentSection,
            });

            currentComment = '';
        });

        return config;
    }

    async saveConfig() {
        if (this.currentFile !== 'env') {
            return; // 只能保存 .env 文件
        }

        const successEl = $('#config-success');
        const errorEl = $('#config-error');
        const saveBtn = $('#save-config-btn');
        const textarea = $('#config-textarea');

        // 隐藏之前的消息
        successEl.style.display = 'none';
        errorEl.style.display = 'none';

        // 禁用保存按钮
        saveBtn.disabled = true;
        saveBtn.innerHTML = '<span class="btn-icon">⏳</span> 保存中...';

        try {
            // 将文本转换为配置对象
            const config = this.envTextToConfig(textarea.value);

            // 使用 WailsAPI 保存配置
            const result = await WailsAPI.saveEnvConfig(config);
            
            // 更新原始内容
            this.envContent = textarea.value;
            this.originalEnvContent = this.envContent;

            // 显示成功消息
            successEl.textContent = '配置保存成功！';
            successEl.style.display = 'block';

            // 3秒后隐藏成功消息
            setTimeout(() => {
                successEl.style.display = 'none';
            }, 3000);
        } catch (error) {
            console.error('保存配置失败:', error);
            let errorMsg = error.message || '保存配置失败';
            errorEl.textContent = `保存配置失败: ${errorMsg}`;
            errorEl.style.display = 'block';
        } finally {
            // 恢复保存按钮
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<span class="btn-icon">💾</span> 保存配置';
        }
    }

    resetConfig() {
        if (this.currentFile !== 'env') {
            return; // 只能重置 .env 文件
        }

        const textarea = $('#config-textarea');
        
        if (textarea.value === this.originalEnvContent) {
            return;
        }

        if (!confirm('确定要重置所有更改吗？')) {
            return;
        }

        // 恢复原始内容
        this.envContent = this.originalEnvContent;
        textarea.value = this.envContent;

        // 显示提示
        const successEl = $('#config-success');
        successEl.textContent = '已重置为原始配置';
        successEl.style.display = 'block';
        setTimeout(() => {
            successEl.style.display = 'none';
        }, 2000);
    }
}

// 创建配置页面控制器实例
let configPageController = null;

// 修改 loadPageData 函数以支持配置页面
const originalLoadPageData = loadPageData;
loadPageData = async function(pageName) {
    if (pageName === 'config') {
        if (!configPageController) {
            configPageController = new ConfigPageController();
        }
        await configPageController.loadConfig();
    } else {
        await originalLoadPageData(pageName);
    }
};

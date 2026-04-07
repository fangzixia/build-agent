// API Adapter Layer - 优雅的自动适配方案
// 统一 Desktop Mode 和 Server Mode 的 API 调用接口
// 使用 Proxy 自动路由，无需为每个方法单独编写适配代码

// 检测运行模式
const isDesktopMode = typeof window.go !== 'undefined' && window.go.wails && window.go.wails.Bridge;

// API 端点映射配置
const API_ENDPOINTS = {
    // 配置相关
    getConfig: { method: 'GET', path: '/v1/config' },
    getEnvConfig: { method: 'GET', path: '/v1/config/env' },
    saveEnvConfig: { method: 'POST', path: '/v1/config/env' },
    
    // 文件操作
    readFile: { method: 'GET', path: '/v1/files/read', queryParam: 'path' },
    saveFile: { method: 'POST', path: '/v1/files/save' },
    listFiles: { method: 'GET', path: '/v1/files/list', queryParam: 'path' },
    
    // 需求和评测
    getRequirements: { method: 'GET', path: '/v1/requirements' },
    getEvaluations: { method: 'GET', path: '/v1/evaluations' },
    
    // 任务执行
    runTask: { method: 'POST', path: '/v1/{agent}/run' },
};

/**
 * 创建统一的 API 代理
 */
function createWailsAPI() {
    // Desktop Mode: 直接调用 Go Bridge
    if (isDesktopMode) {
        return new Proxy({}, {
            get(target, methodName) {
                // 特殊处理流式任务执行
                if (methodName === 'runTaskStreaming') {
                    return async function(agentName, task, filePath = '', onLog, onDone, onError) {
                        const eventName = `task:progress:${Date.now()}`;
                        
                        // 注册事件监听器
                        if (onLog) {
                            window.runtime.EventsOn(eventName, (log) => {
                                onLog({ type: log.type || 'log', ...log });
                            });
                        }

                        try {
                            const result = await window.go.wails.Bridge.RunTaskWithProgress(agentName, task, filePath, eventName);
                            window.runtime.EventsOff(eventName);
                            if (onDone) onDone(result);
                        } catch (error) {
                            window.runtime.EventsOff(eventName);
                            if (onError) {
                                onError({ error: error.message || '任务执行失败' });
                            } else {
                                throw error;
                            }
                        }
                    };
                }

                // 其他方法：自动调用 Go Bridge 对应的方法
                return async function(...args) {
                    try {
                        // 将方法名转换为 PascalCase（Go 方法命名规范）
                        const goMethodName = methodName.charAt(0).toUpperCase() + methodName.slice(1);
                        
                        // 调用 Go Bridge 方法
                        const result = await window.go.wails.Bridge[goMethodName](...args);
                        
                        // 某些方法需要包装返回值
                        if (methodName === 'listFiles') {
                            return { files: result };
                        } else if (methodName === 'readFile') {
                            return { path: args[0], content: result };
                        } else if (methodName === 'saveFile') {
                            return { success: true, message: '保存成功' };
                        } else if (methodName === 'getRequirements') {
                            return { requirements: result };
                        } else if (methodName === 'getEvaluations') {
                            return { evaluations: result };
                        }
                        
                        return result;
                    } catch (error) {
                        throw new Error(error.message || `${methodName} 失败`);
                    }
                };
            }
        });
    }
    
    // Server Mode: 调用 HTTP API
    return new Proxy({}, {
        get(target, methodName) {
            // 特殊处理流式任务执行
            if (methodName === 'runTaskStreaming') {
                return async function(agentName, task, filePath = '', onLog, onDone, onError) {
                    try {
                        const response = await fetch(`${window.location.origin}/v1/${agentName}/run`, {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json',
                                'Accept': 'text/event-stream',
                            },
                            body: JSON.stringify({ task, filePath })
                        });

                        if (!response.ok) {
                            const error = await response.json();
                            throw new Error(error.error || '任务执行失败');
                        }

                        const reader = response.body.getReader();
                        const decoder = new TextDecoder();
                        let buffer = '';

                        while (true) {
                            const { done, value } = await reader.read();
                            if (done) break;

                            buffer += decoder.decode(value, { stream: true });
                            const lines = buffer.split('\n\n');
                            buffer = lines.pop() || '';

                            for (const line of lines) {
                                if (!line.trim()) continue;

                                const eventMatch = line.match(/^event: (.+)$/m);
                                const dataMatch = line.match(/^data: (.+)$/m);

                                if (eventMatch && dataMatch) {
                                    const event = eventMatch[1];
                                    const data = JSON.parse(dataMatch[1]);

                                    if (event === 'start' || event === 'log') {
                                        if (onLog) onLog({ type: event, ...data });
                                    } else if (event === 'done') {
                                        if (onDone) onDone(data);
                                    } else if (event === 'error') {
                                        if (onError) onError(data);
                                    }
                                }
                            }
                        }
                    } catch (error) {
                        if (onError) {
                            onError({ error: error.message });
                        } else {
                            throw error;
                        }
                    }
                };
            }

            // 其他方法：自动调用 HTTP API
            return async function(...args) {
                const config = API_ENDPOINTS[methodName];
                
                if (!config) {
                    throw new Error(`未知的 API 方法: ${methodName}`);
                }

                let url = `${window.location.origin}${config.path}`;
                let options = {
                    method: config.method,
                    headers: { 'Content-Type': 'application/json' },
                };

                // 处理 URL 参数
                if (config.path.includes('{agent}')) {
                    url = url.replace('{agent}', args[0]);
                    args = args.slice(1);
                }

                // 处理查询参数
                if (config.queryParam && args.length > 0) {
                    url += `?${config.queryParam}=${encodeURIComponent(args[0])}`;
                }

                // 处理请求体
                if (config.method === 'POST' && args.length > 0) {
                    if (methodName === 'saveFile') {
                        options.body = JSON.stringify({ path: args[0], content: args[1] });
                    } else if (methodName === 'runTask') {
                        options.body = JSON.stringify({ task: args[0], filePath: args[1] || '' });
                    } else {
                        options.body = JSON.stringify(args[0]);
                    }
                }

                const response = await fetch(url, options);

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.error || `${methodName} 失败`);
                }

                return await response.json();
            };
        }
    });
}

// 创建 WailsAPI 实例
const WailsAPI = createWailsAPI();

// 添加辅助方法
WailsAPI.getMode = () => isDesktopMode ? 'desktop' : 'server';

// 全局暴露
window.WailsAPI = WailsAPI;

// 在控制台输出当前模式
console.log(`[API Adapter] Running in ${WailsAPI.getMode()} mode`);

// 导出（用于模块化）
if (typeof module !== 'undefined' && module.exports) {
    module.exports = WailsAPI;
}

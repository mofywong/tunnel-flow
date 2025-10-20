import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import router from '@/router'

// 创建axios实例
const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    // 添加认证token
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    
    // 添加请求ID用于追踪
    config.headers['X-Request-ID'] = generateRequestId()
    
    // 显示加载状态（可选）
    if (config.showLoading !== false) {
      // 这里可以显示全局loading
    }
    
    return config
  },
  (error) => {
    console.error('Request interceptor error:', error)
    return Promise.reject(error)
  }
)

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    // 隐藏加载状态
    // hideGlobalLoading()
    
    // 检查业务状态码
    if (response.data && response.data.code !== undefined) {
      if (response.data.code !== 0) {
        const message = response.data.message || '请求失败'
        ElMessage.error(message)
        return Promise.reject(new Error(message))
      }
      // 返回实际数据
      return {
        ...response,
        data: response.data.data || response.data
      }
    }
    
    return response
  },
  (error) => {
    // 隐藏加载状态
    // hideGlobalLoading()
    
    console.error('Response interceptor error:', error)
    
    // 处理网络错误
    if (!error.response) {
      ElMessage.error('网络连接失败，请检查网络设置')
      return Promise.reject(error)
    }
    
    const { status, data } = error.response
    
    // 处理不同的HTTP状态码
    switch (status) {
      case 400:
        ElMessage.error(data?.message || '请求参数错误')
        break
      case 401:
        ElMessage.error('认证失败，请重新登录')
        // 清除token并跳转到登录页
        localStorage.removeItem('token')
        router.push('/login')
        break
      case 403:
        ElMessage.error('权限不足，无法访问该资源')
        break
      case 404:
        ElMessage.error('请求的资源不存在')
        break
      case 409:
        ElMessage.error(data?.message || '资源冲突')
        break
      case 422:
        // 表单验证错误
        if (data?.errors) {
          const errorMessages = Object.values(data.errors).flat()
          ElMessage.error(errorMessages.join(', '))
        } else {
          ElMessage.error(data?.message || '数据验证失败')
        }
        break
      case 429:
        ElMessage.error('请求过于频繁，请稍后再试')
        break
      case 500:
        ElMessage.error('服务器内部错误，请稍后再试')
        break
      case 502:
      case 503:
      case 504:
        ElMessage.error('服务暂时不可用，请稍后再试')
        break
      default:
        ElMessage.error(data?.message || `请求失败 (${status})`)
    }
    
    return Promise.reject(error)
  }
)

// 生成请求ID
function generateRequestId() {
  return `req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

// 封装常用的请求方法
const request = {
  get(url, config = {}) {
    return api.get(url, config)
  },
  
  post(url, data = {}, config = {}) {
    return api.post(url, data, config)
  },
  
  put(url, data = {}, config = {}) {
    return api.put(url, data, config)
  },
  
  patch(url, data = {}, config = {}) {
    return api.patch(url, data, config)
  },
  
  delete(url, config = {}) {
    return api.delete(url, config)
  },
  
  // 上传文件
  upload(url, file, config = {}) {
    const formData = new FormData()
    formData.append('file', file)
    
    return api.post(url, formData, {
      ...config,
      headers: {
        'Content-Type': 'multipart/form-data',
        ...config.headers
      }
    })
  },
  
  // 下载文件
  download(url, filename, config = {}) {
    return api.get(url, {
      ...config,
      responseType: 'blob'
    }).then(response => {
      const blob = new Blob([response.data])
      const downloadUrl = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = downloadUrl
      link.download = filename || 'download'
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      window.URL.revokeObjectURL(downloadUrl)
    })
  }
}

// 请求取消功能
export const createCancelToken = () => {
  return axios.CancelToken.source()
}

// 检查是否为取消请求的错误
export const isCancel = (error) => {
  return axios.isCancel(error)
}

// 批量请求
export const all = (requests) => {
  return Promise.all(requests)
}

// 并发控制的批量请求
export const allSettled = (requests) => {
  return Promise.allSettled(requests)
}

// 导出api实例和request对象
export { api }
export default request
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/utils/api'

export const useSystemStore = defineStore('system', () => {
  // 状态
  const systemStatus = ref({
    server: {
      status: 'unknown',
      timestamp: null,
      version: null
    },
    database: {
      status: 'unknown'
    }
  })
  
  const loading = ref(false)
  const error = ref(null)
  const theme = ref(localStorage.getItem('theme') || 'system')
  const sidebarCollapsed = ref(localStorage.getItem('sidebarCollapsed') === 'true')
  
  // 计算属性
  const isSystemHealthy = computed(() => {
    return systemStatus.value?.server?.status === 'running' && 
           systemStatus.value?.database?.status === 'connected'
  })
  
  const connectedClientsCount = computed(() => {
    // 这个值现在通过客户端store获取
    return 0
  })
  
  // 获取实际应用的主题（解析system主题）
  const actualTheme = computed(() => {
    if (theme.value === 'system') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    }
    return theme.value
  })
  
  // 方法
  const initializeSystem = async () => {
    await fetchSystemStatus()
  }
  
  const fetchSystemStatus = async () => {
    try {
      loading.value = true
      error.value = null
      
      const response = await api.get('/status')
      systemStatus.value = response.data
    } catch (err) {
      error.value = err.message || '获取系统状态失败'
      console.error('Failed to fetch system status:', err)
    } finally {
      loading.value = false
    }
  }
  

  
  const applyTheme = (themeToApply = actualTheme.value) => {
    if (themeToApply === 'dark') {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
  }
  
  const toggleTheme = () => {
    // 循环切换：light -> dark -> system -> light
    if (theme.value === 'light') {
      theme.value = 'dark'
    } else if (theme.value === 'dark') {
      theme.value = 'system'
    } else {
      theme.value = 'light'
    }
    
    localStorage.setItem('theme', theme.value)
    applyTheme()
  }
  
  const setTheme = (newTheme) => {
    if (['light', 'dark', 'system'].includes(newTheme)) {
      theme.value = newTheme
      localStorage.setItem('theme', theme.value)
      applyTheme()
    }
  }
  
  const toggleSidebar = () => {
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem('sidebarCollapsed', sidebarCollapsed.value.toString())
  }
  
  const clearError = () => {
    error.value = null
  }
  
  // 监听系统主题变化
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  const handleSystemThemeChange = () => {
    if (theme.value === 'system') {
      applyTheme()
    }
  }
  mediaQuery.addEventListener('change', handleSystemThemeChange)
  
  // 初始化主题
  applyTheme()
  
  return {
    // 状态
    systemStatus,
    loading,
    error,
    theme,
    sidebarCollapsed,
    
    // 计算属性
    isSystemHealthy,
    connectedClientsCount,
    actualTheme,
    
    // 方法
    initializeSystem,
    fetchSystemStatus,
    toggleTheme,
    setTheme,
    applyTheme,
    toggleSidebar,
    clearError
  }
})
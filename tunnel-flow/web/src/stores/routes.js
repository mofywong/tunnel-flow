import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/utils/api'

export const useRoutesStore = defineStore('routes', () => {
  // 状态
  const routes = ref([])
  const loading = ref(false)
  const error = ref(null)
  const pagination = ref({
    page: 1,
    pageSize: 20,
    total: 0
  })
  
  // 计算属性
  const activeRoutes = computed(() => {
    return routes.value.filter(route => route.status === 'active')
  })
  
  const inactiveRoutes = computed(() => {
    return routes.value.filter(route => route.status === 'inactive')
  })
  
  const routesStats = computed(() => {
    return {
      total: routes.value.length,
      active: activeRoutes.value.length,
      inactive: inactiveRoutes.value.length
    }
  })
  
  const routesByClient = computed(() => {
    const grouped = {}
    routes.value.forEach(route => {
      if (!grouped[route.client_id]) {
        grouped[route.client_id] = []
      }
      grouped[route.client_id].push(route)
    })
    return grouped
  })
  
  // 方法
  const fetchRoutes = async (params = {}) => {
    try {
      loading.value = true
      error.value = null
      
      const queryParams = {
        page: pagination.value.page,
        page_size: pagination.value.pageSize,
        ...params
      }
      
      const response = await api.get('/routes', { params: queryParams })
      
      routes.value = response.data?.routes || []
      pagination.value.total = response.data?.total || 0
      
    } catch (err) {
      error.value = err.message || '获取路由列表失败'
      console.error('Failed to fetch routes:', err)
    } finally {
      loading.value = false
    }
  }
  
  const getRouteById = async (routeId) => {
    try {
      const response = await api.get(`/routes/${routeId}`)
      return response.data
    } catch (err) {
      error.value = err.message || '获取路由详情失败'
      console.error('Failed to get route:', err)
      throw err
    }
  }
  
  const createRoute = async (routeData) => {
    try {
      const response = await api.post('/routes', routeData)
      
      // 添加到本地状态
      routes.value.unshift(response.data)
      pagination.value.total++
      
      return response.data
    } catch (err) {
      error.value = err.message || '创建路由失败'
      console.error('Failed to create route:', err)
      throw err
    }
  }
  
  const updateRoute = async (routeId, data) => {
    try {
      const response = await api.put(`/routes/${routeId}`, data)
      
      // 更新本地状态
      const index = routes.value.findIndex(r => r.id === routeId)
      if (index !== -1) {
        routes.value[index] = { ...routes.value[index], ...response.data }
      }
      
      return response.data
    } catch (err) {
      error.value = err.message || '更新路由失败'
      console.error('Failed to update route:', err)
      throw err
    }
  }
  
  const deleteRoute = async (routeId) => {
    try {
      await api.delete(`/routes/${routeId}`)
      
      // 从本地状态中移除
      const index = routes.value.findIndex(r => r.id === routeId)
      if (index !== -1) {
        routes.value.splice(index, 1)
        pagination.value.total--
      }
      
    } catch (err) {
      error.value = err.message || '删除路由失败'
      console.error('Failed to delete route:', err)
      throw err
    }
  }
  
  const activateRoute = async (routeId) => {
    try {
      await api.post(`/routes/${routeId}/activate`)
      
      // 更新本地状态
      const index = routes.value.findIndex(r => r.id === routeId)
      if (index !== -1) {
        routes.value[index].status = 'active'
        routes.value[index].updated_at = new Date().toISOString()
      }
      
    } catch (err) {
      error.value = err.message || '激活路由失败'
      console.error('Failed to activate route:', err)
      throw err
    }
  }
  
  const deactivateRoute = async (routeId) => {
    try {
      await api.post(`/routes/${routeId}/deactivate`)
      
      // 更新本地状态
      const index = routes.value.findIndex(r => r.id === routeId)
      if (index !== -1) {
        routes.value[index].status = 'inactive'
        routes.value[index].updated_at = new Date().toISOString()
      }
      
    } catch (err) {
      error.value = err.message || '停用路由失败'
      console.error('Failed to deactivate route:', err)
      throw err
    }
  }
  

  
  const getRouteStats = async (routeId, timeRange = '24h') => {
    try {
      const response = await api.get(`/routes/${routeId}/stats`, {
        params: { time_range: timeRange }
      })
      return response.data
    } catch (err) {
      error.value = err.message || '获取路由统计失败'
      console.error('Failed to get route stats:', err)
      throw err
    }
  }
  
  const refreshRoutes = () => {
    return fetchRoutes()
  }


  
  const setPage = (page) => {
    pagination.value.page = page
    return fetchRoutes()
  }
  
  const setPageSize = (pageSize) => {
    pagination.value.pageSize = pageSize
    pagination.value.page = 1
    return fetchRoutes()
  }
  
  const clearError = () => {
    error.value = null
  }
  

  
  return {
    // 状态
    routes,
    loading,
    error,
    pagination,
    
    // 计算属性
    activeRoutes,
    inactiveRoutes,
    routesStats,
    routesByClient,
    
    // 方法
    fetchRoutes,
    getRouteById,
    createRoute,
    updateRoute,
    deleteRoute,
    activateRoute,
    deactivateRoute,
    getRouteStats,
    refreshRoutes,
    setPage,
    setPageSize,
    clearError,
    

  }
})
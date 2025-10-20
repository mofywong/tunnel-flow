import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '@/utils/api'

export const useClientsStore = defineStore('clients', () => {
  // 状态
  const clients = ref([]) // 当前显示的客户端列表（可能是筛选后的）
  const allClients = ref([]) // 全部客户端数据（用于统计计算）
  const loading = ref(false)
  const error = ref(null)
  const pagination = ref({
    page: 1,
    pageSize: 20,
    total: 0
  })
  
  // 计算属性 - 基于全部客户端数据计算统计
  const onlineClients = computed(() => {
    return allClients.value.filter(client => client.status === 'online')
  })
  
  const offlineClients = computed(() => {
    return allClients.value.filter(client => client.status === 'offline')
  })
  
  const disabledClients = computed(() => {
    return allClients.value.filter(client => client.enabled === 0)
  })
  
  const enabledClients = computed(() => {
    return allClients.value.filter(client => client.enabled === 1)
  })
  
  const clientsStats = computed(() => {
    return {
      total: allClients.value.length,
      online: onlineClients.value.length,
      offline: offlineClients.value.length,
      disabled: disabledClients.value.length,
      enabled: enabledClients.value.length
    }
  })
  
  // 方法
  const fetchClients = async (params = {}) => {
    try {
      loading.value = true
      error.value = null
      
      const queryParams = {
        page: pagination.value.page,
        page_size: pagination.value.pageSize,
        ...params
      }
      
      const response = await api.get('/clients', { params: queryParams })
      
      // 后端直接返回clients数组，不是包装在对象中
      if (Array.isArray(response.data)) {
        clients.value = response.data
        // 如果没有筛选参数，说明是获取全部数据，更新allClients
        if (!params.status) {
          allClients.value = response.data
        }
        pagination.value.total = response.data.length
      } else if (response.data && response.data.clients) {
        // 兼容包装格式
        clients.value = response.data.clients
        // 如果没有筛选参数，说明是获取全部数据，更新allClients
        if (!params.status) {
          allClients.value = response.data.clients
        }
        pagination.value.total = response.data.total || response.data.clients.length
      } else {
        // 处理空数据情况
        clients.value = []
        pagination.value.total = 0
      }
      
    } catch (err) {
      error.value = err.message || '获取客户端列表失败'
      console.error('Failed to fetch clients:', err)
    } finally {
      loading.value = false
    }
  }
  
  // 获取全部客户端数据（用于统计计算）
  const fetchAllClients = async () => {
    try {
      const response = await api.get('/clients')
      if (Array.isArray(response.data)) {
        allClients.value = response.data
      } else if (response.data && response.data.clients) {
        allClients.value = response.data.clients
      } else {
        allClients.value = []
      }
    } catch (err) {
      console.error('Failed to fetch all clients for stats:', err)
    }
  }

  // 根据状态筛选客户端
  const fetchClientsByStatus = async (status = '') => {
    // 首先确保有全部客户端数据用于统计
    if (allClients.value.length === 0) {
      await fetchAllClients()
    }
    
    const params = {}
    if (status) {
      params.status = status
    }
    await fetchClients(params)
  }
  
  const getClientById = async (clientId) => {
    try {
      const response = await api.get(`/clients/${clientId}`)
      return response.data
    } catch (err) {
      error.value = err.message || '获取客户端详情失败'
      console.error('Failed to get client:', err)
      throw err
    }
  }
  
  const createClient = async (data) => {
    try {
      const response = await api.post('/clients', data)
      
      // 添加到本地状态
      clients.value.unshift(response.data)
      pagination.value.total++
      
      return response.data
    } catch (err) {
      error.value = err.message || '创建客户端失败'
      console.error('Failed to create client:', err)
      throw err
    }
  }

  const updateClient = async (clientId, data) => {
    try {
      const response = await api.put(`/clients/${clientId}`, data)
      
      // 更新本地状态
      const index = clients.value.findIndex(c => c.client_id === clientId)
      if (index !== -1) {
        clients.value[index] = { ...clients.value[index], ...response.data }
      }
      
      return response.data
    } catch (err) {
      error.value = err.message || '更新客户端失败'
      console.error('Failed to update client:', err)
      throw err
    }
  }
  
  const deleteClient = async (clientId) => {
    try {
      await api.delete(`/clients/${clientId}`)
      
      // 从本地状态中移除
      const index = clients.value.findIndex(c => c.client_id === clientId)
      if (index !== -1) {
        clients.value.splice(index, 1)
        pagination.value.total--
      }
      
    } catch (err) {
      error.value = err.message || '删除客户端失败'
      console.error('Failed to delete client:', err)
      throw err
    }
  }
  


  const updateClientStatus = async (clientId, status) => {
    try {
      const response = await api.put(`/clients/${clientId}/status`, { status })
      
      // 更新本地状态
      const index = clients.value.findIndex(c => c.client_id === clientId)
      if (index !== -1) {
        clients.value[index].status = status
        clients.value[index].updated_at = new Date().toISOString()
      }
      
      return response.data
    } catch (err) {
      error.value = err.message || '更新客户端状态失败'
      console.error('Failed to update client status:', err)
      throw err
    }
  }
  
  const updateClientEnabled = async (clientId, enabled) => {
    try {
      const response = await api.put(`/clients/${clientId}/enabled`, { enabled })
      
      // 更新本地状态
      const index = clients.value.findIndex(c => c.client_id === clientId)
      if (index !== -1) {
        clients.value[index].enabled = enabled
        clients.value[index].updated_at = new Date().toISOString()
      }
      
      return response.data
    } catch (err) {
      error.value = err.message || '更新客户端启用状态失败'
      console.error('Failed to update client enabled status:', err)
      throw err
    }
  }

  const sendMessageToClient = async (clientId, message) => {
    try {
      const response = await api.post(`/clients/${clientId}/message`, {
        type: 'control',
        payload: message
      })
      return response.data
    } catch (err) {
      error.value = err.message || '发送消息失败'
      console.error('Failed to send message to client:', err)
      throw err
    }
  }
  
  const refreshClients = () => {
    return fetchClients()
  }


  
  const setPage = (page) => {
    pagination.value.page = page
    return fetchClients()
  }
  
  const setPageSize = (pageSize) => {
    pagination.value.pageSize = pageSize
    pagination.value.page = 1
    return fetchClients()
  }
  
  const clearError = () => {
    error.value = null
  }
  

  
  return {
    // 状态
    clients,
    allClients,
    loading,
    error,
    pagination,
    
    // 计算属性
    onlineClients,
    offlineClients,
    disabledClients,
    enabledClients,
    clientsStats,
    
    // 方法
    fetchClients,
    fetchAllClients,
    fetchClientsByStatus,
    getClientById,
    createClient,
    updateClient,
    deleteClient,
    updateClientStatus,
    updateClientEnabled,
    sendMessageToClient,
    refreshClients,
    setPage,
    setPageSize,
    clearError,
    

  }
})
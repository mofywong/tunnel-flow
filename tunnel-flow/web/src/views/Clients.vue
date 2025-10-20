<template>
  <div class="clients-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">客户端管理</h1>
        <p class="page-description">管理和监控所有连接的客户端</p>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="showAddDialog">
          <el-icon><Plus /></el-icon>
          新增客户端
        </el-button>
        <el-button type="primary" @click="refreshData" :loading="clientsStore.loading">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>
    
    <!-- 统计卡片 -->
    <div class="stats-container">
          <div class="stat-card total-card" @click="handleStatCardClick('')">
            <div class="stat-icon total">
              <el-icon><User /></el-icon>
            </div>
            <div class="stat-content">
              <div class="stat-value">{{ clientsStore.clientsStats.total }}</div>
              <div class="stat-label">总客户端</div>
            </div>
          </div>
          
          <div class="stat-card online-card" @click="handleStatCardClick('online')">
            <div class="stat-icon online">
              <el-icon><Monitor /></el-icon>
            </div>
            <div class="stat-content">
              <div class="stat-value">{{ clientsStore.clientsStats.online }}</div>
              <div class="stat-label">在线客户端</div>
            </div>
          </div>
          
          <div class="stat-card offline-card" @click="handleStatCardClick('offline')">
            <div class="stat-icon offline">
              <el-icon><Warning /></el-icon>
            </div>
            <div class="stat-content">
              <div class="stat-value">{{ clientsStore.clientsStats.offline }}</div>
              <div class="stat-label">离线客户端</div>
            </div>
          </div>
          
          <div class="stat-card enabled-card" @click="handleStatCardClick('enabled')">
            <div class="stat-icon enabled">
              <el-icon><Check /></el-icon>
            </div>
            <div class="stat-content">
              <div class="stat-value">{{ clientsStore.clientsStats.enabled }}</div>
              <div class="stat-label">已启用客户端</div>
            </div>
          </div>
          
          <div class="stat-card disabled-card" @click="handleStatCardClick('disabled')">
            <div class="stat-icon disabled">
              <el-icon><Close /></el-icon>
            </div>
            <div class="stat-content">
              <div class="stat-value">{{ clientsStore.clientsStats.disabled }}</div>
              <div class="stat-label">已停用客户端</div>
            </div>
          </div>
        </div>
    
    <!-- 搜索和筛选 -->
    <div class="search-section">
      <div class="search-left">
        <el-input
          v-model="searchQuery"
          placeholder="搜索客户端名称"
          style="width: 300px"
          clearable
          @input="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        
        <el-select
          v-model="statusFilter"
          placeholder="状态筛选"
          style="width: 120px"
          clearable
          @change="handleFilter"
        >
          <el-option label="在线" value="online" />
          <el-option label="离线" value="offline" />
          <el-option label="启用" value="enabled" />
          <el-option label="停用" value="disabled" />
        </el-select>
      </div>
      
      <div class="search-right">
        <!-- 批量操作按钮已移除 -->
      </div>
    </div>
    
    <!-- 客户端表格 -->
    <div class="table-container">
      <el-table
        :data="filteredClients"
        v-loading="clientsStore.loading"
        stripe
        style="width: 100%"
      >
        <el-table-column label="客户端名称" min-width="200">
          <template #default="{ row }">
            <div class="client-name">
              <el-icon class="client-icon"><Monitor /></el-icon>
              <div>
                <div style="font-weight: 500;">{{ row.name || '未命名' }}</div>
                <div v-if="row.description" class="description-line" :title="row.description">
                  {{ row.description }}
                </div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="客户端编码" prop="client_id" min-width="220" show-overflow-tooltip />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag 
              :type="row.status === 'online' ? 'success' : row.status === 'disabled' ? 'info' : 'danger'" 
              size="small"
            >
              {{ row.status === 'online' ? '在线' : row.status === 'disabled' ? '已禁用' : '离线' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="本地IP" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.local_ips && row.local_ips.length > 0 ? row.local_ips.join(', ') : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">
            {{ formatDateTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="最新心跳时间" width="160">
          <template #default="{ row }">
            {{ formatDateTime(row.last_seen_ts) || '-' }}
          </template>
        </el-table-column>
        
        <el-table-column label="启用状态" width="120" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled === 1"
              @change="(value) => handleSwitchChange(row, value)"
              active-text="启用"
              inactive-text="禁用"
              inline-prompt
              style="--el-switch-on-color: #13ce66; --el-switch-off-color: #ff4949"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button size="small" @click="viewClient(row)">
                <el-icon><View /></el-icon>
                详情
              </el-button>
              <el-button size="small" type="primary" @click="editClient(row)">
                <el-icon><Edit /></el-icon>
                编辑
              </el-button>
              <el-button size="small" type="danger" @click="deleteClient(row)">
                <el-icon><Delete /></el-icon>
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      
      <!-- 分页 -->
      <div class="pagination-container">
        <el-pagination
          v-model:current-page="clientsStore.pagination.page"
          v-model:page-size="clientsStore.pagination.pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="clientsStore.pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </div>
    
    <!-- 客户端详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      title="客户端详情"
      width="800px"
      :before-close="handleDetailClose"
    >
      <div v-if="selectedClient" class="client-detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="客户端ID">
            <el-tag>{{ selectedClient.client_id }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="客户端名称">
            {{ selectedClient.name || '未设置' }}
          </el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="selectedClient.status === 'online' ? 'success' : selectedClient.status === 'disabled' ? 'info' : 'danger'">
              {{ selectedClient.status === 'online' ? '在线' : selectedClient.status === 'disabled' ? '已禁用' : '离线' }}
            </el-tag>
          </el-descriptions-item>

          <el-descriptions-item label="本地IP地址" span="2">
            <div v-if="selectedClient.local_ips && selectedClient.local_ips.length > 0">
              <el-tag v-for="ip in selectedClient.local_ips" :key="ip" style="margin-right: 8px; margin-bottom: 4px;">
                {{ ip }}
              </el-tag>
            </div>
            <span v-else style="color: #909399;">未获取到本地IP地址</span>
          </el-descriptions-item>
        </el-descriptions>
        
        <!-- 认证令牌单独显示 -->
        <div v-if="selectedClient.has_auth_token || selectedClient.auth_token" class="token-section" style="margin-top: 20px;">
          <h4 style="margin-bottom: 10px;">认证令牌</h4>
          <div v-if="selectedClient.auth_token" class="token-display">
            <el-input
              v-model="selectedClient.auth_token"
              readonly
              type="textarea"
              :rows="2"
              style="margin-bottom: 10px;"
            >
              <template #append>
                <el-button @click="copyToken(selectedClient.auth_token)" :icon="DocumentCopy">
                  复制
                </el-button>
              </template>
            </el-input>
            <el-alert
              title="认证令牌"
              type="info"
              description="请妥善保管此令牌，客户端连接时需要使用。"
              :closable="false"
              show-icon
            />
          </div>
          <el-alert
            v-else
            title="认证令牌已配置"
            type="success"
            description="该客户端已配置认证令牌，出于安全考虑不显示具体内容。如需重新生成，请删除并重新创建客户端。"
            :closable="false"
            show-icon
          />
        </div>
        
        <!-- 其他信息 -->
        <el-descriptions :column="2" border style="margin-top: 20px;">
          <el-descriptions-item label="创建时间">
            {{ formatDateTime(selectedClient.created_at) || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="最新心跳时间">
            {{ formatDateTime(selectedClient.last_seen_ts || selectedClient.last_seen) || '-' }}
          </el-descriptions-item>
        </el-descriptions>
        
        <!-- 客户端路由 -->
        <div class="client-routes" v-if="clientRoutes.length > 0">
          <h3>关联路由</h3>
          <el-table :data="clientRoutes" size="small">
            <el-table-column prop="server_path" label="服务器路径" />
            <el-table-column prop="target_url" label="目标地址" />
            <el-table-column prop="status" label="状态">
              <template #default="{ row }">
                <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
                  {{ row.status === 'active' ? '活跃' : '非活跃' }}
                </el-tag>
              </template>
            </el-table-column>
          </el-table>
        </div>
        

      </div>
    </el-dialog>

    <!-- 新增/编辑客户端对话框 -->
    <el-dialog
      v-model="formDialogVisible"
      :title="isEdit ? '编辑客户端' : '新增客户端'"
      width="500px"
      @close="handleFormClose"
    >
      <el-form
        ref="formRef"
        :model="clientForm"
        :rules="formRules"
        label-width="100px"
      >
        <el-form-item label="客户端名称" prop="name">
          <el-input v-model="clientForm.name" placeholder="请输入客户端名称" />
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input
            v-model="clientForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入客户端描述"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="handleFormClose">取消</el-button>
          <el-button type="primary" @click="handleFormSubmit" :loading="formLoading">
            {{ isEdit ? '更新' : '创建' }}
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 客户端创建成功对话框 -->
    <el-dialog
      v-model="successDialogVisible"
      title="客户端创建成功"
      width="600px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
    >
      <div class="success-content">
        <div class="success-icon">
          <el-icon size="48" color="#67c23a"><Check /></el-icon>
        </div>
        <h3>客户端创建成功！</h3>
        <p>请保存以下认证令牌，并配置到客户端：</p>
        
        <div class="token-section">
          <el-form label-width="120px">
            <el-form-item label="客户端编码:">
              <el-input 
                :value="createdClient.client_id" 
                readonly
                class="readonly-input"
              />
            </el-form-item>
            <el-form-item label="认证令牌:">
              <el-input 
                :value="createdClient.auth_token" 
                readonly
                type="textarea"
                :rows="3"
                class="readonly-input token-input"
              />
            </el-form-item>
          </el-form>
          
          <!-- 复制全部信息按钮 -->
          <div style="text-align: center; margin-top: 20px;">
            <el-button type="primary" @click="copyAllInfo">
              <el-icon><DocumentCopy /></el-icon>
              复制全部信息
            </el-button>
          </div>
        </div>
        
      </div>
      
      <template #footer>
        <div class="dialog-footer">
          <el-button type="primary" @click="handleSuccessClose">
            确定
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Refresh, Monitor, User, Warning, Search, View, Delete, ChatDotRound, Plus, Edit, Switch, Check, DocumentCopy
} from '@element-plus/icons-vue'
import { useClientsStore } from '@/stores/clients'
import { useRoutesStore } from '@/stores/routes'

const clientsStore = useClientsStore()
const routesStore = useRoutesStore()

// 响应式数据
const searchQuery = ref('')
const statusFilter = ref('')
const detailDialogVisible = ref(false)
const selectedClient = ref(null)
const clientRoutes = ref([])

// 表单相关
const formDialogVisible = ref(false)
const isEdit = ref(false)
const formLoading = ref(false)
const formRef = ref(null)
const clientForm = ref({
  id: '',
  name: '',
  description: ''
})

// 成功对话框相关
const successDialogVisible = ref(false)
const createdClient = ref({
  client_id: '',
  auth_token: ''
})

// 表单验证规则
const formRules = {
  name: [
    { required: true, message: '请输入客户端名称', trigger: 'blur' }
  ]
}

// 计算属性
const filteredClients = computed(() => {
  let clients = clientsStore.clients
  
  // 搜索筛选（仅在前端进行搜索筛选）
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    clients = clients.filter(client => 
      (client.name && client.name.toLowerCase().includes(query)) ||
      client.client_id.toLowerCase().includes(query)
    )
  }
  
  return clients
})

// 工具函数
const formatDateTime = (dateString) => {
  if (!dateString) return ''
  
  let date
  // 处理时间戳格式（秒或毫秒）
  if (typeof dateString === 'number') {
    // 检查是否为有效的时间戳（大于1970年1月2日）
    if (dateString <= 86400) return ''
    // 如果是秒级时间戳，转换为毫秒
    date = new Date(dateString < 10000000000 ? dateString * 1000 : dateString)
  } else {
    // 处理字符串格式
    date = new Date(dateString)
  }
  
  if (isNaN(date.getTime())) return ''
  
  // 检查日期是否为1970年1月1日或之前（无效时间戳）
  if (date.getFullYear() <= 1970 && date.getMonth() === 0 && date.getDate() === 1) {
    return ''
  }
  
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')
  
  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

const getClientRouteCount = (clientId) => {
  return routesStore.routesByClient[clientId]?.length || 0
}

// 事件处理
const refreshData = async () => {
  try {
    await Promise.all([
      clientsStore.fetchClients(),
      routesStore.fetchRoutes()
    ])
    ElMessage.success('数据已刷新')
  } catch (error) {
    ElMessage.error('刷新数据失败')
  }
}

const handleSearch = () => {
  // 搜索逻辑已在计算属性中处理
}

const handleFilter = async () => {
  // 调用后端API进行筛选
  await clientsStore.fetchClientsByStatus(statusFilter.value)
}

// 统计卡片点击事件
const handleStatCardClick = async (filterType) => {
  statusFilter.value = filterType
  // 调用后端API进行筛选
  await clientsStore.fetchClientsByStatus(filterType)
}



const handleSizeChange = (size) => {
  clientsStore.setPageSize(size)
}

const handleCurrentChange = (page) => {
  clientsStore.setPage(page)
}

// 客户端操作
const viewClient = async (client) => {
  selectedClient.value = client
  
  // 获取客户端关联的路由
  clientRoutes.value = routesStore.routesByClient[client.client_id] || []
  
  detailDialogVisible.value = true
}



// 复制令牌到剪贴板
const copyToken = async (token) => {
  try {
    await navigator.clipboard.writeText(token)
    ElMessage.success('认证令牌已复制到剪贴板')
  } catch (error) {
    // 如果 clipboard API 不可用，使用传统方法
    const textArea = document.createElement('textarea')
    textArea.value = token
    document.body.appendChild(textArea)
    textArea.select()
    document.execCommand('copy')
    document.body.removeChild(textArea)
    ElMessage.success('认证令牌已复制到剪贴板')
  }
}

const deleteClient = async (client) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除客户端 "${client.name || client.client_id}" 吗？此操作不可恢复。`,
      '确认删除',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    await clientsStore.deleteClient(client.client_id)
    ElMessage.success('客户端已删除')
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除客户端失败')
    }
  }
}




const handleSwitchChange = async (client, value) => {
  const action = value ? '启用' : '禁用'
  
  try {
    await ElMessageBox.confirm(
      `确定要${action}客户端 "${client.name || client.client_id}" 吗？`,
      `${action}客户端`,
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    await clientsStore.updateClientEnabled(client.client_id, value)
    
    ElMessage.success(`客户端${action}成功`)
    await refreshData()
  } catch (error) {
    if (error !== 'cancel') {
      // 显示详细的错误信息
      const errorMessage = error.response?.data?.message || error.message || `客户端${action}失败`
      ElMessage.error(errorMessage)
      console.error(`Failed to ${action} client:`, error)
    }
  }
}

const toggleClientStatus = async (client) => {
  const action = client.status === 'disabled' ? '启用' : '禁用'
  
  try {
    await ElMessageBox.confirm(
      `确定要${action}客户端 "${client.name || client.client_id}" 吗？`,
      `${action}客户端`,
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    const newStatus = client.status === 'disabled' ? 'online' : 'disabled'
    await clientsStore.updateClientStatus(client.client_id, newStatus)
    
    ElMessage.success(`客户端${action}成功`)
    await loadClients()
  } catch (error) {
    if (error !== 'cancel') {
      // 显示详细的错误信息
      const errorMessage = error.response?.data?.message || error.message || `客户端${action}失败`
      ElMessage.error(errorMessage)
      console.error(`Failed to ${action} client:`, error)
    }
  }
}

const handleDetailClose = () => {
  detailDialogVisible.value = false
  selectedClient.value = null
  clientRoutes.value = []
}

// 表单操作
const showAddDialog = () => {
  isEdit.value = false
  clientForm.value = {
    id: '',
    name: '',
    description: ''
  }
  formDialogVisible.value = true
}

const editClient = (client) => {
  isEdit.value = true
  clientForm.value = {
    id: client.client_id,
    name: client.name || '',
    description: client.description || ''
  }
  formDialogVisible.value = true
}

const handleFormClose = () => {
  formDialogVisible.value = false
  formRef.value?.resetFields()
}

const handleSuccessClose = () => {
  successDialogVisible.value = false
  createdClient.value = {
    client_id: '',
    auth_token: ''
  }
}

const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch (error) {
    // 如果现代API不可用，使用传统方法
    const textArea = document.createElement('textarea')
    textArea.value = text
    document.body.appendChild(textArea)
    textArea.select()
    try {
      document.execCommand('copy')
      ElMessage.success(`${label}已复制到剪贴板`)
    } catch (err) {
      ElMessage.error('复制失败，请手动复制')
    }
    document.body.removeChild(textArea)
  }
}

// 复制全部信息
const copyAllInfo = async () => {
  const allInfo = `客户端编码: ${createdClient.value.client_id}
认证令牌: ${createdClient.value.auth_token}`
  
  try {
    await navigator.clipboard.writeText(allInfo)
    ElMessage.success('全部信息已复制到剪贴板')
  } catch (error) {
    // 如果现代API不可用，使用传统方法
    const textArea = document.createElement('textarea')
    textArea.value = allInfo
    document.body.appendChild(textArea)
    textArea.select()
    try {
      document.execCommand('copy')
      ElMessage.success('全部信息已复制到剪贴板')
    } catch (err) {
      ElMessage.error('复制失败，请手动复制')
    }
    document.body.removeChild(textArea)
  }
}

const handleFormSubmit = async () => {
  if (!formRef.value) return
  
  try {
    await formRef.value.validate()
    formLoading.value = true
    
    if (isEdit.value) {
      await clientsStore.updateClient(clientForm.value.id, {
        name: clientForm.value.name,
        description: clientForm.value.description
      })
      ElMessage.success('客户端更新成功')
      handleFormClose()
      await refreshData()
    } else {
      // 创建新客户端 - 不发送id字段，由后端自动生成
      const createData = {
        name: clientForm.value.name,
        description: clientForm.value.description
      }
      const result = await clientsStore.createClient(createData)
      
      // 如果返回了authtoken，显示成功对话框
      if (result && result.auth_token) {
        createdClient.value = {
          client_id: result.client_id,
          auth_token: result.auth_token
        }
        handleFormClose()
        successDialogVisible.value = true
      } else {
        ElMessage.success('客户端创建成功')
        handleFormClose()
      }
      
      await refreshData()
    }
    
  } catch (error) {
    if (error !== 'validation failed') {
      ElMessage.error(isEdit.value ? '更新客户端失败' : '创建客户端失败')
    }
  } finally {
    formLoading.value = false
  }
}

// 组件挂载
onMounted(async () => {
  await refreshData()
})

// 监听路由变化
watch(() => routesStore.routes, () => {
  // 路由数据更新时，重新计算客户端路由关联
}, { deep: true })
</script>

<style scoped>
.clients-page {
  padding: 0;
}

/* 页面头部 */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 24px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 8px 0;
  transition: color 0.3s ease;
}

.page-description {
  font-size: 14px;
  color: #909399;
  margin: 0;
  transition: color 0.3s ease;
}

/* 黑暗模式页面标题 */
.dark .page-title {
  color: #e5eaf3;
}

.dark .page-description {
  color: #b1b3b8;
}

/* 统计卡片 */
 .stats-container {
   display: flex;
   justify-content: space-between;
   align-items: stretch;
   gap: 16px;
   margin-bottom: 24px;
   flex-wrap: wrap;
 }

.stat-card {
    background: white;
    border-radius: 12px;
    padding: 20px 16px;
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: 12px;
    box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.08);
    flex: 1;
    min-width: 140px;
    border: 2px solid transparent;
    transition: all 0.3s ease;
    position: relative;
    overflow: hidden;
    cursor: pointer;
  }

/* 黑暗模式统计卡片 */
.dark .stat-card {
  background: #2d2d2d;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.3);
  color: #e5eaf3;
}
  
  .stat-card::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 4px;
    background: var(--card-accent-color, #ddd);
    transition: all 0.3s ease;
  }
  
  .stat-card:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 25px 0 rgba(0, 0, 0, 0.15);
  }
  
  .stat-card:active {
    transform: translateY(-2px);
    box-shadow: 0 4px 15px 0 rgba(0, 0, 0, 0.12);
  }
  
  .stat-card.total-card {
    border-color: #409eff;
    --card-accent-color: #409eff;
  }
  
  .stat-card.online-card {
    border-color: #67c23a;
    --card-accent-color: #67c23a;
  }
  
  .stat-card.offline-card {
    border-color: #f56c6c;
    --card-accent-color: #f56c6c;
  }
  
  .stat-card.enabled-card {
    border-color: #67c23a;
    --card-accent-color: #67c23a;
  }
  
  .stat-card.disabled-card {
    border-color: #909399;
    --card-accent-color: #909399;
  }

.stat-icon {
   width: 48px;
   height: 48px;
   border-radius: 12px;
   display: flex;
   align-items: center;
   justify-content: center;
   color: white;
   font-size: 20px;
   margin-bottom: 4px;
 }

.stat-icon.total {
  background: linear-gradient(135deg, #409eff, #66b3ff);
  box-shadow: 0 4px 12px rgba(64, 158, 255, 0.3);
}

.stat-icon.online {
  background: linear-gradient(135deg, #67c23a, #85ce61);
  box-shadow: 0 4px 12px rgba(103, 194, 58, 0.3);
}

.stat-icon.offline {
  background: linear-gradient(135deg, #f56c6c, #f78989);
  box-shadow: 0 4px 12px rgba(245, 108, 108, 0.3);
}

.stat-icon.enabled {
  background: linear-gradient(135deg, #67c23a, #85ce61);
  box-shadow: 0 4px 12px rgba(103, 194, 58, 0.3);
}

.stat-icon.disabled {
  background: linear-gradient(135deg, #909399, #a6a9ad);
  box-shadow: 0 4px 12px rgba(144, 147, 153, 0.3);
}

/* 成功对话框样式 */
.success-content {
  text-align: center;
}

.success-icon {
  margin-bottom: 16px;
}

.success-content h3 {
  color: #303133;
  margin: 16px 0;
  font-size: 18px;
  font-weight: 600;
  transition: color 0.3s ease;
}

.success-content p {
  color: #606266;
  margin: 8px 0 24px 0;
  transition: color 0.3s ease;
}

/* 黑暗模式成功对话框 */
.dark .success-content h3 {
  color: #e5eaf3;
}

.dark .success-content p {
  color: #b1b3b8;
}

.token-section {
  margin: 24px 0;
  text-align: left;
}

.readonly-input :deep(.el-input__inner) {
  background-color: #f5f7fa;
  color: #303133;
  transition: all 0.3s ease;
}

/* 黑暗模式只读输入框 */
.dark .readonly-input :deep(.el-input__inner) {
  background-color: #1e1e1e;
  color: #e5eaf3;
  border-color: #4c4d4f;
}

.token-input {
  font-family: 'Courier New', monospace;
}

.token-input :deep(.el-textarea__inner) {
  background-color: #f5f7fa;
  color: #303133;
  font-family: 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.4;
  transition: all 0.3s ease;
}

/* 黑暗模式token输入框 */
.dark .token-input :deep(.el-textarea__inner) {
  background-color: #1e1e1e;
  color: #e5eaf3;
  border-color: #4c4d4f;
}

.stat-icon.enabled {
  background: linear-gradient(135deg, #67c23a, #85ce61);
  box-shadow: 0 4px 12px rgba(103, 194, 58, 0.3);
}

.stat-icon.disabled {
  background: linear-gradient(135deg, #909399, #a6a9ad);
  box-shadow: 0 4px 12px rgba(144, 147, 153, 0.3);
}

.stat-content {
   display: flex;
   flex-direction: column;
   align-items: center;
   gap: 4px;
 }
 
 .stat-value {
   font-size: 28px;
   font-weight: 700;
   color: #303133;
   line-height: 1;
   letter-spacing: -0.5px;
   transition: color 0.3s ease;
 }
 
 .stat-label {
   font-size: 13px;
   color: #606266;
   font-weight: 500;
   text-transform: uppercase;
   letter-spacing: 0.5px;
   white-space: nowrap;
   transition: color 0.3s ease;
 }

/* 黑暗模式统计数值和标签 */
.dark .stat-value {
  color: #e5eaf3;
}

.dark .stat-label {
  color: #b1b3b8;
}

/* 搜索区域 */
.search-section {
  background: white;
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  transition: all 0.3s ease;
}

/* 黑暗模式搜索区域 */
.dark .search-section {
  background: #2d2d2d;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.3);
}

.search-left {
  display: flex;
  gap: 16px;
  align-items: center;
}

/* 表格容器 */
.table-container {
  background: white;
  border-radius: 8px;
  padding: 20px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  transition: all 0.3s ease;
}

/* 黑暗模式表格容器 */
.dark .table-container {
  background: #2d2d2d;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.3);
}

.client-name {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.name-line {
  display: flex;
  align-items: center;
  gap: 8px;
}

.description-line {
  font-size: 12px;
  color: #909399;
  max-width: 180px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: help;
  transition: color 0.3s ease;
}

/* 黑暗模式描述文字 */
.dark .description-line {
  color: #b1b3b8;
}

.client-icon {
  color: #409eff;
}

.action-buttons {
  display: flex;
  gap: 8px;
}

.pagination-container {
  display: flex;
  justify-content: center;
  margin-top: 20px;
}

/* 客户端详情 */
.client-detail {
  padding: 0;
}

.client-detail .el-descriptions {
  border-radius: 8px;
  overflow: hidden;
}

.client-detail .el-descriptions :deep(.el-descriptions__header) {
  background-color: #f8f9fa;
}

.client-detail .token-section {
  background-color: #f8f9fa;
  border-radius: 8px;
  padding: 16px;
  border: 1px solid #e4e7ed;
  transition: all 0.3s ease;
}

/* 黑暗模式token区域 */
.dark .client-detail .token-section {
  background-color: #1e1e1e;
  border-color: #4c4d4f;
}

.client-detail .token-section h4 {
  color: #303133;
  font-size: 14px;
  font-weight: 600;
  margin: 0 0 12px 0;
  display: flex;
  align-items: center;
  gap: 8px;
  transition: color 0.3s ease;
}

/* 黑暗模式token标题 */
.dark .client-detail .token-section h4 {
  color: #e5eaf3;
}

.client-detail .token-section h4::before {
  content: '🔐';
  font-size: 16px;
}

.client-routes {
  margin-top: 24px;
  background-color: #f8f9fa;
  border-radius: 8px;
  padding: 16px;
  border: 1px solid #e4e7ed;
  transition: all 0.3s ease;
}

/* 黑暗模式客户端路由区域 */
.dark .client-routes {
  background-color: #1e1e1e;
  border-color: #4c4d4f;
}

.client-routes h3 {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px 0;
  display: flex;
  align-items: center;
  gap: 8px;
  transition: color 0.3s ease;
}

/* 黑暗模式客户端路由标题 */
.dark .client-routes h3 {
  color: #e5eaf3;
}

.client-routes h3::before {
  content: '🔗';
  font-size: 16px;
}

.client-routes .el-table {
  border-radius: 6px;
  overflow: hidden;
}



/* 响应式设计 */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    gap: 16px;
    align-items: stretch;
  }
  
  .search-section {
    flex-direction: column;
    gap: 16px;
    align-items: stretch;
  }
  
  .search-left {
    flex-direction: column;
    align-items: stretch;
  }
  
  .action-buttons {
    flex-direction: column;
  }
  
  .stats-container {
     justify-content: center;
     gap: 12px;
   }
   
   .stat-card {
     min-width: 120px;
     flex: 1 1 calc(50% - 6px);
     max-width: 160px;
   }
   
   .stat-value {
     font-size: 24px;
   }
   
   .stat-label {
     font-size: 12px;
   }
}
</style>
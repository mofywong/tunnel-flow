<template>
  <div class="routes-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <div class="page-info">
        <h1 class="page-title">路由管理</h1>
        <p class="page-description">选择客户端后管理其路由配置，设置请求转发规则</p>
      </div>
    </div>

    <!-- 客户端选择区域 -->
    <div class="client-selection">
      <div class="selection-header">
        <h3>选择客户端</h3>
        <p>请先选择要管理路由的客户端</p>
      </div>
      <div class="client-selector-row">
        <div class="client-selector">
          <el-select
            v-model="selectedClientId"
            placeholder="请选择客户端"
            style="width: 300px"
            @change="handleClientChange"
            clearable
            filterable
            filter-placeholder="搜索客户端名称..."
          >
            <el-option
              v-for="client in availableClients"
              :key="client.client_id"
              :label="`${client.name || client.client_id} (${client.status === 'online' ? '在线' : '离线'})`"
              :value="client.client_id"
              :disabled="client.status !== 'online' && client.enabled !== 1"
            >
              <div class="client-option">
                <span>{{ client.name || client.client_id }}</span>
                <div class="client-status-info">
                  <el-tag 
                    :type="client.status === 'online' ? 'success' : client.enabled === 1 ? 'warning' : 'danger'"
                    size="small"
                  >
                    {{ client.status === 'online' ? '在线' : '离线' }}
                  </el-tag>
                </div>
              </div>
            </el-option>
          </el-select>
        </div>
        <div class="client-actions">
          <el-button 
            type="primary" 
            @click="showCreateDialog"
            :disabled="!selectedClientId"
          >
            <el-icon><Plus /></el-icon>
            新建路由
          </el-button>
          <el-button @click="refreshData">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
      </div>
    </div>
    
    <!-- 统计卡片 - 只在选择客户端后显示 -->
    <div v-if="selectedClientId" class="stats-cards">
      <div class="stat-card">
        <div class="stat-icon active">
          <el-icon><Link /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ clientRouteStats.active }}</div>
          <div class="stat-label">活跃路由</div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon total">
          <el-icon><Connection /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ clientRouteStats.total }}</div>
          <div class="stat-label">总路由</div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon inactive">
          <el-icon><Warning /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ clientRouteStats.inactive }}</div>
          <div class="stat-label">非活跃路由</div>
        </div>
      </div>
    </div>
    
    <!-- 搜索和筛选 - 只在选择客户端后显示 -->
    <div v-if="selectedClientId" class="search-section">
      <div class="search-left">
        <el-input
          v-model="searchQuery"
          placeholder="搜索路由路径或目标地址"
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
          <el-option label="已启用" value="enabled" />
          <el-option label="已禁用" value="disabled" />
        </el-select>

        <el-select
          v-model="modeFilter"
          placeholder="模式筛选"
          style="width: 120px"
          clearable
          @change="handleFilter"
        >
          <el-option label="基础模式" value="basic" />
          <el-option label="完整模式" value="full" />
        </el-select>
      </div>
      
      <div class="search-right">
        <el-button 
          type="success" 
          :disabled="selectedRoutes.length === 0" 
          @click="handleBatchEnable(true)"
        >
          <el-icon><Select /></el-icon>
          批量启用 ({{ selectedRoutes.length }})
        </el-button>
        
        <el-button 
          type="warning" 
          :disabled="selectedRoutes.length === 0" 
          @click="handleBatchEnable(false)"
        >
          <el-icon><Warning /></el-icon>
          批量禁用 ({{ selectedRoutes.length }})
        </el-button>
        
        <el-button type="danger" :disabled="selectedRoutes.length === 0" @click="handleBatchDelete">
          <el-icon><Delete /></el-icon>
          批量删除 ({{ selectedRoutes.length }})
        </el-button>
      </div>
    </div>
    
    <!-- 路由表格 - 只在选择客户端后显示 -->
    <div v-if="selectedClientId" class="table-container">
      <el-table
        :data="filteredRoutes"
        v-loading="routesStore.loading"
        @selection-change="handleSelectionChange"
        stripe
        style="width: 100%"
      >
        <el-table-column type="selection" width="55" />
        
        <el-table-column prop="url_suffix" label="服务器路径" min-width="250">
          <template #default="{ row }">
            <div class="route-path">
              <el-icon class="path-icon"><Link /></el-icon>
              <div class="path-content">
                <div class="path-text">{{ row.url_suffix }}</div>
                <div v-if="row.description" class="description-line" :title="row.description">
                  {{ row.description }}
                </div>
              </div>
              <el-button
                type="primary"
                link
                size="small"
                @click="copyPath(row.url_suffix)"
              >
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="targets_json" label="目标地址" min-width="220">
          <template #default="{ row }">
            <div class="target-url">
              <span class="target-text">{{ getTargetsDisplay(row.targets_json) }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="route_mode" label="路由模式" width="110">
          <template #default="{ row }">
            <el-tag :type="row.route_mode === 'full' ? 'primary' : 'success'" size="small">
              {{ row.route_mode === 'full' ? '完整模式' : '基础模式' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="enabled" label="启用状态" width="100">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled === 1"
              @change="(value) => handleToggleRoute(row, value)"
              :loading="row.toggling"
            />
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="创建时间" width="160">
          <template #default="{ row }">
            {{ formatDateTime(row.created_at) }}
          </template>
        </el-table-column>
        
        <el-table-column prop="updated_at" label="更新时间" width="160">
          <template #default="{ row }">
            {{ formatDateTime(row.updated_at) }}
          </template>
        </el-table-column>
        
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button type="primary" size="small" @click="editRoute(row)">
                <el-icon><Edit /></el-icon>
                编辑
              </el-button>
              
              <el-button type="danger" size="small" @click="deleteRoute(row)">
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
          v-model:current-page="routesStore.pagination.page"
          v-model:page-size="routesStore.pagination.pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="routesStore.pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </div>
    
    <!-- 未选择客户端时的提示 -->
    <div v-else class="no-client-selected">
      <el-empty description="请先选择要管理的客户端">
        <template #image>
          <el-icon size="100" color="#ccc"><Connection /></el-icon>
        </template>
      </el-empty>
    </div>
    
    <!-- 创建/编辑路由对话框 -->
    <el-dialog
      v-model="routeDialogVisible"
      :title="isEditing ? '编辑路由' : '新建路由'"
      width="600px"
      :before-close="handleRouteDialogClose"
    >
      <el-form
        ref="routeFormRef"
        :model="routeForm"
        :rules="routeFormRules"
        label-width="120px"
      >
        <!-- 关联客户端信息显示 -->
        <el-form-item label="关联客户端">
          <el-input
            :value="getSelectedClientName()"
            readonly
            style="width: 100%"
          >
            <template #suffix>
              <el-tag
                :type="getSelectedClientStatus() === 'online' ? 'success' : 'warning'"
                size="small"
              >
                {{ getSelectedClientStatus() === 'online' ? '在线' : '离线' }}
              </el-tag>
            </template>
          </el-input>
          <div class="form-tip">
            只能选择在线的客户端
          </div>
        </el-form-item>
        
        <el-form-item label="服务器路径" prop="url_suffix">
          <el-input
            v-model="routeForm.url_suffix"
            placeholder="例如: /api/v1/users 或 /api/*/users 或 /api/**/users"
          >
            <template #prepend>{{ serverBaseUrl }}</template>
          </el-input>
          <div class="form-tip">
            完整访问地址: {{ serverBaseUrl }}{{ routeForm.url_suffix }}<br>
            <strong>支持的匹配模式:</strong><br>
            • 精确匹配: <code>/api/users</code> - 只匹配 /api/users<br>
            • 单段通配符: <code>/api/*/users</code> - 匹配 /api/v1/users, /api/v2/users 等<br>
            • 段内通配符: <code>/api/user*</code> - 匹配 /api/users, /api/user123 等<br>
            • 双通配符: <code>/api/**/users</code> - 匹配 /api/users, /api/v1/users, /api/v1/v2/users 等<br>
            • 前缀匹配: <code>/api/*</code> - 匹配所有以 /api/ 开头的路径<br>
            • 后缀匹配: <code>*/users</code> - 匹配所有以 /users 结尾的路径
          </div>
        </el-form-item>
        
        <el-form-item label="路由配置模式" prop="route_mode">
          <el-select
            v-model="routeForm.route_mode"
            placeholder="选择路由模式"
            style="width: 100%"
          >
            <el-option label="基础模式" value="basic">
              <div>
                <div>基础模式</div>
                <div style="font-size: 12px; color: #999;">仅配置目的IP:PORT，自动匹配原始请求的URL后缀路径</div>
              </div>
            </el-option>
            <el-option label="完整模式" value="full">
              <div>
                <div>完整模式</div>
                <div style="font-size: 12px; color: #999;">配置固定完整URL地址的路由转发</div>
              </div>
            </el-option>
          </el-select>
          <div class="form-tip">
            <strong>基础模式</strong>：目标地址格式为 http://ip:port，请求时自动拼接原始URL路径<br>
            <strong>完整模式</strong>：目标地址为完整URL，请求时直接转发到指定地址
          </div>
        </el-form-item>
        
        <el-form-item label="目标地址" prop="targets_json">
          <el-input
            v-model="routeForm.targets_json"
            :placeholder="routeForm.route_mode === 'basic' 
              ? '例如: 192.168.1.100:8080'
              : '例如: http://192.168.1.100:8080/api/v1'"
          />
          <div class="form-tip">
            <span v-if="routeForm.route_mode === 'basic'">
              基础模式：仅需配置IP和端口，系统会自动拼接原始请求路径
            </span>
            <span v-else>
              完整模式：配置完整的目标URL地址
            </span>
          </div>
        </el-form-item>


        
        <el-form-item label="启用状态">
          <el-switch
            v-model="routeForm.enabled"
            active-text="启用"
            inactive-text="禁用"
          />
          <div class="form-tip">
            禁用的路由不会处理请求转发
          </div>
        </el-form-item>

        <el-form-item label="描述">
          <el-input
            v-model="routeForm.description"
            type="textarea"
            :rows="3"
            placeholder="路由描述（可选）"
          />
        </el-form-item>
      </el-form>
      
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="handleRouteDialogClose">取消</el-button>
          <el-button type="primary" @click="handleRouteSubmit" :loading="submitting">
            {{ isEditing ? '更新' : '创建' }}
          </el-button>
        </div>
      </template>
    </el-dialog>
    

  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Refresh, Link, Connection, Warning, Search, Delete, Edit,
  CopyDocument, Select
} from '@element-plus/icons-vue'
import { useRoutesStore } from '@/stores/routes'
import { useClientsStore } from '@/stores/clients'
import { api } from '@/utils/api'

const routesStore = useRoutesStore()
const clientsStore = useClientsStore()

// 响应式数据
const selectedClientId = ref('')
const searchQuery = ref('')
const statusFilter = ref('')
const modeFilter = ref('')
const selectedRoutes = ref([])
const clientRoutes = ref([])
const routeDialogVisible = ref(false)
const isEditing = ref(false)
const submitting = ref(false)
const routeFormRef = ref()
const serverInfo = ref({
  base_url: window.location.origin, // 默认值
  proxy_url: null // 代理服务器URL
})

// 表单数据
const routeForm = ref({
  url_suffix: '',
  targets_json: '',
  client_id: '',
  strategy: 'first_available',
  route_mode: 'basic',
  enabled: true,
  description: ''
})



// 表单验证规则
const routeFormRules = {
  url_suffix: [
    { required: true, message: '请输入服务器路径', trigger: 'blur' },
    { pattern: /^\//, message: '路径必须以 / 开头', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        // 检查通配符使用是否合理
        if (value && value.includes('*')) {
          const parts = value.split('*')
          if (parts.length > 5) {
            callback(new Error('通配符使用过多，请简化路径模式'))
            return
          }
        }
        callback()
      },
      trigger: 'blur'
    }
  ],
  targets_json: [
    { required: true, message: '请输入目标地址', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (!value || value.trim() === '') {
          callback(new Error('请输入目标地址'))
          return
        }

        const routeMode = routeForm.value.route_mode
        const urlValue = value.trim()
        
        // 首先检查是否为简单的URL格式（推荐格式）
        if (routeMode === 'basic') {
          // 基础模式：验证是否为 http://ip:port 格式
          if (urlValue.match(/^https?:\/\/[^\/]+:\d+\/?$/)) {
            callback()
            return
          }
        } else {
          // 完整模式：验证是否为完整的HTTP(S) URL
          if (urlValue.match(/^https?:\/\/.+/)) {
            callback()
            return
          }
        }
        
        // 如果不是简单URL格式，尝试解析为JSON数组（兼容旧格式）
        try {
          const targets = JSON.parse(value)
          if (Array.isArray(targets) && targets.length > 0) {
            // 验证JSON数组格式
            for (const target of targets) {
              if (!target.url) {
                callback(new Error('每个目标必须包含url字段'))
                return
              }
              
              if (routeMode === 'basic') {
                if (!target.url.match(/^https?:\/\/[^\/]+:\d+\/?$/)) {
                  callback(new Error('基础模式下目标地址格式应为: http://ip:port'))
                  return
                }
              } else {
                if (!target.url.match(/^https?:\/\/.+/)) {
                  callback(new Error('完整模式下目标地址必须是有效的HTTP(S) URL'))
                  return
                }
              }
            }
            callback()
            return
          }
        } catch (error) {
          // JSON解析失败，显示相应的错误信息
        }
        
        // 如果既不是有效的URL也不是有效的JSON，显示错误
        if (routeMode === 'basic') {
          callback(new Error('基础模式下目标地址格式应为: http://ip:port'))
        } else {
          callback(new Error('完整模式下目标地址必须是有效的HTTP(S) URL'))
        }
      },
      trigger: 'blur'
    }
  ]
}



// 计算属性
const serverBaseUrl = computed(() => {
  // 优先使用从后端获取的代理服务器URL
  return serverInfo.value.proxy_url || serverInfo.value.base_url
})

const availableClients = computed(() => {
  return clientsStore.clients || []
})

const selectedClient = computed(() => {
  return availableClients.value.find(client => client.client_id === selectedClientId.value)
})

const clientRouteStats = computed(() => {
  const routes = clientRoutes.value
  return {
    total: routes.length,
    active: routes.filter(route => route.enabled === 1).length,
    inactive: routes.filter(route => route.enabled === 0).length
  }
})

const filteredRoutes = computed(() => {
  let routes = clientRoutes.value
  
  // 状态筛选
  if (statusFilter.value) {
    if (statusFilter.value === 'enabled') {
      routes = routes.filter(route => route.enabled === 1)
    } else if (statusFilter.value === 'disabled') {
      routes = routes.filter(route => route.enabled === 0)
    }
  }
  
  // 模式筛选
  if (modeFilter.value) {
    routes = routes.filter(route => route.route_mode === modeFilter.value)
  }
  
  // 搜索筛选
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    routes = routes.filter(route => 
      route.url_suffix.toLowerCase().includes(query) ||
      getTargetsDisplay(route.targets_json).toLowerCase().includes(query) ||
      (route.description && route.description.toLowerCase().includes(query))
    )
  }
  
  return routes
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

const getClientName = (clientId) => {
  const client = clientsStore.clients.find(c => c.id === clientId)
  return client ? (client.name || client.id) : '未知客户端'
}

const getClientStatus = (clientId) => {
  const client = clientsStore.clients.find(c => c.id === clientId)
  return client ? client.status : 'offline'
}

const getSelectedClientName = () => {
  return selectedClient.value ? (selectedClient.value.name || selectedClient.value.client_id) : ''
}

const getSelectedClientStatus = () => {
  return selectedClient.value ? selectedClient.value.status : 'offline'
}

const getTargetsDisplay = (targetsJson) => {
  if (!targetsJson) return '-'
  try {
    const targets = JSON.parse(targetsJson)
    if (Array.isArray(targets) && targets.length > 0) {
      if (targets.length === 1) {
        return targets[0].url
      } else {
        return `${targets[0].url} (+${targets.length - 1}个)`
      }
    }
    return '-'
  } catch (e) {
    return targetsJson
  }
}



// 客户端选择处理
const handleClientChange = async (clientId) => {
  if (clientId) {
    await loadClientRoutes(clientId)
  } else {
    clientRoutes.value = []
  }
  // 重置搜索和筛选
  searchQuery.value = ''
  statusFilter.value = ''
  selectedRoutes.value = []
}

// 获取服务器信息
const loadServerInfo = async () => {
  try {
    const response = await api.get('/server-info')
    if (response.data) {
      // 更新服务器信息，优先使用proxy_url
      serverInfo.value = {
        base_url: response.data.base_url || window.location.origin,
        proxy_url: response.data.proxy_url || null
      }
    }
  } catch (error) {
    console.error('获取服务器信息失败:', error)
    // 保持默认值
  }
}

// 加载指定客户端的路由
const loadClientRoutes = async (clientId) => {
  try {
    routesStore.loading = true
    const response = await api.get(`/routes?client_id=${clientId}`)
    clientRoutes.value = response.data || []
  } catch (error) {
    console.error('加载客户端路由失败:', error)
    ElMessage.error('加载客户端路由失败')
    clientRoutes.value = []
  } finally {
    routesStore.loading = false
  }
}

// 事件处理
const refreshData = async () => {
  try {
    await clientsStore.fetchClients()
    if (selectedClientId.value) {
      await loadClientRoutes(selectedClientId.value)
    }
    ElMessage.success('数据已刷新')
  } catch (error) {
    ElMessage.error('刷新数据失败')
  }
}

const handleSearch = () => {
  // 搜索逻辑已在计算属性中处理
}

const handleFilter = () => {
  // 筛选逻辑已在计算属性中处理
}

const handleSelectionChange = (selection) => {
  selectedRoutes.value = selection
}

const handleSizeChange = (size) => {
  routesStore.setPageSize(size)
}

const handleCurrentChange = (page) => {
  routesStore.setPage(page)
}

// 路由操作
const showCreateDialog = () => {
  if (!selectedClientId.value) {
    ElMessage.warning('请先选择客户端')
    return
  }
  isEditing.value = false
  routeForm.value = {
    url_suffix: '',
    targets_json: '',
    client_id: selectedClientId.value,
    strategy: 'first_available',
    route_mode: 'basic',
    enabled: true,
    description: ''
  }
  routeDialogVisible.value = true
}

const editRoute = (route) => {
  isEditing.value = true
  
  // 将JSON格式的targets_json转换为简单的URL显示
  let displayTargets = route.targets_json
  try {
    const targets = JSON.parse(route.targets_json)
    if (Array.isArray(targets) && targets.length > 0 && targets[0].url) {
      displayTargets = targets[0].url
    }
  } catch (e) {
    // 如果解析失败，保持原值
    displayTargets = route.targets_json
  }
  
  routeForm.value = {
    id: route.id,
    url_suffix: route.url_suffix,
    targets_json: displayTargets,
    client_id: route.client_id,
    route_mode: route.route_mode || 'basic',
    enabled: route.enabled === 1,
    description: route.description || ''
  }
  routeDialogVisible.value = true
}

const handleRouteSubmit = async () => {
  if (!routeFormRef.value) return
  
  try {
    await routeFormRef.value.validate()
    submitting.value = true
    
    // 准备提交数据，将enabled字段从boolean转换为int
    const formData = { ...routeForm.value }
    // 将boolean类型的enabled转换为int类型（后端期望的格式）
    formData.enabled = formData.enabled ? 1 : 0
    
    if (isEditing.value) {
      await routesStore.updateRoute(formData.id, formData)
      ElMessage.success('路由更新成功')
    } else {
      await routesStore.createRoute(formData)
      ElMessage.success('路由创建成功')
    }
    
    routeDialogVisible.value = false
    // 重新加载当前客户端的路由
    if (selectedClientId.value) {
      await loadClientRoutes(selectedClientId.value)
    }
  } catch (error) {
    if (error !== 'validation failed') {
      ElMessage.error(isEditing.value ? '路由更新失败' : '路由创建失败')
    }
  } finally {
    submitting.value = false
  }
}

const deleteRoute = async (route) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除路由 "${route.server_path}" 吗？此操作不可恢复。`,
      '确认删除',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    await routesStore.deleteRoute(route.id)
    ElMessage.success('路由已删除')
    
    // 重新加载当前客户端的路由
    if (selectedClientId.value) {
      await loadClientRoutes(selectedClientId.value)
    }
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除路由失败')
    }
  }
}

const handleBatchDelete = async () => {
  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${selectedRoutes.value.length} 个路由吗？此操作不可恢复。`,
      '批量删除确认',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    const promises = selectedRoutes.value.map(route => 
      routesStore.deleteRoute(route.id)
    )
    
    await Promise.all(promises)
    ElMessage.success('批量删除成功')
    selectedRoutes.value = []
    
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('批量删除失败')
    }
  }
}

const handleRouteDialogClose = () => {
  routeDialogVisible.value = false
  if (routeFormRef.value) {
    routeFormRef.value.resetFields()
  }
}

// 工具函数
const copyPath = async (path) => {
  try {
    const fullUrl = `${serverBaseUrl.value}${path}`
    await navigator.clipboard.writeText(fullUrl)
    ElMessage.success('路径已复制到剪贴板')
  } catch (error) {
    ElMessage.error('复制失败')
  }
}



// 路由启用/禁用处理
const handleToggleRoute = async (route, enabled) => {
  try {
    // 设置加载状态
    route.toggling = true
    
    const response = await api.put(`/routes/${route.id}/enabled`, {
      enabled: enabled
    })
    
    // 检查响应状态，204 No Content 也表示成功
    if (response.status === 204 || response.status === 200) {
      // 更新本地数据
      route.enabled = enabled ? 1 : 0
      ElMessage.success(enabled ? '路由已启用' : '路由已禁用')
    }
  } catch (error) {
    console.error('切换路由状态失败:', error)
    ElMessage.error('操作失败')
  } finally {
    route.toggling = false
  }
}

// 批量启用/禁用路由
const handleBatchEnable = async (enabled) => {
  if (selectedRoutes.value.length === 0) {
    ElMessage.warning('请先选择要操作的路由')
    return
  }
  
  try {
    const action = enabled ? '启用' : '禁用'
    await ElMessageBox.confirm(
      `确定要${action}选中的 ${selectedRoutes.value.length} 个路由吗？`,
      `批量${action}路由`,
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    const routeIds = selectedRoutes.value.map(route => route.id)
    
    const response = await api.put('/routes/batch/enabled', {
      route_ids: routeIds,
      enabled: enabled
    })
    
    if (response.data) {
      // 更新本地数据
      selectedRoutes.value.forEach(route => {
        route.enabled = enabled ? 1 : 0
      })
      
      ElMessage.success(`成功${action}了 ${selectedRoutes.value.length} 个路由`)
      selectedRoutes.value = []
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量操作失败:', error)
      ElMessage.error('批量操作失败')
    }
  }
}





// 组件挂载
onMounted(async () => {
  await loadServerInfo()
  await clientsStore.fetchClients()
})
</script>

<style scoped>
.routes-page {
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

/* 客户端选择区域样式 */
.client-selection {
  background: white;
  padding: 24px;
  border-radius: 8px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  margin-bottom: 24px;
  transition: all 0.3s ease;
}

.selection-header h3 {
  margin: 0 0 8px 0;
  color: #303133;
  font-size: 18px;
  font-weight: 600;
  transition: color 0.3s ease;
}

.selection-header p {
  margin: 0 0 16px 0;
  color: #606266;
  font-size: 14px;
  transition: color 0.3s ease;
}

/* 黑暗模式客户端选择区域 */
.dark .client-selection {
  background: #2d2d2d;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
}

.dark .selection-header h3 {
  color: #e5eaf3;
}

.dark .selection-header p {
  color: #b1b3b8;
}

.client-selector-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.client-actions {
  display: flex;
  gap: 12px;
}

.client-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.client-status-info {
  margin-left: 8px;
}

/* 未选择客户端提示样式 */
.no-client-selected {
  background: white;
  padding: 60px 24px;
  border-radius: 8px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  text-align: center;
  transition: all 0.3s ease;
}

/* 统计卡片 */
.stats-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  background: white;
  border-radius: 8px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  transition: all 0.3s ease;
}

/* 黑暗模式未选择客户端提示 */
.dark .no-client-selected {
  background: #2d2d2d;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
  color: #e5eaf3;
}

/* 黑暗模式统计卡片 */
.dark .stat-card {
  background: #2d2d2d;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.3);
}

.stat-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: white;
}

.stat-icon.active {
  background: #67c23a;
}

.stat-icon.total {
  background: #409eff;
}

.stat-icon.inactive {
  background: #909399;
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  color: #303133;
  line-height: 1;
  margin-bottom: 4px;
  transition: color 0.3s ease;
}

.stat-label {
  font-size: 14px;
  color: #909399;
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

/* 黑暗模式搜索区域 */
.dark .search-section {
  background: #2d2d2d;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.3);
}

/* 黑暗模式表格容器 */
.dark .table-container {
  background: #2d2d2d;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.3);
}

.route-path {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 4px 0;
}

.path-icon {
  color: #409eff;
  margin-top: 3px;
  flex-shrink: 0;
}

.path-content {
  flex: 1;
  min-width: 0;
}

.path-text {
  font-weight: 500;
  font-family: 'Courier New', monospace;
  color: #303133;
  line-height: 1.4;
  word-break: break-all;
}

/* 黑暗模式路径文字 */
.dark .path-text {
  color: #e4e7ed;
}

.description-line {
  font-size: 12px;
  color: #909399;
  line-height: 1.3;
  margin-top: 4px;
  max-width: 200px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: help;
  transition: color 0.3s ease;
}

.description-line:hover {
  color: #606266;
}

/* 黑暗模式描述文字 */
.dark .description-line {
  color: #b1b3b8;
}

.dark .description-line:hover {
  color: #c0c4cc;
}

.target-url {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}

.target-text {
  font-family: 'Courier New', monospace;
  color: #606266;
  word-break: break-all;
  line-height: 1.4;
}

/* 黑暗模式目标地址文字 */
.dark .target-text {
  color: #c0c4cc;
}

.client-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.client-status .online {
  color: #67c23a;
}

.client-status .offline {
  color: #f56c6c;
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

/* 表单样式 */
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}

.form-tip code {
  background-color: #f5f7fa;
  border: 1px solid #e4e7ed;
  border-radius: 3px;
  padding: 2px 4px;
  font-family: 'Courier New', monospace;
  font-size: 11px;
  color: #e6a23c;
}

.form-tip strong {
  color: #606266;
  font-weight: 600;
}

.client-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.dialog-footer {
  text-align: right;
}



/* 响应式设计 */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    gap: 16px;
    align-items: stretch;
  }
  
  .header-right {
    justify-content: flex-start;
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
  
  .stats-cards {
    grid-template-columns: 1fr;
  }
  
  .route-path,
  .target-url {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }
}
</style>
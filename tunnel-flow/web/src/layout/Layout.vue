<template>
  <div class="layout-container">
    <!-- 侧边栏 -->
    <aside 
      class="sidebar" 
      :class="{ 'sidebar-collapsed': systemStore.sidebarCollapsed }"
    >
      <div class="sidebar-header">
        <div class="logo">
          <el-icon class="logo-icon"><Connection /></el-icon>
          <span v-show="!systemStore.sidebarCollapsed" class="logo-text">Tunnel Flow</span>
        </div>
      </div>
      
      <nav class="sidebar-nav">
        <el-menu
          :default-active="$route.path"
          :collapse="systemStore.sidebarCollapsed"
          :unique-opened="true"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/clients">
            <el-icon><Monitor /></el-icon>
            <template #title>客户端管理</template>
          </el-menu-item>
          
          <el-menu-item index="/routes">
            <el-icon><Connection /></el-icon>
            <template #title>路由管理</template>
          </el-menu-item>

        </el-menu>
      </nav>
      
      <!-- 系统状态指示器 -->
      <div class="sidebar-footer" v-show="!systemStore.sidebarCollapsed">
        <div class="status-indicator">
          <div class="status-item">
            <el-badge is-dot :type="systemStore.isSystemHealthy ? 'success' : 'danger'" class="status-badge" />
            <span class="status-text">{{ systemStatusText }}</span>
          </div>
          <div class="status-item" v-if="systemStore.connectedClientsCount > 0">
            <el-icon class="status-icon text-success"><User /></el-icon>
            <span class="status-text">{{ systemStore.connectedClientsCount }} 个客户端</span>
          </div>
        </div>
      </div>
    </aside>
    
    <!-- 主内容区 -->
    <div class="main-container">
      <!-- 顶部导航栏 -->
      <header class="header">
        <div class="header-left">
          <el-button 
            type="text" 
            @click="systemStore.toggleSidebar()"
            class="sidebar-toggle"
          >
            <el-icon><Expand v-if="systemStore.sidebarCollapsed" /><Fold v-else /></el-icon>
          </el-button>
          
          <el-breadcrumb separator="/" class="breadcrumb">
            <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item v-if="$route.meta.title">{{ $route.meta.title }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        
        <div class="header-right">

          
          <!-- 主题切换 -->
          <el-dropdown class="theme-dropdown" @command="systemStore.setTheme">
            <el-button 
              type="text" 
              class="theme-toggle"
            >
              <el-icon>
                <Monitor v-if="systemStore.theme === 'system'" />
                <Sunny v-else-if="systemStore.actualTheme === 'light'" />
                <Moon v-else />
              </el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="light" :class="{ 'is-active': systemStore.theme === 'light' }">
                  <el-icon><Sunny /></el-icon>
                  明亮模式
                </el-dropdown-item>
                <el-dropdown-item command="dark" :class="{ 'is-active': systemStore.theme === 'dark' }">
                  <el-icon><Moon /></el-icon>
                  黑暗模式
                </el-dropdown-item>
                <el-dropdown-item command="system" :class="{ 'is-active': systemStore.theme === 'system' }">
                  <el-icon><Monitor /></el-icon>
                  跟随系统
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          
          <!-- 刷新按钮 -->
          <el-button 
            type="text" 
            @click="refreshData"
            :loading="refreshing"
            class="refresh-btn"
          >
            <el-icon><Refresh /></el-icon>
          </el-button>
          
          <!-- 用户菜单 -->
          <el-dropdown class="user-dropdown">
            <div class="user-info">
              <el-avatar :size="32" class="user-avatar">
                <el-icon><User /></el-icon>
              </el-avatar>
              <span class="username">管理员</span>
              <el-icon class="dropdown-icon"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="logout">
                  <el-icon><SwitchButton /></el-icon>
                  退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>
      
      <!-- 页面内容 -->
      <main class="content">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
    
    <!-- 全局错误提示 -->
    <el-alert
      v-if="systemStore.error"
      :title="systemStore.error"
      type="error"
      :closable="true"
      @close="systemStore.clearError()"
      class="global-error"
    />
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Monitor, ChatDotRound,
  Expand, Fold, Sunny, Moon, Refresh, User, 
  ArrowRight, Lock
} from '@element-plus/icons-vue'
import { useSystemStore } from '@/stores/system'
import { useClientsStore } from '@/stores/clients'
import { useRoutesStore } from '@/stores/routes'


const router = useRouter()
const systemStore = useSystemStore()
const clientsStore = useClientsStore()
const routesStore = useRoutesStore()

const refreshing = ref(false)



// 系统状态
const systemStatusClass = computed(() => {
  return systemStore.isSystemHealthy ? 'text-success' : 'text-danger'
})

const systemStatusText = computed(() => {
  return systemStore.isSystemHealthy ? '系统正常' : '系统异常'
})

// 刷新数据
const refreshData = async () => {
  refreshing.value = true
  try {
    await systemStore.fetchSystemStatus()
    ElMessage.success('数据已刷新')
  } catch (error) {
    ElMessage.error('刷新失败')
  } finally {
    refreshing.value = false
  }
}

// 退出登录
const logout = () => {
  localStorage.removeItem('token')
  router.push('/login')
  ElMessage.success('已退出登录')
}



// 组件挂载
onMounted(async () => {
  // 初始化系统状态
  await systemStore.initializeSystem()
})


</script>

<style scoped>
.layout-container {
  display: flex;
  height: 100vh;
  background-color: #f5f5f5;
}

/* 侧边栏样式 */
.sidebar {
  width: 250px;
  background: white;
  border-right: 1px solid #e4e7ed;
  transition: width 0.3s ease;
  display: flex;
  flex-direction: column;
  box-shadow: 2px 0 6px rgba(0, 0, 0, 0.1);
}

.sidebar-collapsed {
  width: 64px;
}

.sidebar-header {
  height: 60px;
  display: flex;
  align-items: center;
  padding: 0 20px;
  border-bottom: 1px solid #e4e7ed;
}

.logo {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-icon {
  font-size: 24px;
  color: #409eff;
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
}

.sidebar-nav {
  flex: 1;
  overflow-y: auto;
}

.sidebar-menu {
  border: none;
}

.sidebar-footer {
  padding: 16px;
  border-top: 1px solid #e4e7ed;
  background: #fafafa;
}

.status-indicator {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #606266;
}

.status-icon {
  font-size: 8px;
}

/* 主内容区样式 */
.main-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.header {
  height: 60px;
  background: white;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.sidebar-toggle {
  font-size: 18px;
}

.breadcrumb {
  font-size: 14px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.connection-status {
  display: flex;
  align-items: center;
}

.connection-icon {
  font-size: 16px;
  cursor: pointer;
}

.theme-toggle,
.refresh-btn {
  font-size: 16px;
}

.theme-dropdown .el-dropdown-menu__item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.theme-dropdown .el-dropdown-menu__item.is-active {
  background-color: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}

.theme-dropdown .el-dropdown-menu__item.is-active .el-icon {
  color: var(--el-color-primary);
}

.user-dropdown {
  cursor: pointer;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-radius: 4px;
  transition: background-color 0.3s;
}

.user-info:hover {
  background-color: #f5f7fa;
}

.username {
  font-size: 14px;
  color: #303133;
}

.dropdown-icon {
  font-size: 12px;
  color: #909399;
}

.content {
  flex: 1;
  padding: 20px;
  overflow-y: auto;
  background: #f5f5f5;
}

/* 全局错误提示 */
.global-error {
  position: fixed;
  top: 20px;
  right: 20px;
  z-index: 9999;
  max-width: 400px;
}

/* 过渡动画 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    height: 100vh;
    z-index: 1000;
    transform: translateX(-100%);
    transition: transform 0.3s ease;
  }
  
  .sidebar:not(.sidebar-collapsed) {
    transform: translateX(0);
  }
  
  .main-container {
    margin-left: 0;
  }
  
  .header {
    padding: 0 16px;
  }
  
  .content {
    padding: 16px;
  }
  
  .user-info .username {
    display: none;
  }
}

/* 暗色主题 */
.dark .layout-container {
  background-color: #1a1a1a;
}

.dark .sidebar {
  background: #2d2d2d;
  border-right-color: #404040;
}

.dark .sidebar-header {
  border-bottom-color: #404040;
}

.dark .logo-text {
  color: #e5eaf3;
}

.dark .sidebar-footer {
  background: #262626;
  border-top-color: #404040;
}

.dark .header {
  background: #2d2d2d;
  border-bottom-color: #404040;
}

.dark .user-info:hover {
  background-color: #404040;
}

.dark .username {
  color: #e5eaf3;
}

.dark .content {
  background: #1a1a1a;
}
</style>
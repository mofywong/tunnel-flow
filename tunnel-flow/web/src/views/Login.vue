<template>
  <div class="login-page">
    <!-- 主题切换按钮 -->
    <div class="theme-toggle-container">
      <el-dropdown class="theme-dropdown" @command="setTheme">
        <el-button 
          type="text" 
          class="theme-toggle-btn"
          size="large"
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
    </div>

    <div class="login-container">
      <div class="login-header">
        <div class="logo">
          <el-icon size="48"><Connection /></el-icon>
        </div>
        <h1>Tunnel Flow</h1>
        <p>HTTP隧道透传代理平台</p>
      </div>
      
      <div class="login-card-wrapper">
        <div class="login-card-glass">
          <div class="card-header">
            <span>用户登录</span>
          </div>
          
          <el-form
            ref="loginFormRef"
            :model="loginForm"
            :rules="loginRules"
            size="large"
            @keyup.enter="handleLogin"
          >
            <el-form-item prop="username">
              <el-input
                v-model="loginForm.username"
                placeholder="请输入用户名"
                :prefix-icon="User"
                clearable
              />
            </el-form-item>
            
            <el-form-item prop="password">
              <el-input
                v-model="loginForm.password"
                type="password"
                placeholder="请输入密码"
                :prefix-icon="Lock"
                show-password
                clearable
              />
            </el-form-item>
            
            <el-form-item>
              <div class="login-options">
                <el-checkbox v-model="loginForm.remember">记住密码</el-checkbox>
                <el-link type="primary" @click="showForgotPassword = true">忘记密码？</el-link>
              </div>
            </el-form-item>
            
            <el-form-item>
              <el-button
                type="primary"
                size="large"
                style="width: 100%;"
                :loading="loading"
                @click.prevent="handleLogin"
                native-type="submit"
              >
                {{ loading ? '登录中...' : '登录' }}
              </el-button>
            </el-form-item>
          </el-form>
          
          <div class="login-footer">
            <el-divider>其他登录方式</el-divider>
            <div class="social-login">
              <el-button circle :icon="Message" @click="showQRLogin = true" />
              <el-button circle :icon="Phone" @click="showPhoneLogin = true" />
            </div>
          </div>
        </div>
      </div>
      
      <div class="login-info">
        <p>© 2025 Tunnel Flow. All rights reserved.</p>
        <p>Version 1.0.0</p>
      </div>
    </div>
    
    <!-- 背景装饰 -->
    <div class="background-decoration">
      <div class="decoration-item item-1"></div>
      <div class="decoration-item item-2"></div>
      <div class="decoration-item item-3"></div>
      <div class="decoration-item item-4"></div>
      <div class="decoration-item item-5"></div>
    </div>

    <!-- 忘记密码对话框 -->
    <el-dialog
      v-model="showForgotPassword"
      title="找回密码"
      width="400px"
    >
      <el-form :model="forgotForm" label-width="80px">
        <el-form-item label="邮箱">
          <el-input v-model="forgotForm.email" placeholder="请输入注册邮箱" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForgotPassword = false">取消</el-button>
        <el-button type="primary" @click="handleForgotPassword">发送重置邮件</el-button>
      </template>
    </el-dialog>
    
    <!-- 二维码登录对话框 -->
    <el-dialog
      v-model="showQRLogin"
      title="扫码登录"
      width="300px"
      align-center
    >
      <div class="qr-login-content">
        <div class="qr-code">
          <el-image
            :src="qrCodeImage"
            alt="登录二维码"
            style="width: 200px; height: 200px;"
          />
        </div>
        <p>请使用手机扫描二维码登录</p>
        <el-button @click="refreshQRCode">刷新二维码</el-button>
      </div>
    </el-dialog>
    
    <!-- 手机登录对话框 -->
    <el-dialog
      v-model="showPhoneLogin"
      title="手机登录"
      width="400px"
    >
      <el-form :model="phoneForm" label-width="80px">
        <el-form-item label="手机号">
          <el-input v-model="phoneForm.phone" placeholder="请输入手机号" />
        </el-form-item>
        <el-form-item label="验证码">
          <div class="sms-container">
            <el-input v-model="phoneForm.code" placeholder="请输入验证码" style="flex: 1;" />
            <el-button
              :disabled="smsCountdown > 0"
              @click="sendSMSCode"
            >
              {{ smsCountdown > 0 ? `${smsCountdown}s` : '发送验证码' }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showPhoneLogin = false">取消</el-button>
        <el-button type="primary" @click="handlePhoneLogin">登录</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Connection, User, Lock, Message, Phone, Sunny, Moon, Monitor
} from '@element-plus/icons-vue'
import * as authAPI from '@/api/auth'
import { useSystemStore } from '@/stores/system'

const router = useRouter()
const systemStore = useSystemStore()

// 主题切换功能
const setTheme = (theme) => {
  systemStore.setTheme(theme)
}

// 响应式数据
const loading = ref(false)

const showForgotPassword = ref(false)
const showQRLogin = ref(false)
const showPhoneLogin = ref(false)

const qrCodeImage = ref('')
const smsCountdown = ref(0)
const loginFormRef = ref()

// 表单数据
const loginForm = reactive({
  username: '',
  password: '',
  remember: false
})

const forgotForm = reactive({
  email: ''
})

const phoneForm = reactive({
  phone: '',
  code: ''
})

// 表单验证规则
const loginRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 20, message: '用户名长度在 3 到 20 个字符', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, max: 20, message: '密码长度在 6 到 20 个字符', trigger: 'blur' }
  ]
}

// 事件处理
const handleLogin = async () => {
  console.log('handleLogin called')
  if (!loginFormRef.value) {
    console.log('loginFormRef is null')
    return
  }
  
  try {
    console.log('Starting form validation')
    const valid = await loginFormRef.value.validate()
    console.log('Form validation result:', valid)
    if (!valid) {
      console.log('Form validation failed')
      return
    }
    
    loading.value = true
    console.log('Calling login API with:', { username: loginForm.username, password: loginForm.password })
    
    // 调用真实的登录API
    const response = await authAPI.login({
      username: loginForm.username,
      password: loginForm.password
    })
    
    console.log('Login API response:', response)
    ElMessage.success('登录成功')
    
    // 保存登录状态
    const responseData = response.data.data || response.data
    console.log('Saving login data:', responseData)
    localStorage.setItem('token', responseData.token)
    localStorage.setItem('user', JSON.stringify(responseData.user))
    
    // 确保token保存完成后再跳转，避免时序问题
    await nextTick()
    
    // 跳转到首页
    router.push('/')
  } catch (error) {
    console.error('Login error:', error)
    console.error('Error response:', error.response)
    ElMessage.error('登录失败: ' + (error.response?.data?.message || error.message))
  } finally {
    loading.value = false
  }
}

const handleForgotPassword = async () => {
  if (!forgotForm.email) {
    ElMessage.warning('请输入邮箱地址')
    return
  }
  
  try {
    // 模拟发送重置邮件
    await new Promise(resolve => setTimeout(resolve, 1000))
    ElMessage.success('重置邮件已发送，请查收')
    showForgotPassword.value = false
    forgotForm.email = ''
  } catch (error) {
    ElMessage.error('发送失败: ' + error.message)
  }
}

const handlePhoneLogin = async () => {
  if (!phoneForm.phone || !phoneForm.code) {
    ElMessage.warning('请输入手机号和验证码')
    return
  }
  
  try {
    // 模拟手机登录
    await new Promise(resolve => setTimeout(resolve, 1000))
    ElMessage.success('登录成功')
    showPhoneLogin.value = false
    router.push('/')
  } catch (error) {
    ElMessage.error('登录失败: ' + error.message)
  }
}



const refreshQRCode = () => {
  // 生成随机二维码
  qrCodeImage.value = `data:image/svg+xml;base64,${btoa(`
    <svg width="200" height="200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="white"/>
      <rect x="20" y="20" width="160" height="160" fill="none" stroke="#333" stroke-width="2"/>
      <text x="100" y="110" text-anchor="middle" font-family="Arial" font-size="14" fill="#666">
        QR Code
      </text>
    </svg>
  `)}`
}

const sendSMSCode = async () => {
  if (!phoneForm.phone) {
    ElMessage.warning('请输入手机号')
    return
  }
  
  try {
    // 模拟发送短信验证码
    await new Promise(resolve => setTimeout(resolve, 1000))
    ElMessage.success('验证码已发送')
    
    // 开始倒计时
    smsCountdown.value = 60
    const timer = setInterval(() => {
      smsCountdown.value--
      if (smsCountdown.value <= 0) {
        clearInterval(timer)
      }
    }, 1000)
  } catch (error) {
    ElMessage.error('发送失败: ' + error.message)
  }
}

// 组件挂载
onMounted(() => {
  // 检查是否已登录
  const token = localStorage.getItem('token')
  if (token) {
    router.push('/')
  }
  
  // 初始化二维码
  refreshQRCode()
})
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  position: relative;
  overflow: hidden;
  transition: all 0.3s ease;
}

/* 黑暗模式背景亮度降低 */
.dark .login-page {
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
  filter: brightness(0.7);
}

/* 主题切换按钮容器 */
.theme-toggle-container {
  position: fixed;
  top: 20px;
  right: 20px;
  z-index: 1000;
}

.theme-toggle-btn {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.15);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: white;
  transition: all 0.3s ease;
  display: flex;
  align-items: center;
  justify-content: center;
}

.theme-toggle-btn:hover {
  background: rgba(255, 255, 255, 0.25);
  transform: scale(1.05);
}

.dark .theme-toggle-btn {
  background: rgba(45, 45, 45, 0.8);
  border-color: rgba(255, 255, 255, 0.1);
}

.dark .theme-toggle-btn:hover {
  background: rgba(64, 64, 64, 0.9);
}

.login-container {
  width: 100%;
  max-width: 400px;
  padding: 20px;
  position: relative;
  z-index: 10;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
  color: white;
}

.logo {
  margin-bottom: 16px;
}

.login-header h1 {
  margin: 0 0 8px 0;
  font-size: 32px;
  font-weight: 600;
  text-shadow: 2px 2px 4px rgba(0, 0, 0, 0.3);
}

.login-header p {
  margin: 0;
  font-size: 16px;
  opacity: 0.9;
}

/* 玻璃态登录卡片容器 */
.login-card-wrapper {
  perspective: 1000px;
}

.login-card-glass {
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 20px;
  padding: 32px;
  box-shadow: 
    0 8px 32px rgba(0, 0, 0, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.2);
  transition: all 0.3s ease;
  transform-style: preserve-3d;
}

.login-card-glass:hover {
  transform: translateY(-2px);
  box-shadow: 
    0 12px 40px rgba(0, 0, 0, 0.15),
    inset 0 1px 0 rgba(255, 255, 255, 0.3);
}

/* 黑暗模式玻璃态卡片 */
.dark .login-card-glass {
  background: rgba(45, 45, 45, 0.3);
  border-color: rgba(255, 255, 255, 0.1);
  box-shadow: 
    0 8px 32px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}

.dark .login-card-glass:hover {
  box-shadow: 
    0 12px 40px rgba(0, 0, 0, 0.4),
    inset 0 1px 0 rgba(255, 255, 255, 0.15);
}

.card-header {
  text-align: center;
  font-size: 20px;
  font-weight: 600;
  color: white;
  margin-bottom: 24px;
  text-shadow: 1px 1px 2px rgba(0, 0, 0, 0.3);
}

.dark .card-header {
  color: #e5eaf3;
}

/* 表单样式优化 */
.login-card-glass :deep(.el-form-item) {
  margin-bottom: 20px;
}

.login-card-glass :deep(.el-input__wrapper) {
  background: rgba(255, 255, 255, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 12px;
  backdrop-filter: blur(10px);
  transition: all 0.3s ease;
}

.login-card-glass :deep(.el-input__wrapper):hover {
  background: rgba(255, 255, 255, 0.2);
  border-color: rgba(255, 255, 255, 0.3);
}

.login-card-glass :deep(.el-input__wrapper.is-focus) {
  background: rgba(255, 255, 255, 0.25);
  border-color: #409eff;
  box-shadow: 0 0 0 2px rgba(64, 158, 255, 0.2);
}

.login-card-glass :deep(.el-input__inner) {
  color: white;
  background: transparent;
}

.login-card-glass :deep(.el-input__inner::placeholder) {
  color: rgba(255, 255, 255, 0.6);
}

/* 黑暗模式输入框 */
.dark .login-card-glass :deep(.el-input__wrapper) {
  background: rgba(45, 45, 45, 0.6);
  border-color: rgba(255, 255, 255, 0.1);
}

.dark .login-card-glass :deep(.el-input__wrapper):hover {
  background: rgba(64, 64, 64, 0.7);
  border-color: rgba(255, 255, 255, 0.2);
}

.dark .login-card-glass :deep(.el-input__wrapper.is-focus) {
  background: rgba(64, 64, 64, 0.8);
  border-color: #79bbff;
  box-shadow: 0 0 0 2px rgba(121, 187, 255, 0.2);
}

.dark .login-card-glass :deep(.el-input__inner) {
  color: #e5eaf3;
}

.dark .login-card-glass :deep(.el-input__inner::placeholder) {
  color: #909399;
}

/* 按钮样式 */
.login-card-glass :deep(.el-button--primary) {
  background: linear-gradient(135deg, #409eff 0%, #79bbff 100%);
  border: none;
  border-radius: 12px;
  font-weight: 600;
  transition: all 0.3s ease;
  box-shadow: 0 4px 15px rgba(64, 158, 255, 0.3);
}

.login-card-glass :deep(.el-button--primary):hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 20px rgba(64, 158, 255, 0.4);
}

/* 复选框和链接 */
.login-options {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.login-card-glass :deep(.el-checkbox__label) {
  color: white;
}

.dark .login-card-glass :deep(.el-checkbox__label) {
  color: #e5eaf3;
}

.login-card-glass :deep(.el-link) {
  color: rgba(255, 255, 255, 0.8);
}

.login-card-glass :deep(.el-link):hover {
  color: white;
}

.dark .login-card-glass :deep(.el-link) {
  color: #79bbff;
}

/* 分割线 */
.login-footer {
  margin-top: 24px;
}

.login-card-glass :deep(.el-divider__text) {
  color: rgba(255, 255, 255, 0.7);
  background: transparent;
}

.dark .login-card-glass :deep(.el-divider__text) {
  color: #909399;
}

.login-card-glass :deep(.el-divider--horizontal) {
  border-top-color: rgba(255, 255, 255, 0.2);
}

.dark .login-card-glass :deep(.el-divider--horizontal) {
  border-top-color: rgba(255, 255, 255, 0.1);
}

/* 社交登录按钮 */
.social-login {
  display: flex;
  justify-content: center;
  gap: 16px;
}

.social-login :deep(.el-button) {
  background: rgba(255, 255, 255, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: white;
  transition: all 0.3s ease;
}

.social-login :deep(.el-button):hover {
  background: rgba(255, 255, 255, 0.25);
  transform: scale(1.05);
}

.dark .social-login :deep(.el-button) {
  background: rgba(45, 45, 45, 0.6);
  border-color: rgba(255, 255, 255, 0.1);
  color: #e5eaf3;
}

.dark .social-login :deep(.el-button):hover {
  background: rgba(64, 64, 64, 0.7);
}

.login-info {
  text-align: center;
  margin-top: 32px;
  color: rgba(255, 255, 255, 0.8);
  font-size: 14px;
}

.login-info p {
  margin: 4px 0;
}

.background-decoration {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  overflow: hidden;
}

.decoration-item {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  animation: float 6s ease-in-out infinite;
}

.decoration-item.item-1 {
  width: 200px;
  height: 200px;
  top: 10%;
  left: 10%;
  animation-delay: 0s;
}

.decoration-item.item-2 {
  width: 150px;
  height: 150px;
  top: 60%;
  right: 10%;
  animation-delay: 2s;
}

.decoration-item.item-3 {
  width: 100px;
  height: 100px;
  bottom: 20%;
  left: 20%;
  animation-delay: 4s;
}

.decoration-item.item-4 {
  width: 80px;
  height: 80px;
  top: 30%;
  right: 30%;
  animation-delay: 1s;
}

.decoration-item.item-5 {
  width: 120px;
  height: 120px;
  bottom: 40%;
  left: 60%;
  animation-delay: 3s;
}

/* 黑暗模式装饰元素 */
.dark .decoration-item {
  background: rgba(255, 255, 255, 0.05);
}

@keyframes float {
  0%, 100% {
    transform: translateY(0px) rotate(0deg);
  }
  50% {
    transform: translateY(-20px) rotate(180deg);
  }
}

/* 对话框样式 */
:deep(.el-dialog) {
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(20px);
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.2);
}

.dark :deep(.el-dialog) {
  background: rgba(45, 45, 45, 0.95);
  border-color: rgba(255, 255, 255, 0.1);
}

.dark :deep(.el-dialog__title) {
  color: #e5eaf3;
}

.dark :deep(.el-dialog__body) {
  color: #e5eaf3;
}

.qr-login-content {
  text-align: center;
}

.qr-code {
  margin-bottom: 16px;
  display: flex;
  justify-content: center;
}

.dark .qr-code {
  background-color: #ffffff;
  border-radius: 8px;
  padding: 10px;
}

.sms-container {
  display: flex;
  gap: 12px;
  align-items: center;
}

/* 主题下拉菜单样式 */
:deep(.el-dropdown-menu) {
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 12px;
}

.dark :deep(.el-dropdown-menu) {
  background: rgba(45, 45, 45, 0.95);
  border-color: rgba(255, 255, 255, 0.1);
}

:deep(.el-dropdown-menu__item) {
  color: #333;
  transition: all 0.3s ease;
}

.dark :deep(.el-dropdown-menu__item) {
  color: #e5eaf3;
}

:deep(.el-dropdown-menu__item:hover) {
  background: rgba(64, 158, 255, 0.1);
}

:deep(.el-dropdown-menu__item.is-active) {
  color: #409eff;
  font-weight: 600;
}

.dark :deep(.el-dropdown-menu__item.is-active) {
  color: #79bbff;
}

@media (max-width: 768px) {
  .login-container {
    padding: 16px;
  }
  
  .login-header h1 {
    font-size: 28px;
  }
  
  .login-card-glass {
    padding: 24px;
    border-radius: 16px;
  }
  
  .theme-toggle-container {
    top: 16px;
    right: 16px;
  }
  
  .theme-toggle-btn {
    width: 40px;
    height: 40px;
  }
  
  .login-options {
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
  }
  
  .sms-container {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
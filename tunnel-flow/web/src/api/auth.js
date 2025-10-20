import request from '@/utils/api'

// 用户登录
export const login = (data) => {
  return request.post('/auth/login', data)
}

// 用户登出
export const logout = () => {
  return request.post('/auth/logout')
}

// 获取用户信息
export const getUserInfo = () => {
  return request.get('/auth/user')
}

// 刷新token
export const refreshToken = () => {
  return request.post('/auth/refresh')
}



// 忘记密码
export const forgotPassword = (data) => {
  return request.post('/auth/forgot-password', data)
}

// 重置密码
export const resetPassword = (data) => {
  return request.post('/auth/reset-password', data)
}



// 手机号登录
export const phoneLogin = (data) => {
  return request.post('/auth/phone-login', data)
}

// 获取二维码
export const getQRCode = () => {
  return request({
    url: '/auth/qr-code',
    method: 'get'
  })
}

// 检查二维码状态
export const checkQRCode = (qrId) => {
  return request({
    url: `/auth/qr-code/${qrId}/status`,
    method: 'get'
  })
}
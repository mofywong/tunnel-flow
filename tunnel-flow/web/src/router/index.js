import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    component: () => import('@/layout/Layout.vue'),
    redirect: '/routes',
    children: []
  },
  {
    path: '/clients',
    component: () => import('@/layout/Layout.vue'),
    children: [
      {
        path: '',
        name: 'Clients',
        component: () => import('@/views/Clients.vue'),
        meta: {
          title: '客户端管理',
          icon: 'Monitor'
        }
      }
    ]
  },

  {
    path: '/routes',
    component: () => import('@/layout/Layout.vue'),
    children: [
      {
        path: '',
        name: 'Routes',
        component: () => import('@/views/Routes.vue'),
        meta: {
          title: '路由管理',
          icon: 'Connection'
        }
      }
    ]
  },



  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: {
      title: '登录',
      hideInMenu: true
    }
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/views/NotFound.vue'),
    meta: {
      title: '页面未找到',
      hideInMenu: true
    }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) {
      return savedPosition
    } else {
      return { top: 0 }
    }
  }
})

// 路由守卫 - 临时禁用用于测试
router.beforeEach((to, from, next) => {
  // 设置页面标题
  if (to.meta.title) {
    document.title = `${to.meta.title} - Tunnel Flow`
  }
  
  // 临时禁用认证逻辑用于测试
  /*
  // 认证逻辑
  const token = localStorage.getItem('token')
  if (!token && to.name !== 'Login') {
    next({ name: 'Login' })
    return
  }
  
  // 如果已登录且访问登录页，重定向到首页
  if (token && to.name === 'Login') {
    next({ name: 'Dashboard' })
    return
  }
  */
  
  next()
})

export default router
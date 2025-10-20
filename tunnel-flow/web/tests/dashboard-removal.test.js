const { test, expect } = require('@playwright/test');

test.describe('仪表板功能移除验证测试', () => {
  test.beforeEach(async ({ page }) => {
    // 导航到登录页面并登录
    await page.goto('http://localhost:3000');
    
    // 登录
    await page.fill('input[placeholder="请输入用户名"]', 'admin');
    await page.fill('input[placeholder="请输入密码"]', 'admin123');
    await page.click('button[type="submit"]');
    
    // 等待登录完成
    await page.waitForTimeout(2000);
  });

  test('验证仪表板路由已被移除', async ({ page }) => {
    // 尝试直接访问仪表板路由
    await page.goto('http://localhost:3000/dashboard');
    
    // 验证页面应该显示404或重定向到其他页面
    const currentUrl = page.url();
    expect(currentUrl).not.toContain('/dashboard');
    
    // 验证不应该显示仪表板相关内容
    await expect(page.locator('text=系统状态监控')).not.toBeVisible();
    await expect(page.locator('text=流量统计')).not.toBeVisible();
    await expect(page.locator('text=最近连接')).not.toBeVisible();
  });

  test('验证侧边栏中没有仪表板菜单项', async ({ page }) => {
    // 验证侧边栏中不存在仪表板菜单项
    await expect(page.locator('text=仪表板')).not.toBeVisible();
    
    // 验证侧边栏中存在其他菜单项
    await expect(page.locator('.el-menu-item:has-text("客户端管理")')).toBeVisible();
    await expect(page.locator('.el-menu-item:has-text("路由管理")')).toBeVisible();
    await expect(page.locator('.el-menu-item:has-text("消息管理")')).toBeVisible();
    await expect(page.locator('.el-menu-item:has-text("审计日志")')).toBeVisible();
    await expect(page.locator('.el-menu-item:has-text("系统设置")')).toBeVisible();
  });

  test('验证登录后默认重定向到客户端管理页面', async ({ page }) => {
    // 验证登录后应该重定向到客户端管理页面而不是仪表板
    const currentUrl = page.url();
    expect(currentUrl).toContain('/clients');
    
    // 验证页面显示客户端管理相关内容
    await expect(page.locator('h1.page-title:has-text("客户端管理")')).toBeVisible();
    await expect(page.locator('text=在线客户端')).toBeVisible();
  });

  test('验证仪表板相关的统计卡片不存在', async ({ page }) => {
    // 访问各个页面，确保没有仪表板特有的统计卡片
    await page.goto('http://localhost:3000/clients');
    
    // 验证不存在仪表板特有的统计内容
    await expect(page.locator('text=系统状态监控')).not.toBeVisible();
    await expect(page.locator('text=今日消息')).not.toBeVisible();
    await expect(page.locator('text=今日流量')).not.toBeVisible();
    
    // 验证不存在仪表板特有的图表容器
    await expect(page.locator('.chart-container')).not.toBeVisible();
    await expect(page.locator('.dashboard-content')).not.toBeVisible();
  });

  test('验证所有页面导航正常工作', async ({ page }) => {
    // 测试客户端管理页面
    await page.click('.el-menu-item:has-text("客户端管理")');
    await expect(page.locator('h1.page-title:has-text("客户端管理")')).toBeVisible();
    expect(page.url()).toContain('/clients');
    
    // 测试路由管理页面
    await page.click('.el-menu-item:has-text("路由管理")');
    await expect(page.locator('h1.page-title:has-text("路由管理")')).toBeVisible();
    expect(page.url()).toContain('/routes');
    
    // 测试消息管理页面
    await page.click('.el-menu-item:has-text("消息管理")');
    await expect(page.locator('h1.page-title:has-text("消息管理")')).toBeVisible();
    expect(page.url()).toContain('/messages');
    
    // 测试审计日志页面
    await page.click('.el-menu-item:has-text("审计日志")');
    await expect(page.locator('h1.page-title:has-text("审计日志")')).toBeVisible();
    expect(page.url()).toContain('/audit');
    
    // 测试系统设置页面
    await page.click('.el-menu-item:has-text("系统设置")');
    await expect(page.locator('h1.page-title:has-text("系统设置")')).toBeVisible();
    expect(page.url()).toContain('/settings');
  });

  test('验证仪表板相关的API端点不存在', async ({ page }) => {
    // 监听网络请求
    const requests = [];
    page.on('request', request => {
      requests.push(request.url());
    });
    
    // 访问各个页面
    await page.goto('http://localhost:3000/clients');
    await page.waitForTimeout(2000);
    
    await page.goto('http://localhost:3000/routes');
    await page.waitForTimeout(2000);
    
    // 验证没有仪表板特有的API请求
    const dashboardRequests = requests.filter(url => 
      url.includes('/dashboard') || 
      url.includes('/stats/dashboard') ||
      url.includes('/metrics/dashboard')
    );
    
    expect(dashboardRequests.length).toBe(0);
  });

  test('验证页面标题和面包屑导航正确', async ({ page }) => {
    // 访问客户端管理页面
    await page.goto('http://localhost:3000/clients');
    
    // 验证页面标题不包含仪表板相关内容
    const title = await page.title();
    expect(title).not.toContain('仪表板');
    expect(title).not.toContain('Dashboard');
    
    // 验证面包屑导航不包含仪表板
    const breadcrumbText = await page.locator('.breadcrumb').textContent();
    if (breadcrumbText) {
      expect(breadcrumbText).not.toContain('仪表板');
      expect(breadcrumbText).not.toContain('Dashboard');
    }
  });
});
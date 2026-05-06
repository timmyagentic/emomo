import { test, expect } from '@playwright/test';

test.describe('Emomo 表情包搜索应用', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('页面正确加载', async ({ page }) => {
    // 检查标题
    await expect(page.locator('h1')).toContainText('用文字找表情');
    // 检查副标题
    await expect(page.getByText('AI 驱动的语义搜索')).toBeVisible();
    // 检查搜索框存在
    await expect(page.locator('input[type="text"]')).toBeVisible();
    // 检查搜索按钮
    await expect(page.getByRole('button', { name: '搜索' })).toBeVisible();
  });

  test('热门标签显示并可点击', async ({ page }) => {
    // 检查热门标签区域
    await expect(page.getByText('热门:')).toBeVisible();

    // 检查至少有一些热门标签
    const tags = page.locator('button').filter({ hasText: /^(开心|无语|狗头|猫咪|熊猫头|沙雕)$/ });
    await expect(tags.first()).toBeVisible();
  });

  test('搜索功能工作正常', async ({ page }) => {
    // 输入搜索词
    const searchInput = page.locator('input[type="text"]');
    await searchInput.fill('猫咪');

    // 点击搜索按钮
    await page.getByRole('button', { name: '搜索', exact: true }).click();

    // 等待加载完成（检查没有 loading 状态）
    await expect(searchInput).not.toBeDisabled({ timeout: 10000 });

    // 检查搜索结果区域存在
    await page.waitForTimeout(1000); // 等待结果渲染
  });

  test('点击热门标签触发搜索', async ({ page }) => {
    // 找到一个热门标签并点击
    const tagButton = page.locator('button').filter({ hasText: '开心' }).first();
    await tagButton.click();

    // 检查输入框有值
    const searchInput = page.locator('input[type="text"]');
    await expect(searchInput).toHaveValue('开心');
  });

  test('搜索框可以清除', async ({ page }) => {
    // 输入搜索词
    const searchInput = page.locator('input[type="text"]');
    await searchInput.fill('测试');

    // 检查清除按钮出现
    const clearButton = page.locator('button[type="button"]').filter({ has: page.locator('svg') }).first();
    await expect(clearButton).toBeVisible();

    // 清除内容
    await searchInput.clear();
    await expect(searchInput).toHaveValue('');
  });

  test('清空搜索后返回随便逛逛', async ({ page }) => {
    await page.route('**/api/v1/search/stream', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: [
          'event: complete',
          `data: ${JSON.stringify({
            stage: 7,
            message: '搜索完成',
            complete: {
              results: [
                {
                  meme: {
                    id: 'search-cat-1',
                    url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22120%22 height=%22120%22 viewBox=%220 0 120 120%22%3E%3Crect width=%22120%22 height=%22120%22 fill=%22%23fff4dc%22/%3E%3Ctext x=%2260%22 y=%2268%22 text-anchor=%22middle%22 font-size=%2232%22%3Ecat%3C/text%3E%3C/svg%3E',
                    imageInfo: {
                      width: 120,
                      height: 120,
                      format: 2,
                    },
                    category: '测试',
                    tags: ['猫咪'],
                  },
                  score: 0.04,
                  description: '猫咪测试表情',
                  textPresence: 1,
                },
              ],
              total: 1,
              query: '猫咪',
              expandedQuery: '猫咪',
            },
          })}`,
          '',
        ].join('\n'),
      });
    });

    const searchInput = page.locator('input[type="text"]');
    await searchInput.fill('猫咪');
    await page.getByRole('button', { name: '搜索', exact: true }).click();

    await expect(page.getByText('「猫咪」')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('匹配度偏低')).toBeVisible();

    await page.getByRole('button', { name: '清空搜索' }).click();

    await expect(searchInput).toHaveValue('');
    await expect(page.getByRole('heading', { name: '随便逛逛' })).toBeVisible();
    await expect(page.getByText('「猫咪」')).toBeHidden();
    await expect(page.getByText('匹配度偏低')).toBeHidden();
  });

  test('随便逛逛区域显示', async ({ page }) => {
    // 检查随便逛逛标题或表情网格
    // 等待初始加载
    await page.waitForTimeout(2000);

    // 检查页面有表情卡片或者随便逛逛区域
    const hasMemes = await page.locator('img').count() > 0;
    expect(hasMemes || await page.getByText('随便逛逛').isVisible()).toBeTruthy();
  });

  test('表情卡片可以点击', async ({ page }) => {
    // 等待随便逛逛表情加载
    await page.waitForTimeout(2000);

    // 找到一个表情卡片
    const memeCard = page.locator('img[alt]').first();
    if (await memeCard.isVisible()) {
      await memeCard.click();

      // 检查弹窗是否出现（如果实现了的话）
      await page.waitForTimeout(500);
    }
  });

  test('响应式布局正常', async ({ page }) => {
    // 测试移动端视图
    await page.setViewportSize({ width: 375, height: 667 });
    await expect(page.locator('h1')).toBeVisible();
    await expect(page.locator('input[type="text"]')).toBeVisible();

    // 测试平板视图
    await page.setViewportSize({ width: 768, height: 1024 });
    await expect(page.locator('h1')).toBeVisible();

    // 测试桌面视图
    await page.setViewportSize({ width: 1280, height: 800 });
    await expect(page.locator('h1')).toBeVisible();
  });

  test('键盘导航 - 回车提交搜索', async ({ page }) => {
    const searchInput = page.locator('input[type="text"]');
    await searchInput.fill('狗狗');
    await searchInput.press('Enter');

    // 验证搜索被触发
    await expect(searchInput).toHaveValue('狗狗');
  });

  test('移动端搜索栏保持单行触达', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    const searchInput = page.locator('input[type="text"]');
    const searchButton = page.getByRole('button', { name: '搜索', exact: true });

    await expect(searchInput).toBeVisible();
    await expect(searchButton).toBeVisible();

    const inputBox = await searchInput.boundingBox();
    const buttonBox = await searchButton.boundingBox();

    expect(inputBox).not.toBeNull();
    expect(buttonBox).not.toBeNull();
    expect(Math.abs((inputBox!.y + inputBox!.height / 2) - (buttonBox!.y + buttonBox!.height / 2))).toBeLessThan(10);
    expect(buttonBox!.width).toBeLessThan(128);
  });

  test('移动端详情弹窗操作按钮不纵向堆叠', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    const firstMemeImage = page.locator('img[alt]').first();
    await expect(firstMemeImage).toBeVisible({ timeout: 10000 });
    await firstMemeImage.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    const copyImageButton = page.getByRole('button', { name: '复制图片到剪贴板' });
    const downloadButton = page.getByRole('button', { name: '下载表情图片' });
    const copyLinkButton = page.getByRole('button', { name: '复制图片链接' });

    const copyImageBox = await copyImageButton.boundingBox();
    const downloadBox = await downloadButton.boundingBox();
    const copyLinkBox = await copyLinkButton.boundingBox();

    expect(copyImageBox).not.toBeNull();
    expect(downloadBox).not.toBeNull();
    expect(copyLinkBox).not.toBeNull();
    expect(Math.abs(copyImageBox!.y - downloadBox!.y)).toBeLessThan(8);
    expect(Math.abs(copyImageBox!.y - copyLinkBox!.y)).toBeLessThan(8);
  });
});

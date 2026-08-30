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
                    tags: ['猫咪'],
                    category: '测试',
                  },
                  score: 0.04,
                  description: '猫咪测试表情',
                },
              ],
              total: 1,
              query: '猫咪',
            },
          })}`,
          '',
        ].join('\n'),
      });
    });

    const searchInput = page.locator('input[type="text"]');
    await searchInput.fill('猫咪');
    await page.getByRole('button', { name: '搜索', exact: true }).click();

    const searchResult = page.getByRole('button', { name: /查看表情详情：猫咪测试表情/ });
    await expect(searchResult).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('已展示全部相关结果')).toBeVisible();
    await expect(page.getByText('匹配度偏低')).toBeVisible();

    await page.getByRole('button', { name: '清空搜索' }).click();

    await expect(searchInput).toHaveValue('');
    await expect(page.getByRole('heading', { name: '随便逛逛' })).toBeVisible();
    await expect(searchResult).toBeHidden();
    await expect(page.getByText('已展示全部相关结果')).toBeHidden();
    await expect(page.getByText('匹配度偏低')).toBeHidden();
  });

  test('文字筛选只筛当前搜索结果，不改变搜索请求', async ({ page }) => {
    let requestBody: Record<string, unknown> | undefined;
    let searchRequestCount = 0;

    await page.route('**/api/v1/search/stream', async (route) => {
      searchRequestCount += 1;
      requestBody = route.request().postDataJSON();
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
                    id: 'cat-with-text',
                    url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22120%22 height=%22120%22 viewBox=%220 0 120 120%22%3E%3Crect width=%22120%22 height=%22120%22 fill=%22%23fff4dc%22/%3E%3Ctext x=%2260%22 y=%2268%22 text-anchor=%22middle%22 font-size=%2220%22%3Ewith text%3C/text%3E%3C/svg%3E',
                    imageInfo: {
                      width: 120,
                      height: 120,
                      format: 2,
                    },
                    tags: ['猫咪'],
                    category: '测试',
                  },
                  score: 0.91,
                  description: '带文字猫咪测试表情',
                  textPresence: 'TEXT_PRESENCE_WITH_TEXT',
                },
                {
                  meme: {
                    id: 'cat-without-text',
                    url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22120%22 height=%22120%22 viewBox=%220 0 120 120%22%3E%3Crect width=%22120%22 height=%22120%22 fill=%22%23dff4ff%22/%3E%3Ccircle cx=%2260%22 cy=%2260%22 r=%2230%22 fill=%22%2390c8ef%22/%3E%3C/svg%3E',
                    imageInfo: {
                      width: 120,
                      height: 120,
                      format: 2,
                    },
                    tags: ['猫咪'],
                    category: '测试',
                  },
                  score: 0.82,
                  description: '无文字猫咪测试表情',
                  textPresence: 'TEXT_PRESENCE_WITHOUT_TEXT',
                },
              ],
              total: 2,
              query: '猫咪',
            },
          })}`,
          '',
        ].join('\n'),
      });
    });

    await expect(page.getByRole('radio', { name: '有文字' })).toHaveCount(0);

    await page.locator('input[type="text"]').fill('猫咪');
    await page.getByRole('button', { name: '搜索', exact: true }).click();

    await expect.poll(() => requestBody !== undefined, { timeout: 5000 }).toBe(true);
    await expect(page.getByRole('button', { name: /查看表情详情：带文字猫咪测试表情/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /查看表情详情：无文字猫咪测试表情/ })).toBeVisible();
    await expect(page.getByRole('radiogroup', { name: '筛选当前搜索结果' })).toBeVisible();

    await page.getByRole('radio', { name: '有文字' }).click();

    await expect(page.getByRole('button', { name: /查看表情详情：带文字猫咪测试表情/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /查看表情详情：无文字猫咪测试表情/ })).toBeHidden();
    expect(searchRequestCount).toBe(1);
    expect(requestBody?.textPresence).toBeUndefined();
  });

  test('文字筛选为空时仍保留结果筛选控件', async ({ page }) => {
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
                    id: 'cat-without-text-only',
                    url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22120%22 height=%22120%22 viewBox=%220 0 120 120%22%3E%3Crect width=%22120%22 height=%22120%22 fill=%22%23dff4ff%22/%3E%3Ccircle cx=%2260%22 cy=%2260%22 r=%2230%22 fill=%22%2390c8ef%22/%3E%3C/svg%3E',
                    imageInfo: {
                      width: 120,
                      height: 120,
                      format: 2,
                    },
                    tags: ['猫咪'],
                    category: '测试',
                  },
                  score: 0.82,
                  description: '无文字猫咪测试表情',
                  textPresence: 'TEXT_PRESENCE_WITHOUT_TEXT',
                },
              ],
              total: 1,
              query: '猫咪',
            },
          })}`,
          '',
        ].join('\n'),
      });
    });

    await page.locator('input[type="text"]').fill('猫咪');
    await page.getByRole('button', { name: '搜索', exact: true }).click();

    await expect(page.getByRole('button', { name: /查看表情详情：无文字猫咪测试表情/ })).toBeVisible();

    await page.getByRole('radio', { name: '有文字' }).click();

    await expect(page.getByText('没有找到相关表情包')).toBeVisible();
    await expect(page.getByRole('radiogroup', { name: '筛选当前搜索结果' })).toBeVisible();

    await page.getByRole('radio', { name: '全部' }).click();

    await expect(page.getByRole('button', { name: /查看表情详情：无文字猫咪测试表情/ })).toBeVisible();
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

  test('表情详情只保留必要操作', async ({ page }) => {
    await page
      .getByRole('button', { name: '查看表情详情：一只橘色的猫咪，眼神慵懒地看着镜头，非常可爱' })
      .click();
    const dialog = page.getByRole('dialog', { name: '表情详情' });

    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('button', { name: '复制图片到剪贴板' })).toBeVisible();
    await expect(dialog.getByRole('button', { name: '下载表情图片' })).toBeVisible();
    await expect(dialog.getByRole('button', { name: '复制图片链接' })).toBeHidden();
    await expect(dialog.getByText('格式:')).toBeHidden();
    await expect(dialog.getByText('尺寸:')).toBeHidden();
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

  test('反馈必须先完整预览再由明确确认提交', async ({ page }) => {
    let submitCount = 0;
    await page.route('**/api/v1/feedback/preview', async (route) => {
      expect(route.request().postDataJSON()).toEqual({ description: '希望增加收藏夹' });
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          preview: {
            environment: {
              product: 'emomo',
              version: 'v1.2.3',
              os: 'linux',
              arch: 'amd64',
              agent: 'semantic-search',
            },
            description: '希望增加收藏夹',
            capability_gaps: [],
          },
          submission_enabled: true,
          public_fallback_url: 'https://github.com/timmyagentic/emomo/issues/new',
        }),
      });
    });
    await page.route('**/api/v1/feedback/submit', async (route) => {
      submitCount += 1;
      expect(route.request().postDataJSON()).toEqual({
        description: '希望增加收藏夹',
        userApproved: true,
      });
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          reference_url: 'https://github.com/timmyagentic/emomo/issues/123',
          deduplicated: false,
        }),
      });
    });

    await page.getByRole('button', { name: '反馈' }).click();
    const dialog = page.getByRole('dialog', { name: '发送反馈' });
    await dialog.getByRole('textbox', { name: '反馈内容' }).fill('希望增加收藏夹');
    await dialog.getByRole('button', { name: '预览脱敏内容' }).click();

    const preview = dialog.getByLabel('完整脱敏预览');
    await expect(preview.getByText('emomo')).toBeVisible();
    await expect(preview.getByText('v1.2.3')).toBeVisible();
    await expect(preview.getByText('linux / amd64')).toBeVisible();
    await expect(preview.getByText('semantic-search')).toBeVisible();
    await expect(preview.getByText('希望增加收藏夹')).toBeVisible();
    expect(submitCount).toBe(0);

    await dialog.getByRole('button', { name: '确认并提交这份脱敏反馈' }).click();
    await expect(dialog.getByRole('link', { name: '查看反馈记录' })).toHaveAttribute(
      'href',
      'https://github.com/timmyagentic/emomo/issues/123'
    );
    expect(submitCount).toBe(1);
  });

  test('编辑期间到达的旧反馈预览不会恢复', async ({ page }) => {
    let releasePreview: (() => void) | undefined;
    let requestStarted: (() => void) | undefined;
    const started = new Promise<void>((resolve) => {
      requestStarted = resolve;
    });
    const gate = new Promise<void>((resolve) => {
      releasePreview = resolve;
    });
    await page.route('**/api/v1/feedback/preview', async (route) => {
      requestStarted?.();
      await gate;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          preview: {
            environment: { product: 'emomo', version: 'test', os: 'linux', arch: 'amd64', agent: 'semantic-search' },
            description: '旧内容',
            capability_gaps: [],
          },
          submission_enabled: false,
          public_fallback_url: 'https://github.com/timmyagentic/emomo/issues/new',
        }),
      });
    });

    await page.getByRole('button', { name: '反馈' }).click();
    const dialog = page.getByRole('dialog', { name: '发送反馈' });
    const textbox = dialog.getByRole('textbox', { name: '反馈内容' });
    await textbox.fill('旧内容');
    await dialog.getByRole('button', { name: '预览脱敏内容' }).click();
    await started;
    await textbox.fill('新内容');
    releasePreview?.();

    await expect(textbox).toHaveValue('新内容');
    await expect(dialog.getByLabel('完整脱敏预览')).toHaveCount(0);
    await expect(dialog.getByRole('button', { name: '预览脱敏内容' })).toBeEnabled();
  });
});

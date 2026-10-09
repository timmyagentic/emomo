for (const button of document.querySelectorAll('[data-copy]')) {
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(document.getElementById(button.dataset.copy).textContent);
      button.textContent = '已复制 ✓';
      document.getElementById('copy-status').textContent = '安装 Prompt 已复制';
      setTimeout(() => { button.textContent = '复制安装 Prompt'; }, 2000);
    } catch {
      button.textContent = '请手动选择 Prompt';
      document.getElementById('copy-status').textContent = '未能访问剪贴板，请选择并复制安装 Prompt';
    }
  });
}

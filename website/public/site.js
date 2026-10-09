for (const button of document.querySelectorAll('[data-copy]')) {
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(document.getElementById(button.dataset.copy).textContent);
      button.textContent = '已复制 ✓';
      document.getElementById('copy-status').textContent = '安装命令已复制';
      setTimeout(() => { button.textContent = '复制命令'; }, 2000);
    } catch {
      button.textContent = '请手动选择命令';
      document.getElementById('copy-status').textContent = '未能访问剪贴板，请选择并复制命令';
    }
  });
}

const menu = document.querySelector('.menu-toggle');
const nav = document.getElementById('site-navigation');
if (menu && nav) {
  const close = () => {
    menu.setAttribute('aria-expanded', 'false');
    nav.classList.remove('is-open');
  };
  menu.addEventListener('click', () => {
    const open = menu.getAttribute('aria-expanded') !== 'true';
    menu.setAttribute('aria-expanded', String(open));
    nav.classList.toggle('is-open', open);
  });
  nav
    .querySelectorAll('a')
    .forEach((link) => link.addEventListener('click', close));
  document.addEventListener('keydown', (event) => {
    if (
      event.key === 'Escape' &&
      menu.getAttribute('aria-expanded') === 'true'
    ) {
      close();
      menu.focus();
    }
  });
}
for (const button of document.querySelectorAll('[data-copy]')) {
  const label = button.textContent;
  button.addEventListener('click', async () => {
    const target = document.getElementById(button.dataset.copy);
    const status = document.getElementById('copy-status');
    try {
      await navigator.clipboard.writeText(target.textContent);
      button.textContent = '已复制';
      if (status) status.textContent = '安装 Prompt 已复制';
      setTimeout(() => {
        button.textContent = label;
      }, 2000);
    } catch {
      const range = document.createRange();
      range.selectNodeContents(target);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      button.textContent = '已选中，请手动复制';
      if (status)
        status.textContent = '未能访问剪贴板，已选中安装 Prompt，请手动复制';
    }
  });
}

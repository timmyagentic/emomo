const $ = id => document.getElementById(id);
let challenge = '', email = '', freshKey = '', resendAt = 0, timer;
function status(message = '', error = false) { $('status').textContent = message; $('status').classList.toggle('error', error); }
async function api(path, body) {
  const r = await fetch('/api/account/' + path, { method: body === undefined ? 'GET' : 'POST', credentials: 'same-origin', headers: body === undefined ? {} : { 'content-type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), cache: 'no-store' });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) { const e = new Error(data.error || '服务暂不可用，请稍后再试。'); e.status = r.status; throw e; } return data;
}
async function busy(button, fn) { button.disabled = true; status(); try { await fn(); } catch (e) { status(e.message, true); } finally { if (button.id === 'resend') countdown(); else button.disabled = false; } }
function clearKey() { freshKey = ''; $('key-value').value = ''; $('new-key').hidden = true; }
function signedOut() { clearKey(); $('dashboard').hidden = true; $('signin').hidden = false; }
function text(tag, value, className) { const el = document.createElement(tag); el.textContent = value; if (className) el.className = className; return el; }
function render(data) {
  $('signin').hidden = true; $('dashboard').hidden = false; $('account-email').textContent = data.email;
  $('quota-cards').replaceChildren();
  for (const [op, title] of [['search', '搜索'], ['read', '详情读取'], ['download', '原图下载']]) {
    const q = data.usage[op], card = text('article', '', 'quota-card');
    card.append(text('h3', title), text('strong', String(q.dayRemaining)), text('span', ` / ${q.dayLimit} 次今日剩余`), text('p', `本小时剩余 ${q.hourRemaining} / ${q.hourLimit} 次`)); $('quota-cards').append(card);
  }
  $('keys').replaceChildren();
  if (!data.keys.length) $('keys').append(text('p', '还没有密钥。给它起个名字，创建你的第一把。', 'empty muted'));
  for (const key of data.keys) {
    const row = text('article', '', 'key-row' + (Number(key.revokedAt) > 0 ? ' revoked' : '')), info = document.createElement('div');
    const title = text('h3', key.name); title.append(text('span', Number(key.revokedAt) > 0 ? '已撤销' : '可使用', 'badge'));
    info.append(title, text('p', `…${key.id.slice(-8)} · 创建于 ${new Date(Number(key.createdAt)).toLocaleDateString('zh-CN')}`)); row.append(info);
    if (!Number(key.revokedAt)) {
      const button = text('button', '撤销', 'text-button revoke'); button.type = 'button'; button.setAttribute('aria-label', `撤销 ${key.name}`);
      button.addEventListener('click', () => { if (!confirm(`撤销“${key.name}”后，使用它的 CLI 将立即失效。确定撤销？`)) return; busy(button, async () => { await api(`keys/${key.id}/revoke`, {}); clearKey(); await refresh(); status('密钥已撤销。'); }); }); row.append(button);
    }
    $('keys').append(row);
  }
}
async function refresh() { render(await api('me')); }
function countdown() { clearTimeout(timer); const seconds = Math.max(0, Math.ceil((resendAt - Date.now()) / 1000)); $('resend').disabled = seconds > 0; $('resend').textContent = seconds ? `${seconds} 秒后重新发送` : '重新发送验证码'; if (seconds) timer = setTimeout(countdown, 1000); }
async function send() {
  const result = await api('code', { email }); challenge = result.challenge; resendAt = Date.now() + result.retryAfter * 1000;
  $('sent-email').textContent = email; $('email-form').hidden = true; $('code-form').hidden = false; $('code').value = ''; countdown(); $('code').focus(); status('验证码已发送，请查看邮箱。');
}
$('email-form').addEventListener('submit', e => { e.preventDefault(); email = $('email').value.trim(); busy(e.submitter, send); });
$('code-form').addEventListener('submit', e => { e.preventDefault(); busy(e.submitter, async () => { await api('verify', { email, challenge, code: $('code').value }); $('code').value = ''; clearTimeout(timer); await refresh(); status('登录成功。现在可以创建密钥了。'); }); });
$('resend').addEventListener('click', () => busy($('resend'), send));
$('change-email').addEventListener('click', () => { clearTimeout(timer); challenge = ''; $('code').value = ''; $('code-form').hidden = true; $('email-form').hidden = false; $('email').focus(); status(); });
$('logout').addEventListener('click', () => busy($('logout'), async () => { await api('logout', {}); signedOut(); $('code-form').hidden = true; $('email-form').hidden = false; status('已退出登录。'); }));
$('refresh').addEventListener('click', () => busy($('refresh'), async () => { await refresh(); status('额度已更新。'); }));
$('key-form').addEventListener('submit', e => { e.preventDefault(); busy(e.submitter, async () => {
  if (freshKey && !confirm('当前新密钥尚未关闭。确定已经保存并继续创建？')) return;
  clearKey(); const result = await api('keys', { name: $('key-name').value.trim() });
  freshKey = result.token; $('key-value').value = freshKey; $('new-key').hidden = false; $('key-name').value = '';
  await refresh(); status('密钥已创建，完整内容只显示这一次。'); $('key-value').focus();
}); });
$('copy-key').addEventListener('click', () => busy($('copy-key'), async () => { await navigator.clipboard.writeText(freshKey); status('密钥已复制。请只粘贴到你自己的安全配置中。'); }));
$('download-key').addEventListener('click', () => {
  if (!freshKey) return; const url = URL.createObjectURL(new Blob([freshKey + '\n'], { type: 'text/plain' })); const a = document.createElement('a'); a.href = url; a.download = 'emomo-access.key'; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); status('密钥文件已下载。请按下方说明配置 CLI。');
});
$('dismiss-key').addEventListener('click', () => { clearKey(); status(); });
refresh().then(() => status()).catch(e => { signedOut(); status(e.status === 401 ? '' : e.message, e.status !== 401); });

window.addEventListener('pagehide', clearKey);

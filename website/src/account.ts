import { DurableObject } from 'cloudflare:workers';

const HOUR = 3_600_000, DAY = 24 * HOUR;
export const SESSION_MS = 14 * DAY;
export const random = (bytes: number) => [...crypto.getRandomValues(new Uint8Array(bytes))].map(b => b.toString(16).padStart(2, '0')).join('');
export const digest = async (text: string) => [...new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)))].map(b => b.toString(16).padStart(2, '0')).join('');
type Code = { challenge: string; hash: string; expires: number; attempts: number; email: string };
type Key = { id: string; name: string; hash: string; createdAt: number; revokedAt?: number };
type State = { email?: string; createdAt?: number; code?: Code; lastCode?: number; pendingCode?: { challenge: string; expires: number }; sessions: Record<string, number>; keys: Key[]; keyDay?: number; keyCount?: number };
const empty = (): State => ({ sessions: {}, keys: [] });
const sessionOK = (s: State, hash: string) => !!s.email && (s.sessions[hash] ?? 0) > Date.now();
const publicKeys = (s: State) => s.keys.map(({ hash: _, ...key }) => key).sort((a, b) => b.createdAt - a.createdAt);

export class Account extends DurableObject {
  async reserve(hourLimit: number, dayLimit: number): Promise<number> {
    return this.ctx.storage.transaction(async tx => {
      const now = Date.now(), hour = Math.floor(now / HOUR), day = Math.floor(now / DAY);
      const s = await tx.get<{ hour: number; day: number; hourly: number; daily: number }>('budget') ?? { hour, day, hourly: 0, daily: 0 };
      if (s.hour !== hour) { s.hour = hour; s.hourly = 0; }
      if (s.day !== day) { s.day = day; s.daily = 0; }
      if (s.daily >= dayLimit) return Math.ceil(((day + 1) * DAY - now) / 1000);
      if (s.hourly >= hourLimit) return Math.ceil(((hour + 1) * HOUR - now) / 1000);
      s.hourly++; s.daily++; await tx.put('budget', s); await tx.setAlarm((day + 1) * DAY + 60_000); return 0;
    });
  }
  // Claim before touching cross-object send budgets. A bounded lease recovers
  // abandoned requests without replacing a still-usable verification code.
  async claimCode(challenge: string): Promise<number> {
    return this.ctx.storage.transaction(async tx => {
      const s = await tx.get<State>('account') ?? empty(), now = Date.now();
      const until = Math.max((s.lastCode ?? 0) + 60_000, s.pendingCode?.expires ?? 0);
      if (until > now) return Math.ceil((until - now) / 1000);
      s.pendingCode = { challenge, expires: now + 60_000 };
      await tx.put('account', s); if (!s.email && !s.code) await tx.setAlarm(now + 660_000);
      return 0;
    });
  }
  async releaseCode(challenge: string) {
    await this.ctx.storage.transaction(async tx => {
      const s = await tx.get<State>('account');
      if (s?.pendingCode?.challenge === challenge) { delete s.pendingCode; await tx.put('account', s); }
    });
  }
  async beginCode(email: string, challenge: string, hash: string): Promise<boolean> {
    return this.ctx.storage.transaction(async tx => {
      const s = await tx.get<State>('account'), now = Date.now();
      // Fence requests that resume after their lease has expired or been replaced.
      if (s?.pendingCode?.challenge !== challenge || s.pendingCode.expires <= now) return false;
      delete s.pendingCode;
      s.lastCode = now; s.code = { email, challenge, hash, expires: now + 600_000, attempts: 0 };
      await tx.put('account', s); if (!s.email) await tx.setAlarm(now + 660_000); return true;
    });
  }
  async cancelCode(challenge: string) {
    await this.ctx.storage.transaction(async tx => { const s = await tx.get<State>('account'); if (s?.code?.challenge === challenge) { delete s.code; await tx.put('account', s); } });
  }
  async verify(challenge: string, hash: string, sessionHash: string): Promise<boolean> {
    return this.ctx.storage.transaction(async tx => {
      const s = await tx.get<State>('account'); const now = Date.now();
      if (!s?.code || s.code.challenge !== challenge || s.code.expires <= now || s.code.attempts >= 5) return false;
      s.code.attempts++;
      if (s.code.hash !== hash) { await tx.put('account', s); return false; }
      s.email = s.code.email; s.createdAt ??= now; delete s.code;
      s.sessions = Object.fromEntries(Object.entries(s.sessions).filter(([, expiry]) => expiry > now).sort((a, b) => b[1] - a[1]).slice(0, 4));
      s.sessions[sessionHash] = now + SESSION_MS; await tx.put('account', s); await tx.setAlarm(now + SESSION_MS); return true;
    });
  }
  async profile(sessionHash: string) {
    const s = await this.ctx.storage.get<State>('account');
    return s && sessionOK(s, sessionHash) ? { email: s.email!, createdAt: s.createdAt!, keys: publicKeys(s) } : null;
  }
  async logout(sessionHash: string) {
    await this.ctx.storage.transaction(async tx => { const s = await tx.get<State>('account'); if (s) { delete s.sessions[sessionHash]; await tx.put('account', s); } });
  }
  async createKey(sessionHash: string, accountId: string, name: string) {
    return this.ctx.storage.transaction(async tx => {
      const s = await tx.get<State>('account'); if (!s || !sessionOK(s, sessionHash)) return { error: 'unauthorized' };
      const now = Date.now(), day = Math.floor(now / DAY);
      if (s.keys.filter(k => !k.revokedAt).length >= 3) return { error: 'active-limit' };
      if (s.keyDay !== day) { s.keyDay = day; s.keyCount = 0; }
      if ((s.keyCount ?? 0) >= 10) return { error: 'daily-limit' };
      const id = random(8), token = `emomo_web-${accountId}-${id}_${random(32)}`;
      const key = { id, name, hash: await digest(token), createdAt: now };
      s.keys = [...s.keys.filter(k => !k.revokedAt), ...s.keys.filter(k => k.revokedAt && k.revokedAt > now - 30 * DAY).sort((a, b) => b.createdAt - a.createdAt).slice(0, 10)];
      s.keys.push(key); s.keyCount = (s.keyCount ?? 0) + 1; await tx.put('account', s);
      return { token, key: { id, name, createdAt: now } };
    });
  }
  async revokeKey(sessionHash: string, id: string): Promise<boolean> {
    return this.ctx.storage.transaction(async tx => {
      const s = await tx.get<State>('account'); if (!s || !sessionOK(s, sessionHash)) return false;
      const key = s.keys.find(k => k.id === id); if (!key) return false;
      key.revokedAt ??= Date.now(); await tx.put('account', s); return true;
    });
  }
  async authenticate(token: string): Promise<boolean> {
    const match = /^emomo_web-[a-f0-9]{24}-([a-f0-9]{16})_[a-f0-9]{64}$/.exec(token); if (!match) return false;
    const s = await this.ctx.storage.get<State>('account');
    const key = s?.keys.find(k => k.id === match[1] && !k.revokedAt);
    return !!key && key.hash === await digest(token);
  }
  async alarm() {
    const s = await this.ctx.storage.get<State>('account');
    if (!s?.email) { await this.ctx.storage.deleteAll(); return; }
    const now = Date.now();
    s.sessions = Object.fromEntries(Object.entries(s.sessions).filter(([, expiry]) => expiry > now));
    if (s.code && s.code.expires <= now) delete s.code;
    await this.ctx.storage.put('account', s);
    if (Object.keys(s.sessions).length) await this.ctx.storage.setAlarm(Math.max(...Object.values(s.sessions)) + 1);
  }
}

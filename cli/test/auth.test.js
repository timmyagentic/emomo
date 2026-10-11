import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, writeFile, readFile, stat, chmod, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configureAccess, readAccess, accessStatus, clearAccess } from '../src/auth.js';
import { run } from '../src/cli.js';
import { EmomoClient, DEFAULT_API_URL } from '../src/client.js';
const token = 'emomo_fixture_' + 'a'.repeat(64);
test('private auth configuration binds its endpoint, keeps secrets out of output and preserves local selection', async () => {
 const dir = await mkdtemp(join(tmpdir(),'emomo-auth-'));
 const env = { EMOMO_CONFIG_DIR: dir };
 try {
  const file = join(dir,'key'); await writeFile(file, token, { mode: 0o600 });
  const result = await configureAccess(file, DEFAULT_API_URL, env);
  assert.ok(!JSON.stringify(result).includes(token));
  if (process.platform !== 'win32') assert.equal((await stat(result.configPath)).mode & 0o777, 0o600);
  assert.deepEqual(await readAccess(env), { apiUrl: DEFAULT_API_URL, token });
  assert.equal((await accessStatus(env)).configured, true);
  const originalFetch = globalThis.fetch;
  const sent = [];
  globalThis.fetch = async (url, init) => { sent.push(init.headers.Authorization); return Response.json({ total_memes: '1', total_categories: 1, available_profiles: ['keyword'] }); };
  try {
    await run(['stats','--api-url',DEFAULT_API_URL], {env});
    await run(['stats','--api-url','https://other.example/agent/v1'], {env});
    await run(['stats','--api-url',DEFAULT_API_URL], {env:{...env,EMOMO_API_TOKEN:'custom',EMOMO_API_TOKEN_URL:DEFAULT_API_URL}});
    await run(['stats','--api-url',DEFAULT_API_URL], {env:{...env,EMOMO_API_TOKEN:'custom',EMOMO_API_TOKEN_URL:'https://other.example/agent/v1'}});
    assert.deepEqual(sent,[`Bearer ${token}`,undefined,'Bearer custom',undefined]);
  } finally { globalThis.fetch = originalFetch; }
  assert.ok(!JSON.stringify(await run(['auth','status'],{env})).includes(token));
  if (process.platform !== 'win32') { await chmod(result.configPath,0o644); await assert.rejects(readAccess(env), /private access/); }
  await chmod(result.configPath,0o600); await clearAccess(env); assert.equal(await readAccess(env),null);
  await assert.rejects(configureAccess(file, 'http://example.test/agent/v1', env));
 } finally { await rm(dir,{recursive:true,force:true}); }
});
test('official image authorization is origin-bound and authenticated redirects never forward it', async () => {
 const dir = await mkdtemp(join(tmpdir(),'emomo-image-auth-'));
 const id = 'a'.repeat(64);
 try {
  const calls=[];
  const client = new EmomoClient({ baseUrl: DEFAULT_API_URL, token });
  client.fetch = async(url,init) => { calls.push({url:String(url),init}); return new Response(null,{status:302,headers:{location:'https://other.example/file.png'}}); };
  client.get = async() => ({ meme: { id, url: `https://images.timmyagentic.si/collections/reviewed/${id}.png` } });
  await assert.rejects(client.download(id,dir),e=>e.code==='IMAGE_REDIRECT');
  assert.equal(calls.length,1); assert.equal(calls[0].init.headers.Authorization,`Bearer ${token}`);
 } finally { await rm(dir,{recursive:true,force:true}); }
});

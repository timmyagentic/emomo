import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createHmac } from 'node:crypto';
import { build } from 'esbuild';
import { Miniflare, Response as MFResponse } from 'miniflare';

const bundle = await build({ stdin: { resolveDir: process.cwd(), contents: `
import handler from './src/index.ts';
import { authenticate, admit, AccessError, accessError } from '../deployments/cloudflare/agent-search/src/access.ts';
import { Account as RealAccount } from './src/account.ts';
export class Account extends RealAccount {
 async inspect(key) { return await this.ctx.storage.get(key) ?? null; }
 async seed(key,value) { await this.ctx.storage.put(key,value); }
}
export { Admission } from '../deployments/cloudflare/agent-search/src/access.ts';
let offset = 0; Date.now = () => 1800000000000 + offset;
export default { async fetch(r,e,c) {
 if (new URL(r.url).pathname === '/__code-rpc') { const {name,method,args}=await r.json(); const a=e.ACCOUNT_STORE.getByName(name); return Response.json(await a[method](...args) ?? null); }
 if (new URL(r.url).pathname === '/__storage') { const {name,key,value}=await r.json(); const a=e.ACCOUNT_STORE.getByName(name); if(value!==undefined)await a.seed(key,value); return Response.json(await a.inspect(key)); }
 if (new URL(r.url).pathname === '/__advance') { offset += Number(await r.text()); return new Response('ok'); }
 if (new URL(r.url).pathname === '/__access') {
  try { const identity = await authenticate(r,'{}','read',e.ACCOUNT_STORE); identity.quota={hour:2,day:2}; const reservation=await admit(e,'read',identity); await reservation.finish(false); return Response.json({id:identity.id}); }
  catch(error){if(error instanceof AccessError)return accessError(error);throw error;}
 }
 return handler.fetch(r,{...e,EMAIL:{send:async message=> {const response=await e.CAPTURE.fetch('http://capture',{method:'POST',body:JSON.stringify(message)});if(!response.ok)throw Error('mail failure');}}},c);
}};` }, bundle: true, write: false, format: 'esm', platform: 'browser', external: ['cloudflare:workers'] });
async function fixture(mailFails = false) {
 const mail: Array<{to:string;text:string}> = [];
 const mf = new Miniflare({ modules: true, script: bundle.outputFiles[0].text, compatibilityDate:'2026-07-30',
  bindings: { ACCOUNTS_ENABLED:'true', ACCOUNT_SECRET:'fixture-only-secret-not-used-in-production', SITE_ORIGINS:'https://emomo.test', EMAIL_FROM:'login@example.invalid', SERVICE_PAUSED:'false', GLOBAL_LIMITS_JSON:JSON.stringify({read:{perSecond:100,perDay:1000,inFlight:100,failures:5,cooldownSeconds:30}}) },
  durableObjects: {ACCOUNT_STORE:{className:'Account',useSQLite:true}, ADMISSION:{className:'Admission',useSQLite:true}},
  ratelimits: { ACCOUNT_RATE_LIMITER:{namespace_id:'1010',simple:{limit:1000,period:60}}},
  serviceBindings: {CAPTURE:async (r: { json(): Promise<unknown> })=>{mail.push(await r.json() as {to:string;text:string});return new MFResponse('ok',{status:mailFails?503:200});}},
 });
 let n = 1;
 const call = (path:string,data?:object,cookie='',origin='https://emomo.test',ip?:string) => mf.dispatchFetch('https://emomo.test/api/account/'+path,{
  method:data?'POST':'GET',headers:{'content-type':'application/json',origin,cookie,'cf-connecting-ip':ip ?? `192.0.2.${n++}`},body:data?JSON.stringify(data):undefined,
 });
 const code = async(email:string) => {const r=await call('code',{email});assert.equal(r.status,200);const challenge=(await r.json() as any).challenge;const sent=[...mail].reverse().find(m=>m.to===email)!;return {email,challenge,code:sent.text.match(/\d{6}/)![0]};};
 const login=async(email:string)=>{const r=await call('verify',await code(email));assert.equal(r.status,200);return r.headers.get('set-cookie')!.split(';')[0];};
 const advance = (ms=60001) => mf.dispatchFetch('https://emomo.test/__advance',{method:'POST',body:String(ms)});
 const storage = async(name:string) => ({ get: async(key:string) => (await mf.dispatchFetch('https://emomo.test/__storage',{method:'POST',body:JSON.stringify({name,key})})).json(), put: async(key:string,value:unknown) => mf.dispatchFetch('https://emomo.test/__storage',{method:'POST',body:JSON.stringify({name,key,value})}) });
 const hash = (value:string) => createHmac('sha256','fixture-only-secret-not-used-in-production').update(value).digest('hex');
 const accountId = (email:string) => hash('account:'+email).slice(0,24);
 const budgets = async(email:string,ip:string) => Promise.all([`send-ip/${hash('ip:'+ip)}`,`send-email/${accountId(email)}`,'send-global'].map(async name => (await storage(name)).get('budget')));
 const rpc = async(email:string,method:string,...args:unknown[]) => (await mf.dispatchFetch('https://emomo.test/__code-rpc',{method:'POST',body:JSON.stringify({name:'user/'+accountId(email),method,args})})).json();
 return {mf,mail,call,code,login,advance,storage,accountId,budgets,rpc};
}
test('email registration creates a private session; key is shown once, authenticated and revoked',async()=>{
 const f=await fixture();try{
  assert.equal((await f.call('me')).status,401);
  const login=await f.call('verify',await f.code('one@example.com'));
  assert.equal(login.status,200);const header=login.headers.get('set-cookie')!;
  for(const attribute of ['HttpOnly','Secure','SameSite=Lax','Path=/'])assert.ok(header.includes(attribute));
  const cookie=header.split(';')[0];
  const me=await (await f.call('me',undefined,cookie)).json() as any;assert.equal(me.email,'one@example.com');assert.deepEqual(me.keys,[]);
  const created=await f.call('keys',{name:'我的电脑'},cookie);assert.equal(created.status,201);const key=await created.json() as any;
  assert.match(key.token,/^emomo_web-[a-f0-9]{24}-[a-f0-9]{16}_[a-f0-9]{64}$/);
  const listing=await (await f.call('me',undefined,cookie)).text();assert.ok(!listing.includes(key.token));assert.ok(!listing.includes('"hash"'));
  const access=await f.mf.dispatchFetch('https://emomo.test/__access',{headers:{authorization:`Bearer ${key.token}`}});assert.equal(access.status,200);
  assert.equal((await f.call(`keys/${key.key.id}/revoke`,{},cookie)).status,200);
  assert.equal((await f.mf.dispatchFetch('https://emomo.test/__access',{headers:{authorization:`Bearer ${key.token}`}})).status,401);
  assert.equal((await f.call('logout',{},cookie)).status,200);assert.equal((await f.call('me',undefined,cookie)).status,401);
 }finally{await f.mf.dispose();}
});
test('code is single-use under concurrency, limited to five guesses and expires',async()=>{
 const f=await fixture();try{
  const code=await f.code('once@example.com');const race=await Promise.all([f.call('verify',code),f.call('verify',code)]);assert.deepEqual(race.map(r=>r.status).sort(),[200,400]);
  const guesses=await f.code('guesses@example.com');const wrong=guesses.code==='000000'?'111111':'000000';
  for(let i=0;i<5;i++)assert.equal((await f.call('verify',{...guesses,code:wrong})).status,400);
  assert.equal((await f.call('verify',guesses)).status,400);
  const expired=await f.code('expired@example.com');await f.mf.dispatchFetch('https://emomo.test/__advance',{method:'POST',body:'601000'});
  assert.equal((await f.call('verify',expired)).status,400);
 }finally{await f.mf.dispose();}
});
test('accounts are isolated and replacement keys share the same cumulative quota',async()=>{
 const f=await fixture();try{
  const alice=await f.login('alice@example.com'),bob=await f.login('bob@example.com');
  const create=async(cookie:string)=>(await (await f.call('keys',{name:'CLI'},cookie)).json()) as any;
  const first=await create(alice),second=await create(alice);
  assert.equal((await f.call(`keys/${first.key.id}/revoke`,{},bob)).status,404);
  const access=(key:string)=>f.mf.dispatchFetch('https://emomo.test/__access',{headers:{authorization:`Bearer ${key}`}});
  assert.equal((await access(first.token)).status,200);assert.equal((await access(second.token)).status,200);
  assert.equal((await access(first.token)).status,429);
  const bobKey=await create(bob);assert.equal((await access(bobKey.token)).status,200);
  await f.call(`keys/${first.key.id}/revoke`,{},alice);const replacement=await create(alice);assert.equal((await access(replacement.token)).status,429);
  const me=await (await f.call('me',undefined,alice)).json() as any;assert.equal(me.usage.read.daily,2);
 }finally{await f.mf.dispose();}
});
test('CSRF, oversized bodies, rapid resend and failed delivery do not create usable sessions',async()=>{
 const f=await fixture();try{
  assert.equal((await f.call('code',{email:'one@example.com'},'','https://evil.test')).status,403);assert.equal(f.mail.length,0);
  assert.equal((await f.call('code',{email:'x'.repeat(3000)})).status,413);
  await f.code('one@example.com');assert.equal((await f.call('code',{email:'one@example.com'})).status,429);assert.equal(f.mail.length,1);
  const cookie=await f.login('two@example.com');for(let i=0;i<3;i++)assert.equal((await f.call('keys',{name:'key'+i},cookie)).status,201);
  assert.equal((await f.call('keys',{name:'extra'},cookie)).status,429);
 }finally{await f.mf.dispose();}
 const failed=await fixture(true);try{assert.equal((await failed.call('code',{email:'one@example.com'})).status,503);assert.equal((await failed.call('me')).status,401);}finally{await failed.mf.dispose();}
});

test('session expiry and key history pruning preserve active credentials',async()=>{
 const f=await fixture();try{
  const cookie=await f.login('retention@example.com');
  const create=async()=>(await (await f.call('keys',{name:'rotation'},cookie)).json()) as any;
  const original=await create();
  for(let i=0;i<12;i++){
   if(i===8)await f.mf.dispatchFetch('https://emomo.test/__advance',{method:'POST',body:'86400000'});
   const key=await create();assert.ok(key.token);assert.equal((await f.call(`keys/${key.key.id}/revoke`,{},cookie)).status,200);
  }
  const access=await f.mf.dispatchFetch('https://emomo.test/__access',{headers:{authorization:`Bearer ${original.token}`}});assert.equal(access.status,200);
  await f.mf.dispatchFetch('https://emomo.test/__advance',{method:'POST',body:String(14*86400000)});
  assert.equal((await f.call('me',undefined,cookie)).status,401);
 }finally{await f.mf.dispose();}
});


test('cooldown repeats and concurrent requests leave all send budgets unchanged',async()=>{
 const f=await fixture();try{
  const email='repeat@example.com',ip='192.0.2.200';
  const send=()=>f.call('code',{email},'','https://emomo.test',ip);
  const first=await send();assert.equal(first.status,200);
  const challenge=(await first.json() as any).challenge;
  const before=await f.budgets(email,ip);
  const repeated=await Promise.all(Array.from({length:12},send));
  assert.ok(repeated.every(r=>r.status===429));
  assert.deepEqual(await f.budgets(email,ip),before);assert.equal(f.mail.length,1);
  assert.equal((await f.call('verify',{email,challenge,code:f.mail[0].text.match(/\d{6}/)![0]})).status,200);
  await f.advance();
  const race=await Promise.all(Array.from({length:12},send));
  assert.equal(race.filter(r=>r.status===200).length,1);assert.equal(f.mail.length,2);
  for(const budget of await f.budgets(email,ip))assert.equal((budget as any).daily,2);
  for(let i=0;i<3;i++){await f.advance();assert.equal((await send()).status,200);}
  await f.advance();assert.equal((await send()).status,429);assert.equal(f.mail.length,5);
 }finally{await f.mf.dispose();}
});

test('budget rejection preserves the previous code and does not start a cooldown',async()=>{
 const f=await fixture();try{
  const email='budget@example.com',ip='192.0.2.201';
  const code=await f.code(email);await f.advance();
  const budget=await f.storage('send-ip/'+createHmac('sha256','fixture-only-secret-not-used-in-production').update('ip:'+ip).digest('hex'));
  const now=1800000060001,hour=Math.floor(now/3600000),day=Math.floor(now/86400000);
  await budget.put('budget',{hour,day,hourly:5,daily:5});
  assert.equal((await f.call('code',{email},'','https://emomo.test',ip)).status,429);
  assert.equal((await f.call('verify',code)).status,200);
  assert.equal((await f.call('code',{email})).status,200);
 }finally{await f.mf.dispose();}
});

test('failed delivery consumes one send budget and cooldown, and cancels its code',async()=>{
 const f=await fixture(true);try{
  const email='failed@example.com',ip='192.0.2.202';
  const send=()=>f.call('code',{email},'','https://emomo.test',ip);
  assert.equal((await send()).status,503);
  const state=await (await f.storage('user/'+f.accountId(email))).get('account') as any;
  assert.equal(state.code,undefined);assert.equal(state.lastCode,1800000000000);
  assert.equal((await send()).status,429);assert.equal(f.mail.length,1);
  for(const budget of await f.budgets(email,ip))assert.equal((budget as any).daily,1);
  await f.advance();assert.equal((await send()).status,503);assert.equal(f.mail.length,2);
  for(const budget of await f.budgets(email,ip))assert.equal((budget as any).daily,2);
 }finally{await f.mf.dispose();}
});


test('abandoned claims expire; stale commit, release and delivery failure cannot replace a newer code',async()=>{
 const f=await fixture();try{
  const email='lease@example.com';
  assert.equal(await f.rpc(email,'claimCode','old'),0);
  assert.equal((await f.call('code',{email})).status,429);
  assert.deepEqual(await f.budgets(email,'192.0.2.1'),[null,null,null]);
  await f.advance();
  assert.equal(await f.rpc(email,'beginCode',email,'old','hash'),false);
  assert.equal(await f.rpc(email,'claimCode','new'),0);
  await f.rpc(email,'releaseCode','old');
  assert.equal(await f.rpc(email,'beginCode',email,'old','hash'),false);
  assert.equal(await f.rpc(email,'beginCode',email,'new','new-hash'),true);
  await f.rpc(email,'cancelCode','old');
  const state=await (await f.storage('user/'+f.accountId(email))).get('account') as any;
  assert.equal(state.code.challenge,'new');assert.equal(state.pendingCode,undefined);
 }finally{await f.mf.dispose();}
});

test('email and global hourly/daily limits retain conservative partial admission semantics',async()=>{
 for(const dimension of ['email','global'])for(const period of ['hourly','daily']){
  const f=await fixture();try{
   const email='limit@example.com',ip='192.0.2.203',name=dimension==='email'?'send-email/'+f.accountId(email):'send-global';
   const hour=Math.floor(1800000000000/3600000),day=Math.floor(1800000000000/86400000);
   const limit=dimension==='email'?(period==='hourly'?5:10):(period==='hourly'?100:500);
   await (await f.storage(name)).put('budget',{hour,day,hourly:0,daily:0,[period]:limit});
   const response=await f.call('code',{email},'','https://emomo.test',ip);
   assert.equal(response.status,429);assert.ok(Number(response.headers.get('retry-after'))>60);assert.equal(f.mail.length,0);
   const budgets=await f.budgets(email,ip) as any[];
   assert.equal(budgets[0].daily,1);if(dimension==='global')assert.equal(budgets[1].daily,1);
   const state=await (await f.storage('user/'+f.accountId(email))).get('account') as any;
   assert.equal(state.code,undefined);assert.equal(state.lastCode,undefined);assert.equal(state.pendingCode,undefined);
  }finally{await f.mf.dispose();}
 }
});


test('ten actual email sends across hourly windows exhaust the daily budget',async()=>{
 const f=await fixture();try{
  const email='daily@example.com';
  for(let i=0;i<10;i++){
   if(i===5)await f.advance(3600000);else if(i)await f.advance();
   assert.equal((await f.call('code',{email})).status,200);
  }
  await f.advance();assert.equal((await f.call('code',{email})).status,429);assert.equal(f.mail.length,10);
 }finally{await f.mf.dispose();}
});

test('concurrent distinct emails cannot exceed one IP send allowance',async()=>{
 const f=await fixture();try{
  const results=await Promise.all(Array.from({length:12},(_,i)=>f.call('code',{email:`race${i}@example.com`},'','https://emomo.test','192.0.2.204')));
  assert.equal(results.filter(r=>r.status===200).length,5);
  assert.equal(results.filter(r=>r.status===429).length,7);assert.equal(f.mail.length,5);
 }finally{await f.mf.dispose();}
});

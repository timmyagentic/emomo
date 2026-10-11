import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';
import { Miniflare, Response as MFResponse } from 'miniflare';

const bundle = await build({ stdin: { resolveDir: process.cwd(), contents: `
import handler from './src/index.ts';
import { authenticate, admit, AccessError, accessError } from '../deployments/cloudflare/agent-search/src/access.ts';
export { Account } from './src/account.ts';
export { Admission } from '../deployments/cloudflare/agent-search/src/access.ts';
let offset = 0; const realNow = Date.now; Date.now = () => realNow() + offset;
export default { async fetch(r,e,c) {
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
 const call = (path:string,data?:object,cookie='',origin='https://emomo.test') => mf.dispatchFetch('https://emomo.test/api/account/'+path,{
  method:data?'POST':'GET',headers:{'content-type':'application/json',origin,cookie,'cf-connecting-ip':`192.0.2.${n++}`},body:data?JSON.stringify(data):undefined,
 });
 const code = async(email:string) => {const r=await call('code',{email});assert.equal(r.status,200);const challenge=(await r.json() as any).challenge;const sent=[...mail].reverse().find(m=>m.to===email)!;return {email,challenge,code:sent.text.match(/\d{6}/)![0]};};
 const login=async(email:string)=>{const r=await call('verify',await code(email));assert.equal(r.status,200);return r.headers.get('set-cookie')!.split(';')[0];};
 return {mf,mail,call,code,login};
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

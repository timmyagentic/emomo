import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, mkdtemp, rm, realpath } from 'node:fs/promises';
import { join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { tmpdir } from 'node:os';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { build } from 'esbuild';
import { Miniflare, Response as MFResponse } from 'miniflare';
import { indexRow, importSQL } from '../src/metadata.js';
import { identifyImage } from '../../../../cli/src/client.js';

const execute = promisify(execFile);
const hash = (bytes: Uint8Array | string) => createHash('sha256').update(bytes).digest('hex');
type Case = { userQuery: string; query: string; expectedIds: string[]; category?: string };
type ObjectFile = { id:string; key:string; sha256:string; bytes:number; relativeFile:string };
type Candidate = { id: string; category: string; description: string; tags: string[]; textPresence: string };
const [source,output,casesPath,...extra] = process.argv.slice(2);
if (!source || !output || !casesPath || extra.length) throw new Error('Usage: npm run static:verify -- <prepared-directory> <new-receipt-directory> <cases.json>');
const root=await realpath(resolve(source)), target=resolve(output);
await mkdir(target); // Fail rather than overwrite any prior acceptance evidence.
const scratch=await mkdtemp(join(tmpdir(),'emomo-real-static-'));
let mf:Miniflare|undefined;
let calls=0, requests=0, blockedImage:string|undefined;
const objects:ObjectFile[]=JSON.parse(await readFile(join(root,'objects.json'),'utf8'));
const mapping=new Map(objects.map(o=>[o.key,o]));
async function asset(object:ObjectFile) {
  if (isAbsolute(object.relativeFile) || object.relativeFile.includes('\\') || object.relativeFile.split('/').includes('..')) throw Error('Invalid prepared image path.');
  const path=await realpath(join(root,object.relativeFile));const rel=relative(root,path);
  if (rel==='..'||rel.startsWith('../')||isAbsolute(rel)) throw Error('Prepared asset escapes its directory.');
  const bytes=await readFile(path);
  assert.equal(bytes.length,object.bytes);assert.equal(hash(bytes),object.sha256);
  return bytes;
}
const server=createServer(async(incoming,outgoing)=>{
  try {
    const key=(incoming.url??'').slice(1);
    const object=mapping.get(key);
    if (object) {
      if (key===blockedImage) { outgoing.writeHead(404);outgoing.end();return; }
      const bytes=await asset(object);const image=identifyImage(bytes);
      outgoing.writeHead(200,{'content-type':image.mimeType});outgoing.end(bytes);return;
    }
    const chunks:Buffer[]=[];let length=0;
    for await(const chunk of incoming){length+=chunk.length;if(length>8192){outgoing.writeHead(413);outgoing.end();return;}chunks.push(Buffer.from(chunk));}
    const response=await mf!.dispatchFetch(`${origin}${incoming.url}`,{method:incoming.method,
      headers:{'content-type':incoming.headers['content-type']??'application/json','cf-connecting-ip':`192.0.${Math.floor(requests/250)}.${++requests%250+1}`},
      body:incoming.method==='POST'?Buffer.concat(chunks):undefined});
    outgoing.writeHead(response.status,Object.fromEntries(response.headers));outgoing.end(Buffer.from(await response.arrayBuffer()));
  } catch { outgoing.writeHead(500);outgoing.end('Local verification bridge failed.'); }
});
server.listen(0,'127.0.0.1');await once(server,'listening');const address=server.address();assert.ok(address&&typeof address!=='string');
const origin=`http://127.0.0.1:${address.port}`;
try {
  const meta=await readFile(join(root,'metadata.jsonl'),'utf8'),sql=await readFile(join(root,'index.sql'),'utf8');
  const preparation=JSON.parse(await readFile(join(root,'preparation.json'),'utf8'));
  assert.equal(hash(meta),preparation.metadataSha256);assert.equal(hash(sql),preparation.sqlSha256);
  const rows=meta.trim().split('\n').map(line=>indexRow(JSON.parse(line)));
  assert.equal(rows.length,preparation.records);assert.equal(rows.length,objects.length);
  assert.equal(new Set(rows.map(r=>r.id)).size,rows.length);
  for(let i=0;i<rows.length;i++){
    const meme=JSON.parse(rows[i].meme_json);
    assert.equal(meme.storage_key,objects[i].key);assert.equal(meme.id,objects[i].id);assert.equal(meme.content_hash,objects[i].sha256);
    assert.ok([1,2,3].includes(meme.image_info.format));assert.ok(!objects[i].key.endsWith('.gif'));
    assert.ok([1,2].includes(rows[i].text_presence));await asset(objects[i]);
  }
  assert.equal(rows.map(importSQL).join('\n')+'\n',sql);
  const workerRoot=fileURLToPath(new URL('../',import.meta.url).href);
  const bundle=await build({entryPoints:[join(workerRoot,'src/index.ts')],write:false,bundle:true,format:'esm',platform:'browser',target:'es2022',metafile:true});
  const inputs=Object.keys(bundle.metafile!.inputs);
  assert.ok(inputs.every(p=>{const rel=relative(workerRoot,resolve(p));return rel.startsWith('src/')||rel.startsWith('gen/')||rel.startsWith('node_modules/@bufbuild/protobuf/');}));
  mf=new Miniflare({modules:true,script:bundle.outputFiles[0].text,compatibilityDate:'2026-07-30',compatibilityFlags:['nodejs_compat'],
    bindings:{AGENT_API_ENABLED:'true',IMAGE_BASE_URL:origin},d1Databases:{DB:'actual-static-index'},
    ratelimits:{EMOMO_RATE_LIMITER:{namespace_id:'1100',simple:{limit:30,period:60}}},
    outboundService:()=>{calls++;return new MFResponse('External HTTP prohibited',{status:502});}});
  const db=await mf.getD1Database('DB');
  const schema=await readFile(join(workerRoot,'migrations/0001_text_index.sql'),'utf8');
  await db.batch(schema.split('-- statement').map(s=>s.trim()).filter(Boolean).map(s=>db.prepare(s)));
  for(let start=0;start<rows.length;start+=50) await db.batch(rows.slice(start,start+50).map(row=>db.prepare(importSQL(row))));
  const stored=await db.prepare('SELECT id,text_presence,category FROM memes ORDER BY id').all<{id:string;text_presence:number;category:string}>();
  assert.equal(stored.results.length,rows.length);assert.deepEqual(stored.results.map((r:{id:string})=>r.id).sort(),rows.map(r=>r.id).sort());
  const cases:Case[]=JSON.parse(await readFile(casesPath,'utf8'));
  assert.ok(cases.length>0&&cases.every(c=>typeof c.query==='string'&&c.expectedIds.length>0));
  const cliRoot=fileURLToPath(new URL('../../../../cli/',import.meta.url).href);
  const pack=JSON.parse((await execute('npm',['pack','--pack-destination',target,'--json'],{cwd:cliRoot})).stdout)[0];
  const prefix=join(target,'installed');
  await execute('npm',['install','--global','--prefix',prefix,'--ignore-scripts','--no-audit','--no-fund','--offline',join(target,pack.filename)]);
  const binary=join(prefix,'bin/emomo'),cwd=join(target,'clean');await mkdir(cwd);
  const env={AGENT_API_ENABLED:'false',IMAGE_BASE_URL:'',PATH:process.env.PATH,HOME:process.env.HOME,TMPDIR:process.env.TMPDIR,EMOMO_CONFIG_DIR:join(scratch,'config'),CODEX_HOME:join(target,'codex')};
  const invoke=async(args:string[],expectedOK=true)=>{
    let stdout:string;
    try{stdout=(await execute(binary,['--api-url',`${origin}/agent/v1`,...args],{env,cwd})).stdout;assert.ok(expectedOK);}
    catch(error){if(expectedOK)throw error;stdout=(error as {stdout:string}).stdout;}
    assert.equal(stdout.trim().split('\n').length,1);const value=JSON.parse(stdout);assert.equal(value.schemaVersion,1);assert.equal(value.ok,expectedOK);return value;
  };
  const doctor=await invoke(['doctor']);assert.equal(Number(doctor.data.stats.totalMemes),rows.length);
  const skill=await execute(binary,['skill','install'],{env,cwd});assert.ok(JSON.parse(skill.stdout).ok);
  const regressions=[];
  for(const c of cases){
    const result=await invoke(['search',c.query,'--limit','8',...(c.category?['--category',c.category]:[])]);
    const candidates:Candidate[]=result.data.results;
    if(!c.category)assert.ok(candidates.every(r=>r.category!=='object_sticker'));
    regressions.push({...c,results:candidates,passed:candidates.some(r=>c.expectedIds.includes(r.id))});
  }
  const downloads=[];const byId=new Map(objects.map(o=>[o.id,o]));
  for(const format of [1,2,3]){
    const row=rows.find(r=>JSON.parse(r.meme_json).image_info.format===format);if(!row)continue;
    const object=byId.get(row.id)!;const detail=await invoke(['get',row.id]);assert.equal(detail.data.meme.id,row.id);
    const downloaded=await invoke(['download',row.id,'--dir',join(target,'downloads')]);const bytes=await readFile(downloaded.data.path);
    assert.equal(hash(bytes),object.sha256);assert.deepEqual(bytes,await asset(object));
    const overwrite=await invoke(['download',row.id,'--dir',join(target,'downloads')],false);assert.equal(overwrite.error.code,'FILE_EXISTS');
    downloads.push({...downloaded.data,format,originalBytesVerified:true});
  }
  const selectionFlows=[];
  for(const regression of regressions.slice(0,3)){
    const selected=regression.results.find(r=>regression.expectedIds.includes(r.id))!;
    const detail=await invoke(['get',selected.id]);assert.equal(detail.data.meme.id,selected.id);
    const download=await invoke(['download',selected.id,'--dir',join(target,'selected')]);
    assert.equal(hash(await readFile(download.data.path)),byId.get(selected.id)!.sha256);
    selectionFlows.push({userQuery:regression.userQuery,query:regression.query,selectedId:selected.id,download:download.data,originalBytesVerified:true});
  }
  const unknownRow=rows.find(r=>r.text_presence===1)!;
  const unknownMeme=JSON.parse(unknownRow.meme_json);const unknownQuery=unknownMeme.tags[0]||unknownRow.description;
  const unknown=await invoke(['search',unknownQuery,'--text','unknown','--limit','8']);assert.ok(unknown.data.results.length>0&&unknown.data.results.every((r:Candidate)=>r.textPresence==='unknown'));
  const noText=await invoke(['search',unknownQuery,'--text','without']);assert.equal(noText.data.total,0);
  const noMatch=await invoke(['search','zzzxqnonexistent987']);assert.equal(noMatch.data.total,0);
  const excludedId=preparation.excluded[0] ? `${preparation.collection}-${preparation.excluded[0].id}` : 'not-in-static';
  const missing=await invoke(['get',excludedId],false);assert.equal(missing.error.code,'NOT_FOUND');
  blockedImage=objects[0].key;
  const missingImage=await invoke(['download',objects[0].id,'--dir',join(target,'missing-image')],false);assert.equal(missingImage.error.code,'NOT_FOUND');
  assert.equal(calls,0);
  const report={ok:regressions.every(r=>r.passed),records:rows.length,usable:preparation.usable,objects:preparation.objects,assetHashesVerified:objects.length,
    sourceContentRevision:preparation.sourceContentRevision,regressions,passed:regressions.filter(r=>r.passed).length,total:regressions.length,
    downloads,selectionFlows,doctor,unknownTextPreserved:true,withoutTextNotInferred:true,noMatchCount:0,gifNotFound:true,missingImageError:missingImage.error.code,
    workerOutboundCalls:calls,publicReleaseReady:false,productionDeployed:false,package:pack.filename,cleanWorkingDirectory:cwd};
  await writeFile(join(target,'receipt.json'),JSON.stringify(report,null,2)+'\n',{flag:'wx',mode:0o600});
  console.log(JSON.stringify({ok:report.ok,records:report.records,passed:report.passed,total:report.total,workerOutboundCalls:calls,receipt:join(target,'receipt.json')}));
  if(!report.ok)process.exitCode=1;
}finally{server.closeAllConnections();server.close();await mf?.dispose();await rm(scratch,{recursive:true,force:true});}

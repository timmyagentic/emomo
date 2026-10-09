import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, writeFile, readFile, rm, mkdir, stat } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fromJson } from '@bufbuild/protobuf';
import { MemeSchema } from '../gen/emomo/v1/meme_pb.js';
import { prepareStatic, staticRecord } from '../scripts/prepare-static.js';
import { indexRow } from '../src/metadata.js';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import type { LibraryRecord } from '../../../../cli/src/library.js';
const execute=promisify(execFile);
const PNG=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=','base64');
const GIF=Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAAAAAAALAAAAAABAAEAAAIBRAA7','base64');
const hash=(bytes:Uint8Array)=>createHash('sha256').update(bytes).digest('hex');
const record:LibraryRecord={id:'local-1',sourceId:'1',description:'猫笑了',imageText:'',category:'usable',tags:['开心'],subjects:['猫'],scenarios:['聊天'],aliases:['笑不活了'],contentFlags:[],quality:'可辨识',mediaKind:'image',previewOnly:false,image:{width:1,height:1,frames:1,format:'png'},sha256:hash(PNG),publicReleaseClearance:'UNVERIFIED',asset:`assets/${hash(PNG)}.png`};

test('static canonical projection preserves identity, metadata and UNKNOWN; excludes every GIF and animation',()=>{
 const projected=staticRecord('first-batch',record);const row=indexRow(projected);
 assert.equal(row.id,'first-batch-local-1');assert.equal(row.text_presence,1);assert.equal(row.description,record.description);
 const meme=fromJson(MemeSchema,JSON.parse(row.meme_json));assert.equal(meme.contentHash,record.sha256);assert.deepEqual(meme.tags,['开心','猫']);assert.ok(!meme.tags.includes('笑不活了'));assert.match(row.tag_terms,/笑不/);
 assert.throws(()=>staticRecord('batch',{...record,image:{...record.image,format:'gif'}}));
 assert.throws(()=>staticRecord('batch',{...record,image:{...record.image,frames:2},mediaKind:'animation'}));
 assert.throws(()=>staticRecord('x'.repeat(120),record));
 const withText=staticRecord('first-batch',{...record,imageText:'哈哈哈'});assert.equal(indexRow(withText).text_presence,2);
});

test('real snapshot converter preserves asset bytes and private modes, refuses overwrite and validates corruption',async t=>{
 const root=await mkdtemp(join(tmpdir(),'emomo-static-'));t.after(()=>rm(root,{recursive:true,force:true}));
 await writeFile(join(root,'one.png'),PNG);await writeFile(join(root,'one.gif'),GIF);
 const base={id:1,description:'猫笑了',category:'usable',image_text:'',width:1,height:1,frames:1,media_kind:'image',file_path:join(root,'one.png'),sha256:hash(PNG),query_aliases:['笑不活了'],subjects:['猫']};
 const input=join(root,'input.json');await writeFile(input,JSON.stringify([base,{...base,id:2,category:'object_sticker'},{...base,id:3,file_path:join(root,'one.gif'),sha256:hash(GIF)}]));
 const cli=new URL('../../../../cli/bin/emomo.js',import.meta.url).pathname;
 const maintenance=new URL('../../../../cli/bin/emomo-library.js',import.meta.url).pathname;
 const catalog=join(root,'catalog'),snapshot=join(root,'snapshot');
 await execute(process.execPath,[cli,'catalog','import',input,'--dir',catalog]);
 await execute(process.execPath,[maintenance,'prepare','--catalog',catalog,'--dir',snapshot,'--collection','batch','--revision','v1']);
 const out=join(root,'static');const result=await prepareStatic(snapshot,out);
 assert.equal(result.records,2);assert.equal(result.usable,1);assert.equal(result.objects,1);assert.equal(result.excluded.length,1);assert.equal(result.textUnknown,2);assert.equal(result.publicReleaseReady,false);
 const objects=JSON.parse(await readFile(join(out,'objects.json'),'utf8'));
 assert.deepEqual(await readFile(join(out,objects[0].relativeFile)),PNG);assert.equal((await stat(join(out,'metadata.jsonl'))).mode&0o777,0o600);
 assert.ok((await readFile(join(out,'metadata.jsonl'),'utf8')).split('\n').filter(Boolean).every(line=>indexRow(JSON.parse(line)).text_presence===1));
 await assert.rejects(()=>prepareStatic(snapshot,out));
 const empty=join(root,'empty');await mkdir(empty);await assert.rejects(()=>prepareStatic(snapshot,empty));
 await writeFile(join(snapshot,record.asset),'corrupt');await assert.rejects(()=>prepareStatic(snapshot,join(root,'bad')));
 await assert.rejects(()=>stat(join(root,'bad')),{code:'ENOENT'});
});

// Regression: public imports must not resurrect duplicate variants or uncertain OCR.
test('uncertain OCR is omitted from static index projection', () => {
 const row=indexRow(staticRecord('reviewed',{...record,imageText:'uncertain words',ocrReview:'partially_illegible'}));
 assert.equal(row.ocr_text,'');assert.equal(row.text_presence,1);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import {demoManifest,loadDemo,importedContent,digest} from './content.js';

const base='https://game.example/play/demo/manifest.json';
globalThis.location={origin:'https://game.example'};
async function fixture() {
  const values=[new Blob(['authored archive fixture']),new Blob(['authored readme fixture'])];
  const files=[];
  for (const [i,path] of ['TADemo.hpi','TADemoReadme.txt'].entries()) files.push({path,url:'files/'+i,size:values[i].size,sha256:await digest(values[i])});
  return {manifest:{schema:1,kind:'ta-demo',files},values};
}
function served(manifest,url=base){const r=Response.json(manifest); Object.defineProperty(r,'url',{value:url}); return r;}

test('folder mounting strips the install root and rejects traversal', () => {
  const blob=new Blob(['authored']);
  const c=importedContent([['TotalA/TotalA1.hpi',blob],['TotalA/maps/map.ota',blob],['outside.txt',blob]]);
  assert.equal(c.kind,'retail'); assert.deepEqual(c.files.map(x=>x[0]),['TotalA1.hpi','maps/map.ota']);
  assert.equal(importedContent([['Demo/TADemo.hpi',blob]]).kind,'demo');
  assert.throws(()=>importedContent([['TotalA/TotalA1.hpi',blob],['TotalA/../bad',blob]]),/Invalid local/);
});

test('manifest rejects outside origins, escaping paths, names, sizes and duplicate assets', async () => {
  const {manifest}=await fixture();
  for(const patch of [{url:'https://other.example/a'},{url:'../retail.hpi'},{path:'TotalA.exe'},{size:0},{size:65*1024*1024},{sha256:'invalid'},{path:null}]) {
    const invalid=structuredClone(manifest); Object.assign(invalid.files[0],patch);
    globalThis.fetch=async()=>served(invalid); await assert.rejects(demoManifest(base),/Invalid demo/);
  }
  const duplicate=structuredClone(manifest);duplicate.files[1].path=duplicate.files[0].path;
  globalThis.fetch=async()=>served(duplicate);await assert.rejects(demoManifest(base),/Invalid demo/);
  globalThis.fetch=async()=>served(manifest,'https://other.example/demo/manifest.json');await assert.rejects(demoManifest(base),/same-origin/);
  globalThis.fetch=async()=>new Response('',{status:404});assert.equal(await demoManifest(base),null);
});

test('corrupt cached bytes are replaced and only verified downloads enter the cache', async () => {
  const {manifest,values}=await fixture();let puts=0,downloads=0;
  globalThis.caches={open:async()=>({match:async()=>new Response('corrupt'),delete:async()=>{throw Error('cache denied');},put:async(_,r)=>{puts++;assert.match(await r.text(),/authored/);}})};
  globalThis.fetch=async url=>{downloads++;return new Response(values[Number(url.slice(-1))]);};
  const c=await loadDemo(manifest,new AbortController().signal,()=>{});
  assert.equal(c.files.length,2);assert.equal(downloads,2);assert.equal(puts,2);
});

test('cache absence or denial leaves uncached demo loading available', async () => {
  const {manifest,values}=await fixture();
  for(const cache of [undefined,{open:async()=>{throw Error('denied');}}]) {
    globalThis.caches=cache;
    globalThis.fetch=async url=>new Response(values[Number(url.slice(-1))]);
    assert.equal((await loadDemo(manifest,new AbortController().signal,()=>{})).files.length,2);
  }
});

test('mismatched download and cancelled acquisition never produce a mounted source', async () => {
  const {manifest}=await fixture();let puts=0;
  globalThis.caches={open:async()=>({match:async()=>undefined,put:async()=>puts++})};
  globalThis.fetch=async()=>new Response(new Uint8Array(manifest.files[0].size));
  await assert.rejects(loadDemo(manifest,new AbortController().signal,()=>{}),/integrity/);assert.equal(puts,0);
  globalThis.fetch=async()=>new Response('too long'.repeat(100));
  await assert.rejects(loadDemo(manifest,new AbortController().signal,()=>{}),/size mismatch/);
  const controller=new AbortController();controller.abort();
  await assert.rejects(loadDemo(manifest,controller.signal,()=>{}),{name:'AbortError'});
});

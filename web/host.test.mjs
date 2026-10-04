import test from 'node:test';
import assert from 'node:assert/strict';
import {BrowserHost} from './host.js';

// A retired element leaves the document: its browsing context is gone, not
// merely navigating away.
function makeFrame(retired) {
  return {contentWindow:{postMessage(){}}, hidden:true, src:null,
    removeAttribute(name){this[name]=null;},
    cloneNode(){ return makeFrame(retired); },
    replaceWith(){ retired.push(this); }};
}
function setup() {
  let message;
  globalThis.location = {origin:'https://game.example'};
  globalThis.window = {addEventListener:(_, handler) => { message = handler; }};
  const leases = new Map();
  Object.defineProperty(globalThis, 'navigator', {configurable:true, value:{locks:{request:async (name, options, callback) => {
    if (leases.has(name)) return callback(null);
    leases.set(name, true);
    try { return await callback({name}); } finally { leases.delete(name); }
  }}}});
  const retired=[], frame = makeFrame(retired);
  const received=[];
  const host = new BrowserHost(frame, (...event) => received.push(event));
  return {host, frame, retired, received, send:(data, source=host.frame.contentWindow, origin=location.origin) => message({data, source, origin}), leases};
}

test('restart discards the prior iframe element before reacquiring its lease', async () => {
  const {host,frame,retired,leases}=setup();
  await host.start({name:'first'}); const old=host.run.id, first=host.frame;
  assert.notEqual(first,frame, 'start begins from a fresh element'); assert.deepEqual(retired,[frame]);
  await host.start({name:'second'});
  assert.notEqual(host.run.id,old); assert.equal(host.run.config.name,'second');
  assert.equal(Object.isFrozen(host.run.config),true);
  assert.deepEqual(retired,[frame,first], 'the first runtime element was removed');
  assert.equal(leases.size,1); assert.equal(host.frame.hidden,false); assert.match(host.frame.src,/\/web\/game\.html\?run=/);
  await host.stop(); assert.equal(leases.size,0); assert.equal(host.frame.hidden,true); assert.equal(host.frame.src,null);
  assert.equal(retired.length,3);
});

test('stop cancels admission while a launch is awaiting the prior lease', async () => {
  const {host,leases}=setup(); await host.start({});
  const pending=host.start({}); await host.stop();
  await assert.rejects(pending,{name:'AbortError'}); assert.equal(host.run,null); assert.equal(leases.size,0);
});

test('another launcher at the same origin cannot take the active runtime lease', async () => {
  const {host,leases}=setup(); await host.start({});
  const other=new BrowserHost(makeFrame([]),()=>{});
  await assert.rejects(other.start({}),/another tab/); assert.equal(other.run,null); assert.equal(leases.size,1);
  await host.stop(); await other.start({}); await other.stop(); assert.equal(leases.size,0);
});

test('messages require current run, source and origin; exit retires the run', async () => {
  const {host,send,received}=setup(); await host.start({}); const run=host.run.id;
  const data={channel:'nanolathe',run,type:'sample',payload:{tick:1}};
  send({...data,run:'stale'}); send(data,{}); send(data,host.frame.contentWindow,'https://other.example');
  assert.equal(received.length,0); send(data); assert.equal(received.length,1);
  const live=host.frame; send({...data,type:'stopped'}); assert.notEqual(host.frame,live); assert.equal(host.frame.hidden,true); assert.equal(host.run,null);
  send(data); send({channel:'nanolathe',type:'sample'}); assert.equal(received.length,2);
  await host.stop();
});

test('failed compilation clears the promise so a later launch can retry', async () => {
  const {host}=setup(); let requests=0;
  globalThis.fetch=async () => { requests++; return new Response('',{status:404}); };
  await assert.rejects(host.compiledModule(),/Missing browser build/);
  await assert.rejects(host.compiledModule(),/Missing browser build/);
  assert.equal(requests,2);
});

test('one compiled module and its matching runtime are reused across starts', async () => {
  const {host}=setup(); let requests=0;
  globalThis.fetch=async url=>{requests++;return String(url).endsWith('/build.json') ? Response.json({schema:1,wasm:'nanolathe.0123456789abcdef.wasm',runtime:'wasm_exec.0123456789abcdef.js',version:'authored fixture'}) : new Response(new Uint8Array([0,97,115,109,1,0,0,0]));};
  const [a,b]=await Promise.all([host.compiledModule(),host.compiledModule()]);
  assert.equal(a.module,b.module);assert.equal(new URL(a.runtime).pathname.split('/').pop(),'wasm_exec.0123456789abcdef.js');assert.equal(requests,2);
  await host.start({module:Promise.resolve(a)});await host.stop();
  assert.equal((await host.compiledModule()).module,a.module);assert.equal(requests,2);
});

test('the gzip variant is decompressed in the page; transparently decoded bytes fall back to the plain module', async () => {
  const {gzipSync}=await import('node:zlib');
  const wasm=new Uint8Array([0,97,115,109,1,0,0,0]), gz=gzipSync(wasm);
  const build={schema:1,wasm:'nanolathe.0123456789abcdef.wasm',wasm_gz:'nanolathe.0123456789abcdef.wasm.gz',runtime:'wasm_exec.0123456789abcdef.js',version:'authored fixture'};
  const serve=gzBody=>{const urls=[]; globalThis.fetch=async url=>{const name=String(url).split('/').pop(); urls.push(name);
    if(name==='build.json') return Response.json(build); return new Response(name.endsWith('.gz') ? gzBody : wasm);}; return urls;};
  const {host}=setup(); let urls=serve(gz);
  assert.ok((await host.compiledModule()).module instanceof WebAssembly.Module);
  assert.deepEqual(urls,['build.json','nanolathe.0123456789abcdef.wasm.gz']);
  const decoded=new BrowserHost(makeFrame([]),()=>{}); urls=serve(wasm);
  assert.ok((await decoded.compiledModule()).module instanceof WebAssembly.Module);
  assert.deepEqual(urls,['build.json','nanolathe.0123456789abcdef.wasm.gz','nanolathe.0123456789abcdef.wasm']);
});

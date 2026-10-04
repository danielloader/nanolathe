import test from 'node:test';
import assert from 'node:assert/strict';
import {collectEntry} from './folder.js';

const file = (name, bytes) => ({name, isFile:true, file(resolve){resolve(new Blob([bytes]));}});
function directory(name, batches) {
  return {name,isFile:false,createReader(){let index=0;return {readEntries(resolve){resolve(batches[index++] || []);}};}};
}

test('a directory drop drains every reader batch and preserves nested names', async () => {
  const root=directory('TotalA',[[file('totala1.hpi','first')],[directory('maps',[[file('custom.ufo','second')]])],[]]);
  const files=await collectEntry(root);
  assert.deepEqual(files.map(([name])=>name),['TotalA/totala1.hpi','TotalA/maps/custom.ufo']);
  assert.equal(await files[1][1].text(),'second');
});

test('an unreadable dropped file rejects the whole directory rather than mounting a partial install', async () => {
  const unreadable={name:'totala1.hpi',isFile:true,file(resolve,reject){reject(new Error('permission denied'));}};
  await assert.rejects(collectEntry(directory('TotalA',[[file('readme.txt','okay'),unreadable]])),/permission denied/);
  const badDirectory={name:'maps',createReader(){return {readEntries(resolve,reject){reject(new Error('directory unavailable'));}};}};
  await assert.rejects(collectEntry(badDirectory),/directory unavailable/);
});

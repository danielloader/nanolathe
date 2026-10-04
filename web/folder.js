export async function collectEntry(entry, prefix = '') {
  if (entry.isFile) return [[prefix + entry.name, await new Promise((resolve,reject) => entry.file(resolve,reject))]];
  const reader = entry.createReader(); let children = [];
  for (;;) { const batch = await new Promise((resolve,reject) => reader.readEntries(resolve,reject)); if (!batch.length) break; children.push(...batch); }
  const files = [];
  for (const child of children) files.push(...await collectEntry(child, prefix + entry.name + '/'));
  return files;
}

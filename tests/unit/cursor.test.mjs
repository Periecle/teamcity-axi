import {test} from 'node:test';
import assert from 'node:assert/strict';
import {encodeCursor,decodeCursor,assertCursor} from '../../dist/adapter/cursor.js';
const now=1790938800000,binding={command:'run.list',server:'work',filterHash:'a'.repeat(64),count:20,window:{since:'2026-09-25T08:00:00.000Z',until:'2026-10-02T08:00:00.000Z'}};
const cursor={...binding,version:1,position:20,expiresAt:now+1800000,window:{since:'2026-09-25T08:00:00.000Z',until:'2026-10-02T08:00:00.000Z'}};
test('cursor binds command/server/filters and reconstructible page position without raw URL',()=>{
  const token=encodeCursor(cursor,now);assert.deepEqual(decodeCursor(token,now),cursor);assertCursor(cursor,binding);
  for(const patch of [{command:'job.list'},{server:'other'},{filterHash:'b'.repeat(64)},{count:100}])assert.throws(()=>assertCursor(cursor,{...binding,...patch}),e=>e.code==='USAGE_ERROR');
  assert.throws(()=>assertCursor({...cursor,window:{since:'1970-01-01T00:00:00.000Z',until:'2099-01-01T00:00:00.000Z'}},binding),e=>e.code==='USAGE_ERROR');
  for(const bad of ['https://evil.example/api',token+'=',token+'\n','x'.repeat(4097),Buffer.from([255]).toString('base64url')])assert.throws(()=>decodeCursor(bad,now),e=>e.code==='USAGE_ERROR');
  for(const patch of [{position:0},{position:5000},{position:1.5},{expiresAt:now},{expiresAt:now+1800001},{token:'credential'},{providerUrl:'https://evil.example'},{window:{since:'2026-02-30T00:00:00.000Z',until:cursor.window.until}},{window:{since:cursor.window.since,until:cursor.window.until,extra:true}}])assert.throws(()=>decodeCursor(Buffer.from(JSON.stringify({...cursor,...patch})).toString('base64url'),now),e=>e.code==='USAGE_ERROR');
});

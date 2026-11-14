import { test } from 'node:test'
import assert from 'node:assert/strict'
import { clipMetroPath } from '../src/metro-geometry.ts'

const outer = [[0,0],[10,0],[10,10],[0,10],[0,0]]
test('district clipping keeps crossings even when both stations are outside', () => {
  assert.deepEqual(clipMetroPath([[-2,5],[12,5]], [[outer]]), [[[0,5],[10,5]]])
  assert.deepEqual(clipMetroPath([[-2,15],[12,15]], [[outer]]), [])
})
test('district clipping removes holes and keeps detached islands', () => {
  const hole = [[4,4],[6,4],[6,6],[4,6],[4,4]]
  const island = [[20,0],[22,0],[22,10],[20,10],[20,0]]
  const paths = clipMetroPath([[-2,5],[24,5]], [[outer,hole],[island]])
  assert.equal(paths.length,3)
  const rounded = paths.map(path => path.map(p=>p.map(v=>Math.round(v*1e8)/1e8)))
  assert.deepEqual(rounded, [[[0,5],[4,5]],[[6,5],[10,5]],[[20,5],[22,5]]])
})

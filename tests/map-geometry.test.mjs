import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { topology } from 'topojson-server'
import { feature, transform } from 'topojson-client'

const source = JSON.parse(await readFile('data/maps/100000.json', 'utf8'))
const original = topology({ regions: source })
const rings = geometry => geometry.type === 'Polygon' ? geometry.coordinates : geometry.coordinates.flat()
const area = ring => ring.reduce((sum, p, i) => i ? sum + ring[i - 1][0] * p[1] - p[0] * ring[i - 1][1] : sum, 0) / 2
function distance(p, a, b) {
  const dx = b[0] - a[0], dy = b[1] - a[1]
  const t = dx || dy ? Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / (dx * dx + dy * dy))) : 0
  return Math.hypot(p[0] - a[0] - t * dx, p[1] - a[1] - t * dy)
}
for (const [level, tolerance] of [['overview', .03], ['detail', .003]]) {
  test(`${level} retains provinces, islands, holes, winding and bounded error on shared arcs`, async () => {
    const text = await readFile(`src/assets/china-${level}.topo.json`, 'utf8')
    const compact = JSON.parse(text), decoded = feature(compact, compact.objects.regions)
    assert.equal(decoded.features.length, source.features.length)
    assert.equal(decoded.features.filter(f => /^\d{6}$/.test(String(f.properties.adcode))).length, 34)
    decoded.features.forEach((f, i) => {
      assert.deepEqual(f.properties, source.features[i].properties)
      assert.equal(f.geometry.type, source.features[i].geometry.type)
      const before = rings(source.features[i].geometry), after = rings(f.geometry)
      assert.equal(after.length, before.length, f.properties.name)
      after.forEach((ring, j) => {
        assert.ok(new Set(ring.map(p => p.join(','))).size >= 3, `${f.properties.name}: collapsed ring`)
        assert.deepEqual(ring[0], ring.at(-1))
        assert.equal(Math.sign(area(ring)), Math.sign(area(before[j])), `${f.properties.name}: flipped ring`)
      })
    })
    assert.equal(compact.arcs.length, original.arcs.length)
    compact.arcs.forEach((arc, i) => {
      const decode = transform(compact.transform)
      const points = arc.map((p, j) => decode(p, j))
      assert.ok(distance(original.arcs[i][0], points[0], points[0]) < .0001)
      assert.ok(distance(original.arcs[i].at(-1), points.at(-1), points.at(-1)) < .0001)
      for (const p of original.arcs[i]) {
        let nearest = Infinity
        for (let j = 1; j < points.length; j++) nearest = Math.min(nearest, distance(p, points[j - 1], points[j]))
        assert.ok(nearest <= tolerance + .0001, `arc ${i}: ${nearest} degrees exceeds error bound`)
      }
    })
    if (level === 'overview') {
      assert.ok(Buffer.byteLength(text) < Buffer.byteLength(JSON.stringify(source)) * .2)
      assert.ok(compact.arcs.reduce((n, a) => n + a.length, 0) < original.arcs.reduce((n, a) => n + a.length, 0) * .5)
    }
  })
}

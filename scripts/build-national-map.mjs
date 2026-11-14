import { readFile, mkdir, writeFile } from 'node:fs/promises'
import { topology } from 'topojson-server'
import { feature, quantize } from 'topojson-client'

// Simplify shared arcs, rather than independently simplifying neighboring provinces.
// Douglas–Peucker bounds vertex displacement in degrees; closed islands keep rings.
function distanceSquared(p, a, b) {
  const dx = b[0] - a[0], dy = b[1] - a[1]
  const t = dx || dy ? Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / (dx * dx + dy * dy))) : 0
  return (p[0] - a[0] - t * dx) ** 2 + (p[1] - a[1] - t * dy) ** 2
}
function simplifyLine(points, tolerance) {
  const keep = new Set([0, points.length - 1]), stack = [[0, points.length - 1]]
  while (stack.length) {
    const [first, last] = stack.pop()
    let max = tolerance ** 2, split = -1
    for (let i = first + 1; i < last; i++) {
      const distance = distanceSquared(points[i], points[first], points[last])
      if (distance > max) { max = distance; split = i }
    }
    if (split !== -1) { keep.add(split); stack.push([first, split], [split, last]) }
  }
  return points.filter((_, index) => keep.has(index))
}
function simplifyArc(arc, tolerance) {
  if (arc.length < 4) return arc
  const first = arc[0], last = arc.at(-1)
  if (first[0] !== last[0] || first[1] !== last[1]) return simplifyLine(arc, tolerance)
  let opposite = 1
  for (let i = 2; i < arc.length - 1; i++) {
    if (distanceSquared(arc[i], first, first) > distanceSquared(arc[opposite], first, first)) opposite = i
  }
  const result = [...simplifyLine(arc.slice(0, opposite + 1), tolerance).slice(0, -1), ...simplifyLine(arc.slice(opposite), tolerance)]
  return result.length >= 4 ? result : arc
}
function rings(geometry) {
  return geometry.type === 'Polygon' ? geometry.coordinates : geometry.coordinates.flat()
}
function area(ring) {
  return ring.reduce((sum, p, i) => i ? sum + ring[i - 1][0] * p[1] - p[0] * ring[i - 1][1] : sum, 0) / 2
}
const source = JSON.parse(await readFile('data/maps/100000.json', 'utf8'))
const original = topology({ regions: source })
await mkdir('src/assets', { recursive: true })
for (const [level, tolerance] of [['overview', .03], ['detail', .003]]) {
  const simplified = { ...original, arcs: original.arcs.map(arc => simplifyArc(arc, tolerance)) }
  // Protect small islands/holes assembled from several arcs from collapsing or flipping.
  const decoded = feature(simplified, simplified.objects.regions)
  decoded.features.forEach((f, i) => {
    rings(f.geometry).forEach((ring, j) => {
      const before = area(rings(source.features[i].geometry)[j])
      if (!area(ring) || Math.sign(area(ring)) !== Math.sign(before)) {
        const object = original.objects.regions.geometries[i]
        const refs = object.type === 'Polygon' ? object.arcs[j] : object.arcs.flat()[j]
        for (const ref of refs) { const index = ref < 0 ? ~ref : ref; simplified.arcs[index] = original.arcs[index] }
      }
    })
  })
  const result = quantize(simplified, 1e6)
  const text = JSON.stringify(result)
  await writeFile(`src/assets/china-${level}.topo.json`, text + '\n')
  console.log(`${level}: ${original.arcs.reduce((n, a) => n + a.length, 0)} -> ${simplified.arcs.reduce((n, a) => n + a.length, 0)} shared vertices; ${Buffer.byteLength(text)} bytes`)
}

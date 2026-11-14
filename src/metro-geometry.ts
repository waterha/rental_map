import type { Position } from './amap'

// Clip station connections to all district polygons, including holes. Intersect
// first so connections with both endpoints outside can still cross the district.
export function clipMetroPath(points: Position[], polygons: Position[][][]): Position[][] {
  const contains = ([x, y]: Position) => polygons.some(polygon => {
    const inRing = (ring: Position[]) => {
      let inside = false
      for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
        const [ax, ay] = ring[i], [bx, by] = ring[j]
        if ((ay > y) !== (by > y) && x < (bx - ax) * (y - ay) / (by - ay) + ax) inside = !inside
      }
      return inside
    }
    return inRing(polygon[0]) && !polygon.slice(1).some(inRing)
  })
  const result: Position[][] = []
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1], b = points[i], dx = b[0] - a[0], dy = b[1] - a[1]
    const cuts = [0, 1]
    for (const polygon of polygons) for (const ring of polygon) for (let j = 1; j < ring.length; j++) {
      const c = ring[j - 1], d = ring[j], ex = d[0] - c[0], ey = d[1] - c[1]
      const divisor = dx * ey - dy * ex
      if (Math.abs(divisor) < 1e-14) continue
      const t = ((c[0] - a[0]) * ey - (c[1] - a[1]) * ex) / divisor
      const u = ((c[0] - a[0]) * dy - (c[1] - a[1]) * dx) / divisor
      if (t > 0 && t < 1 && u >= 0 && u <= 1) cuts.push(t)
    }
    const sorted = [...new Set(cuts)].sort((x, y) => x - y)
    const at = (t: number): Position => [a[0] + dx * t, a[1] + dy * t]
    for (let j = 1; j < sorted.length; j++) {
      const start = sorted[j - 1], end = sorted[j]
      if (end - start < 1e-12 || !contains(at((start + end) / 2))) continue
      const from = at(start), to = at(end), previous = result.at(-1)
      if (previous && Math.hypot(previous.at(-1)![0] - from[0], previous.at(-1)![1] - from[1]) < 1e-10) previous.push(to)
      else result.push([from, to])
    }
  }
  return result
}

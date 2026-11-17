import { writeFile } from 'node:fs/promises'

// Maintainer-only refresh. Browsers and production startup never call this URL.
const response = await fetch('https://geo.datav.aliyun.com/areas_v3/bound/all.json', { signal: AbortSignal.timeout(25000) })
if (!response.ok) throw new Error(`Region index: HTTP ${response.status}`)
const source = await response.json()
if (!Array.isArray(source) || source.length < 3000 || source.length > 10000) throw new Error('Unexpected region count')
const codes = new Set(source.map(item => String(item.adcode)))
const parents = new Set(source.map(item => String(item.parent)))
const regions = source.filter(item => item.level !== 'country').map(item => {
  const code = String(item.adcode), parent = String(item.parent)
  if (!/^\d{6}$/.test(code) || !codes.has(parent) || typeof item.name !== 'string' || item.name.length > 40 ||
    !['province', 'city', 'district'].includes(item.level) || !Number.isFinite(item.lng) || !Number.isFinite(item.lat) ||
    item.lng < 70 || item.lng > 140 || item.lat < 0 || item.lat > 60) throw new Error(`Invalid region: ${code}`)
  return { code, name: item.name, level: item.level, parent, center: [item.lng, item.lat], hasChildren: parents.has(code) }
}).sort((a, b) => a.code.localeCompare(b.code))
if (new Set(regions.map(item => item.code)).size !== regions.length) throw new Error('Duplicate region codes')
await writeFile(new URL('../internal/atlas/region-index.json', import.meta.url), JSON.stringify(regions) + '\n')
console.log(`Bundled ${regions.length} regions; no API key used.`)

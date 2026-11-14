import { mkdir, writeFile, readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const mapDir = path.join(root, 'data', 'maps')
await mkdir(mapDir, { recursive: true })
await mkdir(path.join(root, 'data', 'images'), { recursive: true })
const regions = new Map()
async function download(code) {
  const destination = path.join(mapDir, `${code}.json`)
  let data
  try { data = JSON.parse(await readFile(destination, 'utf8')) } catch {
    let last
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        const response = await fetch(`https://geo.datav.aliyun.com/areas_v3/bound/${code}_full.json`, { signal: AbortSignal.timeout(25000) })
        if (!response.ok) throw new Error(`${code}: HTTP ${response.status}`)
        data = await response.json()
        if (!data.features?.length) throw new Error(`Invalid GeoJSON: ${code}`)
        await writeFile(destination, JSON.stringify(data))
        break
      } catch (error) { last = error }
    }
    if (!data) throw last
  }
  for (const feature of data.features) {
    const p = feature.properties
    if (!/^\d{6}$/.test(String(p.adcode)) || p.adcode === 100000) continue
    const item = { code: String(p.adcode), name: p.name, level: p.level, parent: String(code), center: p.center || p.centroid || [0, 0], hasChildren: p.childrenNum > 0 }
    regions.set(item.code, item)
  }
  return data
}
const country = await download('100000')
const provinceCodes = country.features.filter(f => f.properties.childrenNum > 0).map(f => String(f.properties.adcode)).filter(c => /^\d{6}$/.test(c) && c !== '100000')
const cityCodes = ['440100', '440300', '330100', '320100', '320500', '510100', '420100', '610100', '350200', '370200', '430100', '410100', '340100', '530100', '350100', '210200']
const tasks = [...new Set([...provinceCodes, ...cityCodes])]
let cursor = 0
await Promise.all(Array.from({ length: 5 }, async () => {
  while (cursor < tasks.length) {
    const code = tasks[cursor++]
    try { await download(code); console.log(`Map ${code} cached`) } catch (error) { console.error(`Map ${code}: ${error.message}`) }
  }
}))
await writeFile(path.join(root, 'data', 'regions.json'), JSON.stringify([...regions.values()]))
const photos = ['photo-1600210492486-724fe5c67fb0', 'photo-1600607687920-4e2a09cf159d', 'photo-1600566753086-00f18fb6b3ea', 'photo-1600607687939-ce8a6c25118c', 'photo-1522708323590-d24dbb6b0267', 'photo-1502672260266-1c1ef2d93688', 'photo-1616486338812-3dadae4b4ace', 'photo-1600607688969-a5bfcd646154']
await Promise.all(photos.map(async (id, i) => {
  const file = path.join(root, 'data', 'images', `home-${i + 1}.jpg`)
  try { await readFile(file); return } catch {}
  try {
    const response = await fetch(`https://images.unsplash.com/${id}?auto=format&fit=crop&w=900&q=85`, { signal: AbortSignal.timeout(30000) })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    await writeFile(file, Buffer.from(await response.arrayBuffer()))
    console.log(`Photo ${i + 1} cached`)
  } catch (error) { console.error(`Photo ${i + 1}: ${error.message}`) }
}))
console.log(`${regions.size} regions indexed; ${tasks.length + 1} maps requested.`)

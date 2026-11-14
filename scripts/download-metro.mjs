import { mkdir, writeFile } from 'node:fs/promises'

// Public AMap subway data; no application key or personal configuration is used.
const base = 'https://webapi.amap.com/subway/data/'
async function json(path) {
  const response = await fetch(base + path, { signal: AbortSignal.timeout(15000) })
  if (!response.ok) throw new Error(`Subway data returned ${response.status}`)
  return response.json()
}
await mkdir('data/metro', { recursive: true })
const { citylist } = await json('citylist.json')
await writeFile('data/metro/cities.json', JSON.stringify(citylist) + '\n')
const requested = process.argv.slice(2)
for (const city of citylist.filter(city => (requested.length ? requested : ['330100']).includes(city.adcode + '00'))) {
  const source = await json(`${city.adcode}_drw_${city.spell}.json`)
  const lines = source.l.filter(line => line.su === '1').map(line => ({
    id: line.ls, name: line.ln + (line.la ? `（${line.la}）` : ''), color: /^[a-f0-9]{6}$/i.test(line.cl) ? `#${line.cl}` : '#4cddff', loop: line.lo === '1',
    stations: line.st.map(station => ({ id: station.si || station.sid, name: station.n, position: station.sl.split(',').map(Number), transfer: station.t === '1' })),
  })).filter(line => line.stations.length >= 2)
  await writeFile(`data/metro/${city.adcode}00.json`, JSON.stringify({ cityCode: city.adcode + '00', cityName: city.cityname, source: '高德地铁', sourceUrl: base + `${city.adcode}_drw_${city.spell}.json`, fetchedAt: new Date().toISOString(), schematic: true, lines }) + '\n')
  console.log(`${city.cityname}: ${lines.length} lines`)
}

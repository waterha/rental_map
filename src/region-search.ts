import type { Region } from './types'

const normalize = (value: string) => value.normalize('NFKC').toLowerCase().replace(/[\s/／·,，]+/g, '')
const shortName = (value: string) => value.replace(/壮族自治区$|回族自治区$|维吾尔自治区$|特别行政区$|自治区$|自治州$|地区$|省$|市$|区$|县$/u, '')

export interface RegionMatch { region: Region; path: string }

export function buildRegionSearch(regions: Region[]) {
  const byCode = new Map(regions.map(region => [region.code, region]))
  const entries = regions.filter(region => region.level !== 'country').map(region => {
    const chain = [region]
    const seen = new Set([region.code])
    let parent = byCode.get(region.parent)
    while (parent && parent.level !== 'country' && !seen.has(parent.code)) {
      chain.unshift(parent); seen.add(parent.code); parent = byCode.get(parent.parent)
    }
    const name = normalize(region.name)
    return { region, name, short: normalize(shortName(region.name)), path: chain.map(item => item.name).join(' / '),
      full: normalize(chain.map(item => item.name).join('')), brief: normalize(chain.map(item => shortName(item.name)).join('')) }
  })
  return (input: string): RegionMatch[] => {
    const query = normalize(input.trim().slice(0, 80))
    if (!query) return []
    return entries.map(entry => {
      const score = entry.region.code === query ? 0 : entry.name === query || entry.short === query ? 1
        : entry.name.startsWith(query) ? 2 : entry.name.includes(query) ? 3
          : entry.full.includes(query) || entry.brief.includes(query) ? 4 : 10
      return { entry, score }
    }).filter(item => item.score < 10).sort((a, b) => a.score - b.score || a.entry.region.code.localeCompare(b.entry.region.code))
      .slice(0, 12).map(({ entry }) => ({ region: entry.region, path: entry.path }))
  }
}

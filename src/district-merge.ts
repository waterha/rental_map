import type { Region } from './types'
import type { MapCollection, MapFeature } from './map-data'
import type { Position } from './amap'

export const maxMergedDistricts = 16
export interface DistrictMerge { city: Region; codes: string[]; phase: 'select' | 'view' }

export function readDistrictMerge(url: URL, regions: Region[]): DistrictMerge | null {
  const codes = url.searchParams.get('districts')?.split(',') || []
  if (codes.length < 2 || codes.length > maxMergedDistricts || new Set(codes).size !== codes.length) return null
  const city = regions.find(region => region.code === url.searchParams.get('region'))
  if (!city || !['city', 'province'].includes(city.level)) return null
  if (!codes.every(code => regions.some(region => region.code === code && region.level === 'district' && region.parent === city.code))) return null
  return { city, codes: codes.sort(), phase: 'view' }
}

// Keep the original polygons (including islands and holes). AMap masks, fitting,
// point containment and metro clipping all receive the same combined geometry.
export function mergedDistrictFeature(collection: MapCollection | null, codes: string[]): MapFeature | undefined {
  if (!collection || codes.length < 2) return undefined
  const features = codes.map(code => collection.features.find(feature => String(feature.properties.adcode) === code))
  if (features.some(feature => !feature || !['Polygon', 'MultiPolygon'].includes(feature.geometry.type))) return undefined
  const coordinates: Position[][][] = []
  for (const feature of features) {
    if (feature!.geometry.type === 'Polygon') coordinates.push(feature!.geometry.coordinates as Position[][])
    if (feature!.geometry.type === 'MultiPolygon') coordinates.push(...feature!.geometry.coordinates as Position[][][])
  }
  return { type: 'Feature', properties: { adcode: codes.join(','), name: features.map(feature => feature!.properties.name).join('、') },
    geometry: { type: 'MultiPolygon', coordinates } }
}

import { useCallback, useEffect, useState } from 'react'
import type { Feature, FeatureCollection, Geometry, MultiLineString, MultiPolygon, Polygon } from 'geojson'
import { api } from './types'
import type { Region } from './types'
import { loadNationalMap } from './national-map'

export interface Properties { adcode: string | number; name: string; center?: [number, number]; centroid?: [number, number]; childrenNum?: number; level?: Region['level'] }
export type MapFeature = Feature<Geometry, Properties>
export interface MapCollection extends FeatureCollection<Geometry, Properties> { boundaryMesh?: MultiLineString }
const cache = new Map<string, MapCollection>()

export function useMapCollection(code: string) {
  const [result, setResult] = useState<{ code: string; data: MapCollection } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const retry = useCallback(() => setAttempt(value => value + 1), [])
  useEffect(() => {
    const abort = new AbortController()
    setError('')
    if (cache.has(code)) { setResult({ code, data: cache.get(code)! }); setLoading(false); return () => abort.abort() }
    setLoading(true)
    const request = code === '100000' ? loadNationalMap() : api<MapCollection>(`/api/maps/${code}`, { signal: abort.signal })
    request.then(data => {
      cache.set(code, data)
      if (!abort.signal.aborted) setResult({ code, data })
    }).catch(error => { if (!abort.signal.aborted) setError(error.message) }).finally(() => { if (!abort.signal.aborted) setLoading(false) })
    return () => abort.abort()
  }, [code, attempt])
  return { collection: result?.code === code ? result.data : cache.get(code) || null, loading, error, retry }
}

export function normalizeWinding(collection: MapCollection): MapCollection {
  const ring = (coordinates: number[][], exterior: boolean) => {
    let area = 0
    for (let i = 0; i < coordinates.length - 1; i++) area += coordinates[i][0] * coordinates[i + 1][1] - coordinates[i + 1][0] * coordinates[i][1]
    return (area > 0) === exterior ? [...coordinates].reverse() : coordinates
  }
  return { ...collection, features: collection.features.map(feature => {
    const geometry = feature.geometry
    if (geometry.type === 'Polygon') return { ...feature, geometry: { ...geometry, coordinates: geometry.coordinates.map((r, i) => ring(r, i === 0)) } as Polygon }
    if (geometry.type === 'MultiPolygon') return { ...feature, geometry: { ...geometry, coordinates: geometry.coordinates.map(p => p.map((r, i) => ring(r, i === 0))) } as MultiPolygon }
    return feature
  }) }
}

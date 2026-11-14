import { feature, mesh } from 'topojson-client'
import type { GeometryCollection, Topology } from 'topojson-specification'
import type { Properties, MapCollection } from './map-data'
import overviewUrl from './assets/china-overview.topo.json?url'
import detailUrl from './assets/china-detail.topo.json?url'

const pending = new Map<boolean, Promise<MapCollection>>()
export function loadNationalMap(detail = false): Promise<MapCollection> {
  let request = pending.get(detail)
  if (!request) {
    request = fetch(detail ? detailUrl : overviewUrl).then(async response => {
      if (!response.ok) throw new Error('全国地图加载失败，请重试')
      const topology = await response.json() as Topology<{ regions: GeometryCollection<Properties> }>
      return { ...feature(topology, topology.objects.regions), boundaryMesh: mesh(topology, topology.objects.regions) }
    }).catch(error => { pending.delete(detail); throw error })
    pending.set(detail, request)
  }
  return request
}

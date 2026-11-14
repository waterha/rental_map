import { memo, useEffect, useMemo, useState } from 'react'
import { geoPath, type GeoProjection } from 'd3-geo'
import { LoaderCircle, TrainFront } from 'lucide-react'
import type { Region } from './types'
import { api } from './types'
import './metro.css'

export interface MetroStation { id: string; name: string; position: [number, number]; transfer: boolean }
export interface MetroNetwork {
  cityCode: string; cityName: string; source: string; fetchedAt: string; schematic: boolean; stale: boolean
  lines: { id: string; name: string; color: string; loop: boolean; stations: MetroStation[] }[]
}
const cache = new Map<string, { data: MetroNetwork; until: number }>()
export function metroAvailable(region: Region) {
  return region.level === 'city' || region.level === 'province' && ['110000', '120000', '310000', '500000', '810000', '820000'].includes(region.code)
}
export function useMetro(code: string, enabled: boolean) {
  const [result, setResult] = useState<{ code: string; data: MetroNetwork } | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    setError('')
    if (!enabled) { setLoading(false); return }
    const cached = cache.get(code)
    if (cached && cached.until > Date.now()) { setResult({ code, data: cached.data }); setLoading(false); return }
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), 18000)
    let active = true
    setLoading(true)
    api<MetroNetwork>(`/api/metro/${code}`, { signal: controller.signal }).then(data => {
      if (!active) return
      cache.set(code, { data, until: Date.now() + (data.stale ? 5 * 60 : 60 * 60) * 1000 })
      setResult({ code, data })
    }).catch(error => { if (active) setError(controller.signal.aborted ? '地铁线路加载超时，请重试' : error.message) })
      .finally(() => { clearTimeout(timer); if (active) setLoading(false) })
    return () => { active = false; clearTimeout(timer); controller.abort() }
  }, [code, enabled, attempt])
  return { network: enabled && result?.code === code ? result.data : null, loading, error, retry: () => setAttempt(value => value + 1) }
}

export const MetroGeometry = memo(function MetroGeometry({ network, projection, zoom }: { network: MetroNetwork; projection: GeoProjection; zoom: number }) {
  const geometry = useMemo(() => {
    const path = geoPath(projection)
    const stations = new Map<string, { name: string; point: [number, number]; lines: Set<string>; transfer: boolean }>()
    const lines = network.lines.map(line => {
      const coordinates = line.stations.map(station => station.position)
      if (line.loop && coordinates.length > 2 && coordinates[0].join() !== coordinates.at(-1)!.join()) coordinates.push(coordinates[0])
      for (const station of line.stations) {
        const point = projection(station.position)
        if (!point) continue
        const key = `${station.name}:${station.position.map(v => v.toFixed(4)).join(',')}`
        const existing = stations.get(key)
        if (existing) { existing.lines.add(line.id); existing.transfer ||= station.transfer }
        else stations.set(key, { name: station.name, point, lines: new Set([line.id]), transfer: station.transfer })
      }
      return { ...line, d: path({ type: 'LineString', coordinates }) || '' }
    })
    return { lines, stations: [...stations.entries()].map(([id, station]) => ({ ...station, id })) }
  }, [network, projection])
  return <g className="metro-layer" role="group" aria-label={`${network.cityName}地铁线路`} pointerEvents="none" data-lines={network.lines.length}>
    <g className="metro-route-glow" opacity=".22">{geometry.lines.map(line => <path key={line.id} d={line.d} fill="none" stroke={line.color} strokeWidth="9" vectorEffect="non-scaling-stroke" />)}</g>
    {geometry.lines.map(line => <path key={line.id} className="metro-route" data-line={line.id} d={line.d} fill="none" stroke={line.color} strokeWidth="2.6" vectorEffect="non-scaling-stroke"><title>{line.name}（站点连线示意）</title></path>)}
    {zoom >= 1.8 && geometry.stations.map(station => <g key={station.id}>
      <circle className="metro-station" cx={station.point[0]} cy={station.point[1]} r={(station.transfer || station.lines.size > 1 ? 3.1 : 1.9) / zoom} stroke="#d3f8ff" strokeWidth="1" fill="#0a2639" vectorEffect="non-scaling-stroke"><title>{station.name}</title></circle>
      {zoom >= 2.5 && <text className="metro-station-label" x={station.point[0] + 6 / zoom} y={station.point[1] - 5 / zoom} fontSize={10 / zoom} strokeWidth={2.5 / zoom}>{station.name}</text>}
    </g>)}
  </g>
})

export function MetroControl({ enabled, onToggle, network, loading, error, retry }: {
  enabled: boolean; onToggle: () => void; network: MetroNetwork | null; loading: boolean; error: string; retry: () => void
}) {
  return <div className="metro-tools">
    <button className={`metro-toggle ${enabled ? 'active' : ''}`} aria-label="地铁图层" aria-pressed={enabled} title={enabled ? '关闭地铁图层' : '显示地铁图层'} onClick={onToggle}>
      {loading ? <LoaderCircle size={19} className="spin" /> : <TrainFront size={19} />}
    </button>
    {enabled && <div className="metro-status" role={error ? 'alert' : 'status'}>
      {loading ? '正在加载地铁线路' : error ? <>{error}<button onClick={retry}>重试</button></> : network && !network.lines.length ? '该城市暂无收录的地铁线路' : network ? <>
        <span>地铁 · {network.lines.length} 条线路</span>
        <small>{network.source} · 站点连线示意{network.stale ? ' · 离线缓存' : ''}</small>
        <small>数据获取于 {new Date(network.fetchedAt).toLocaleDateString('zh-CN')}</small>
        <div className="metro-legend">{network.lines.map(line => <span key={line.id}><i style={{ background: line.color }} />{line.name}</span>)}</div>
      </> : null}
    </div>}
  </div>
}

import { useEffect, useState } from 'react'
import { LoaderCircle, TrainFront } from 'lucide-react'
import { api } from './types'
import './metro.css'

export interface MetroStation { id: string; name: string; position: [number, number]; transfer: boolean }
export interface MetroNetwork {
  cityCode: string; cityName: string; source: string; fetchedAt: string; schematic: boolean; stale: boolean
  lines: { id: string; name: string; color: string; loop: boolean; stations: MetroStation[] }[]
}
const cache = new Map<string, { data: MetroNetwork; until: number }>()
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

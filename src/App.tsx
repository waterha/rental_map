import { useCallback, useEffect, useState } from 'react'
import { LoaderCircle, RotateCcw, Plus, X } from 'lucide-react'
import MapView from './MapView'
import DistrictRentals from './DistrictRentals'
import type { Region } from './types'
import { api, national } from './types'
import { useMapCollection } from './map-data'
import AccountPanel from './AccountPanel'
import type { Account } from './AccountPanel'
import './publication.css'

const currentCode = () => new URLSearchParams(location.search).get('region') || national.code

export default function App() {
  const [user, setUser] = useState<Account | null>(null)
  const [wantsPublish, setWantsPublish] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [loginRequest, setLoginRequest] = useState(0)
  const cancelPublication = useCallback(() => { setPublishing(false); setWantsPublish(false) }, [])
  useEffect(() => { if (wantsPublish && user) setPublishing(true) }, [wantsPublish, user])
  const [regions, setRegions] = useState<Region[]>([national])
  const [region, setRegion] = useState(national)
  const [ready, setReady] = useState(false)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const isDistrict = region.level === 'district'
  const { collection, loading, error: mapError, retry } = useMapCollection(isDistrict || !region.hasChildren ? region.parent : region.code)
  const refreshRegions = useCallback(async () => {
    const items = await api<Region[]>('/api/regions')
    setRegions(items)
    return items
  }, [])

  useEffect(() => {
    const abort = new AbortController()
    setError('')
    api<Region[]>('/api/regions', { signal: abort.signal }).then(items => {
      setRegions(items)
      setRegion(items.find(item => item.code === currentCode()) || national)
      setReady(true)
    }).catch(error => { if (!abort.signal.aborted) setError(error.message) })
    return () => abort.abort()
  }, [reload])
  useEffect(() => { if (collection) void refreshRegions().catch(() => {}) }, [collection, refreshRegions])
  useEffect(() => {
    const popstate = () => setRegion(regions.find(item => item.code === currentCode()) || national)
    window.addEventListener('popstate', popstate)
    return () => window.removeEventListener('popstate', popstate)
  }, [regions])
  function navigate(next: Region) {
    setRegions(previous => previous.some(item => item.code === next.code) ? previous : [...previous, next])
    setRegion(next)
    const url = new URL(location.href)
    if (next.code === national.code) url.searchParams.delete('region')
    else url.searchParams.set('region', next.code)
    if (url.href !== location.href) history.pushState({}, '', url)
  }
  const ancestors: Region[] = []
  let parent = regions.find(item => item.code === region.parent)
  const seen = new Set<string>([region.code])
  while (parent && !seen.has(parent.code)) {
    seen.add(parent.code); ancestors.unshift(parent)
    parent = regions.find(item => item.code === parent!.parent)
  }
  const trail = region.code === national.code ? [national] : [...ancestors.filter(item => item.code !== national.code), region]
  if (trail[0]?.code !== national.code) trail.unshift(national)
  const districtFeature = collection?.features.find(feature => String(feature.properties.adcode) === region.code)

  return <main className="atlas-fullscreen" aria-label="租房版图">
    <AccountPanel onUserChange={setUser} loginRequest={loginRequest} onLoginCancel={cancelPublication} />
    <button className="publish-trigger" aria-label="发布房源" title="发布房源" aria-pressed={publishing} onClick={() => { setWantsPublish(true); if (user) setPublishing(true); if (!user) setLoginRequest(value => value + 1) }}><Plus size={21} /></button>
    {publishing && !isDistrict && <div className="publication-guide" role="status"><span>请选择房源所在的省、市、区县，再在街道地图上点选位置。</span><button onClick={cancelPublication} aria-label="取消发布"><X size={16} /></button></div>}
    {ready && (isDistrict
      ? <DistrictRentals key={region.code} region={region} feature={districtFeature} publishing={publishing} onPublishEnd={cancelPublication} onLoginRequired={() => setLoginRequest(value => value + 1)} />
      : <MapView region={region} collection={collection} onNavigate={navigate} />)}
    {ready && region.code !== national.code && <nav className="map-breadcrumbs" aria-label="当前行政区路径">{trail.map((item, index) => <span key={item.code}>{index > 0 && <i aria-hidden="true">/</i>}{index === trail.length - 1 ? <strong aria-current="location">{item.code === national.code ? '中国' : item.name}</strong> : <button onClick={() => navigate(item)}>{item.code === national.code ? '中国' : item.name}</button>}</span>)}</nav>}
    {(!ready || loading) && !error && !mapError && <div className="map-loading" role="status"><LoaderCircle size={20} className="spin" /><span>正在展开地图</span></div>}
    {(error || mapError) && <div className="map-notice" role="alert"><p>{error || mapError}</p><button onClick={() => { retry(); setReload(value => value + 1) }}><RotateCcw size={14} />重新加载</button></div>}
  </main>
}

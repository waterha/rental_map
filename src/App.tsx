import { useCallback, useEffect, useMemo, useState } from 'react'
import { LoaderCircle, RotateCcw, Plus, X } from 'lucide-react'
import MapView from './MapView'
import DistrictRentals from './DistrictRentals'
import type { Region } from './types'
import { api, national } from './types'
import { useMapCollection } from './map-data'
import AccountPanel from './AccountPanel'
import type { Account } from './AccountPanel'
import './publication.css'
import RegionSearch from './RegionSearch'
import DistrictMergeControl from './DistrictMergeControl'
import type { DistrictMerge } from './district-merge'
import { maxMergedDistricts, mergedDistrictFeature, readDistrictMerge } from './district-merge'

const currentCode = () => new URLSearchParams(location.search).get('region') || national.code
function writeLocation(region: Region, districts?: string[]) {
  const url = new URL(location.href)
  if (region.code === national.code) url.searchParams.delete('region')
  else url.searchParams.set('region', region.code)
  if (districts) url.searchParams.set('districts', districts.join(','))
  else url.searchParams.delete('districts')
  if (url.href !== location.href) history.pushState({}, '', url)
}

export default function App() {
  const [user, setUser] = useState<Account | null>(null)
  const [wantsPublish, setWantsPublish] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [loginRequest, setLoginRequest] = useState(0)
  const cancelPublication = useCallback(() => { setPublishing(false); setWantsPublish(false) }, [])
  useEffect(() => { if (wantsPublish && user) setPublishing(true) }, [wantsPublish, user])
  const [regions, setRegions] = useState<Region[]>([national])
  const [region, setRegion] = useState(national)
  const [merge, setMerge] = useState<DistrictMerge | null>(null)
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
      setMerge(previous => previous?.phase === 'select' && previous.city.code === currentCode() ? previous : readDistrictMerge(new URL(location.href), items))
      setReady(true)
    }).catch(error => { if (!abort.signal.aborted) setError(error.message) })
    return () => abort.abort()
  }, [reload])
  useEffect(() => { if (collection) void refreshRegions().catch(() => {}) }, [collection, refreshRegions])
  useEffect(() => {
    const popstate = () => {
      setRegion(regions.find(item => item.code === currentCode()) || national)
      const restored = readDistrictMerge(new URL(location.href), regions)
      setMerge(restored)
      if (restored) cancelPublication()
    }
    window.addEventListener('popstate', popstate)
    return () => window.removeEventListener('popstate', popstate)
  }, [regions, cancelPublication])
  function navigate(next: Region) {
    setRegions(previous => previous.some(item => item.code === next.code) ? previous : [...previous, next])
    setRegion(next)
    setMerge(null)
    writeLocation(next)
  }
  const exitMerge = useCallback(() => {
    if (!merge) return
    setMerge(null); setRegion(merge.city); writeLocation(merge.city)
  }, [merge])
  useEffect(() => {
    if (!merge) return
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.isComposing || document.querySelector('[aria-modal="true"]')) return
      event.preventDefault(); event.stopImmediatePropagation(); exitMerge()
    }
    document.addEventListener('keydown', escape, true)
    return () => document.removeEventListener('keydown', escape, true)
  }, [merge, exitMerge])
  const availableDistricts = useMemo(() => {
    if (!merge || !collection) return []
    const codes = new Set(collection.features.filter(feature => ['Polygon', 'MultiPolygon'].includes(feature.geometry.type)).map(feature => String(feature.properties.adcode)))
    return regions.filter(item => item.level === 'district' && item.parent === merge.city.code && codes.has(item.code))
  }, [merge?.city.code, collection, regions])
  const mergedFeature = useMemo(() => merge?.phase === 'view' ? mergedDistrictFeature(collection, merge.codes) : undefined, [collection, merge])
  const primaryDistrict = merge?.phase === 'view' ? regions.find(item => item.code === merge.codes[0]) : undefined
  const rentalRegion = useMemo(() => primaryDistrict ? { ...primaryDistrict, name: mergedFeature?.properties.name || primaryDistrict.name } : region, [primaryDistrict, mergedFeature, region])
  const parentCity = isDistrict ? regions.find(item => item.code === region.parent) : undefined
  function startMerge() {
    if (!parentCity || !isDistrict) return
    navigate(parentCity)
    setMerge({ city: parentCity, codes: [region.code], phase: 'select' })
  }
  function toggleDistrict(next: Region) {
    if (!availableDistricts.some(item => item.code === next.code)) return
    setMerge(previous => {
      if (!previous || previous.phase !== 'select') return previous
      const codes = previous.codes.includes(next.code) ? previous.codes.filter(code => code !== next.code)
        : previous.codes.length < maxMergedDistricts ? [...previous.codes, next.code].sort() : previous.codes
      return { ...previous, codes }
    })
  }
  function confirmMerge() {
    if (!merge || merge.codes.length < 2 || !merge.codes.every(code => availableDistricts.some(item => item.code === code))) return
    setMerge({ ...merge, phase: 'view' }); writeLocation(merge.city, merge.codes)
  }
  function editMerge() {
    if (!merge) return
    setMerge({ ...merge, phase: 'select' }); writeLocation(merge.city)
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
  const districtError = isDistrict && collection && !districtFeature ? '当前边界数据未包含该区县，请返回上级选择其他地区。' : ''
  const mergeError = merge?.phase === 'view' && collection && !mergedFeature ? '当前边界数据未包含全部所选区县，请重新选区。' : ''

  return <main className="atlas-fullscreen" aria-label="租房版图">
    <AccountPanel onUserChange={setUser} loginRequest={loginRequest} onLoginCancel={cancelPublication} />
    {ready && <RegionSearch regions={regions} current={`${region.code}:${merge?.phase || ''}`} onNavigate={navigate} />}
    <button className="publish-trigger" aria-label="发布房源" title="发布房源" aria-pressed={publishing} onClick={() => { if (merge) navigate(primaryDistrict || merge.city); setWantsPublish(true); if (user) setPublishing(true); if (!user) setLoginRequest(value => value + 1) }}><Plus size={21} /></button>
    {ready && !publishing && (merge || parentCity && districtFeature) && <DistrictMergeControl merge={merge} available={availableDistricts} onStart={startMerge} onToggle={toggleDistrict} onConfirm={confirmMerge} onEdit={editMerge} onExit={exitMerge} />}
    {publishing && !isDistrict && <div className="publication-guide" role="status"><span>请选择房源所在的省、市、区县，再在街道地图上点选位置。</span><button onClick={cancelPublication} aria-label="取消发布"><X size={16} /></button></div>}
    {ready && (merge?.phase === 'view'
      ? mergedFeature && primaryDistrict && <DistrictRentals key={`merge:${merge.codes.join(',')}`} region={rentalRegion} feature={mergedFeature} districts={merge.codes} publishing={false} onPublishEnd={cancelPublication} onLoginRequired={() => setLoginRequest(value => value + 1)} />
      : isDistrict
      ? districtFeature && <DistrictRentals key={region.code} region={region} feature={districtFeature} publishing={publishing} onPublishEnd={cancelPublication} onLoginRequired={() => setLoginRequest(value => value + 1)} />
      : <MapView region={region} collection={collection} onNavigate={merge?.phase === 'select' ? toggleDistrict : navigate} selectedCodes={merge?.phase === 'select' ? merge.codes : undefined} />)}
    {ready && region.code !== national.code && <nav className="map-breadcrumbs" aria-label="当前行政区路径">{trail.map((item, index) => <span key={item.code}>{index > 0 && <i aria-hidden="true">/</i>}{index === trail.length - 1 ? <strong aria-current="location">{item.code === national.code ? '中国' : item.name}</strong> : <button onClick={() => navigate(item)}>{item.code === national.code ? '中国' : item.name}</button>}</span>)}</nav>}
    {(!ready || loading) && !error && !mapError && <div className="map-loading" role="status"><LoaderCircle size={20} className="spin" /><span>正在展开地图</span></div>}
    {(error || mapError || districtError || mergeError) && <div className="map-notice" role="alert"><p>{error || mapError || districtError || mergeError}</p><button onClick={() => { if (mergeError) editMerge(); else { retry(); setReload(value => value + 1) } }}><RotateCcw size={14} />{mergeError ? '重新选区' : '重新加载'}</button></div>}
  </main>
}

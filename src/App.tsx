import { useCallback, useEffect, useRef, useState } from 'react'
import { LoaderCircle, RotateCcw, X } from 'lucide-react'
import MapView from './MapView'
import DistrictMap from './DistrictMap'
import type { Listing, Marker, Region, Results } from './types'
import { api, money, national } from './types'
import { useMapCollection } from './map-data'
import AccountPanel from './AccountPanel'

const currentCode = () => new URLSearchParams(location.search).get('region') || national.code

export default function App() {
  const [regions, setRegions] = useState<Region[]>([national])
  const [region, setRegion] = useState(national)
  const [ready, setReady] = useState(false)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [pins, setPins] = useState<{ code: string; markers: Marker[] }>({ code: '', markers: [] })
  const [listing, setListing] = useState<Listing | null>(null)
  const [listingLoading, setListingLoading] = useState(false)
  const [listingError, setListingError] = useState('')
  const detailRequest = useRef<AbortController | null>(null)
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
    const popstate = () => { closeListing(); setRegion(regions.find(item => item.code === currentCode()) || national) }
    window.addEventListener('popstate', popstate)
    return () => window.removeEventListener('popstate', popstate)
  }, [regions])
  useEffect(() => {
    if (!isDistrict) return
    const abort = new AbortController()
    setListingError('')
    api<Results>(`/api/listings?region=${region.code}`, { signal: abort.signal }).then(data => setPins({ code: region.code, markers: data.markers }))
      .catch(error => { if (!abort.signal.aborted) setListingError(error.message) })
    return () => abort.abort()
  }, [region.code, isDistrict, reload])
  useEffect(() => () => detailRequest.current?.abort(), [])

  function closeListing() {
    detailRequest.current?.abort()
    setListing(null); setListingLoading(false); setListingError('')
  }
  function navigate(next: Region) {
    closeListing()
    setRegions(previous => previous.some(item => item.code === next.code) ? previous : [...previous, next])
    setRegion(next)
    const url = new URL(location.href)
    if (next.code === national.code) url.searchParams.delete('region')
    else url.searchParams.set('region', next.code)
    if (url.href !== location.href) history.pushState({}, '', url)
  }
  async function openListing(id: string) {
    detailRequest.current?.abort()
    const abort = new AbortController()
    detailRequest.current = abort
    setListing(null); setListingLoading(true); setListingError('')
    try { setListing(await api<Listing>(`/api/listings/${encodeURIComponent(id)}`, { signal: abort.signal })) }
    catch (error) { if (!abort.signal.aborted) setListingError((error as Error).message) }
    finally { if (!abort.signal.aborted) setListingLoading(false) }
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
  const markers = pins.code === region.code ? pins.markers : []

  return <main className="atlas-fullscreen" aria-label="租房版图">
    <AccountPanel />
    {ready && (isDistrict
      ? <DistrictMap key={region.code} region={region} feature={districtFeature} markers={markers} onListing={id => { void openListing(id) }} />
      : <MapView region={region} collection={collection} onNavigate={navigate} />)}
    {ready && region.code !== national.code && <nav className="map-breadcrumbs" aria-label="当前行政区路径">{trail.map((item, index) => <span key={item.code}>{index > 0 && <i aria-hidden="true">/</i>}{index === trail.length - 1 ? <strong aria-current="location">{item.code === national.code ? '中国' : item.name}</strong> : <button onClick={() => navigate(item)}>{item.code === national.code ? '中国' : item.name}</button>}</span>)}</nav>}
    {(!ready || loading) && !error && !mapError && <div className="map-loading" role="status"><LoaderCircle size={20} className="spin" /><span>正在展开地图</span></div>}
    {(error || mapError) && <div className="map-notice" role="alert"><p>{error || mapError}</p><button onClick={() => { retry(); setReload(value => value + 1) }}><RotateCcw size={14} />重新加载</button></div>}
    {(listing || listingLoading || listingError) && <ListingPopover listing={listing} loading={listingLoading} error={listingError} onClose={closeListing} onRetry={() => setReload(value => value + 1)} />}
  </main>
}

function ListingPopover({ listing, loading, error, onClose, onRetry }: { listing: Listing | null; loading: boolean; error: string; onClose: () => void; onRetry: () => void }) {
  const panel = useRef<HTMLElement>(null)
  const close = useRef(onClose)
  close.current = onClose
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    panel.current?.focus()
    const keydown = (event: KeyboardEvent) => { if (event.key === 'Escape') close.current() }
    document.addEventListener('keydown', keydown)
    return () => { document.removeEventListener('keydown', keydown); previous?.focus() }
  }, [])
  return <section className="listing-popover" ref={panel} tabIndex={-1} role="dialog" aria-label="房源详情">
    <button className="close-listing" aria-label="关闭房源详情" onClick={onClose}><X size={18} /></button>
    {loading ? <div className="detail-loading"><LoaderCircle size={20} className="spin" /></div> : error ? <div className="detail-error"><p>{error}</p><button onClick={() => { onClose(); onRetry() }}>重新加载房源</button></div> : listing && <>
      <img className="listing-photo" src={listing.image} alt={`${listing.community}室内展示`} />
      <div className="listing-content"><div className="listing-meta"><span>{listing.type}</span><small>{listing.isDemo ? '示例房源' : '用户发布 · 未核验'}</small></div>
        <h1>{listing.community}</h1><p className="listing-subtitle">{listing.title}</p><p className="listing-address">{listing.city} · {listing.district}</p>
        <div className="listing-facts"><span>{listing.rooms} 室</span><i /><span>{listing.area} m²</span><i /><span>{listing.orientation || '朝向待补充'}</span></div>
        <div className="listing-price"><strong>¥ {money(listing.price)}</strong><span>/ 月</span></div>
        <p className="listing-description">{listing.description}</p><p className="listing-contact">{listing.isDemo ? '示例位置与图片仅供体验，无真实出租信息。' : `联系方式：${listing.contact}`}</p>
      </div>
    </>}
  </section>
}

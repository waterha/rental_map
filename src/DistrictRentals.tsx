import { useCallback, useEffect, useRef, useState } from 'react'
import DistrictMap from './DistrictMap'
import PublicationForm from './PublicationForm'
import type { Position } from './amap'
import { X } from 'lucide-react'
import ListingPopover from './ListingPopover'
import type { MapFeature } from './map-data'
import type { Listing, Marker, Region, Results } from './types'
import { api } from './types'
import './rentals.css'

const rentalTypes = ['', '整租', '转租', '合租'] as const
const noMarkers: Marker[] = []
type RentalType = typeof rentalTypes[number]
type PinResult = { key: string; markers: Marker[]; total: number; error: string }
type DetailResult = { id: string; listing: Listing | null; loading: boolean; error: string }

// This subtree owns rental requests. It never changes geography, map retry state
// or the DistrictMap key, including when retrying a failed list or detail request.
export default function DistrictRentals({ region, feature, publishing, onPublishEnd, onLoginRequired }: { region: Region; feature?: MapFeature; publishing: boolean; onPublishEnd: () => void; onLoginRequired: () => void }) {
  const [draft, setDraft] = useState<Position | null>(null)
  const [selecting, setSelecting] = useState(true)
  const [pickError, setPickError] = useState('')
  const [type, setType] = useState<RentalType>('')
  const [attempt, setAttempt] = useState(0)
  const [pins, setPins] = useState<PinResult | null>(null)
  const [detail, setDetail] = useState<DetailResult | null>(null)
  const detailRequest = useRef<AbortController | null>(null)
  const params = new URLSearchParams({ region: region.code })
  if (type) params.set('type', type)
  const requestKey = params.toString()
  const loading = pins?.key !== requestKey
  const result = loading ? null : pins

  useEffect(() => {
    const controller = new AbortController()
    let active = true
    const timer = setTimeout(() => controller.abort(), 15000)
    setPins(null)
    api<Results>(`/api/listings?${requestKey}`, { signal: controller.signal }).then(data => {
      if (active) setPins({ key: requestKey, markers: data.markers, total: data.total, error: '' })
    }).catch(error => {
      if (active) setPins({ key: requestKey, markers: noMarkers, total: 0, error: controller.signal.aborted ? '房源加载超时，请重试' : error.message })
    }).finally(() => clearTimeout(timer))
    return () => { active = false; clearTimeout(timer); controller.abort() }
  }, [requestKey, attempt])

  const closeListing = useCallback(() => {
    detailRequest.current?.abort()
    detailRequest.current = null
    setDetail(null)
  }, [])
  const openListing = useCallback(async (id: string) => {
    detailRequest.current?.abort()
    const controller = new AbortController()
    detailRequest.current = controller
    const timer = setTimeout(() => controller.abort(), 15000)
    setDetail({ id, listing: null, loading: true, error: '' })
    try {
      const listing = await api<Listing>(`/api/listings/${encodeURIComponent(id)}`, { signal: controller.signal })
      if (detailRequest.current === controller && !controller.signal.aborted) setDetail({ id, listing, loading: false, error: '' })
    } catch (error) {
      if (detailRequest.current === controller) setDetail({ id, listing: null, loading: false, error: controller.signal.aborted ? '房源详情加载超时，请重试' : (error as Error).message })
    } finally { clearTimeout(timer) }
  }, [])
  useEffect(() => () => { detailRequest.current?.abort(); detailRequest.current = null }, [])

  useEffect(() => {
    if (publishing) closeListing()
    else { setDraft(null); setSelecting(true); setPickError('') }
  }, [publishing, closeListing])

  function published(listing: Listing) {
    setType(''); setAttempt(value => value + 1); onPublishEnd()
    closeListing()
    setDetail({ id: listing.id, listing, loading: false, error: '' })
  }

  return <>
    <DistrictMap region={region} feature={feature} markers={result?.markers || noMarkers} onListing={openListing} selecting={publishing && selecting} draft={publishing ? draft : null} onPick={point => {
      if (!point) { setPickError('请在当前区县边界内点选位置'); return }
      setDraft(point); setSelecting(false); setPickError('')
    }} />
    {publishing && selecting && <div className="publication-guide" role="status"><span>{pickError || `在${region.name}地图上点击，确认房源位置`}</span><button aria-label="取消发布" onClick={onPublishEnd}><X size={16} /></button></div>}
    {publishing && draft && <PublicationForm region={region} position={draft} hidden={selecting} onReselect={() => setSelecting(true)} onCancel={onPublishEnd} onPublished={published} onLoginRequired={onLoginRequired} />}
    <section className="rental-legend" aria-label="房源颜色图例">
      <p>房源类型</p>
      <ul>
        <li><i className="rental-legend-dot sublet" aria-hidden="true" /><span>转租</span><small>红色</small></li>
        <li><i className="rental-legend-dot" aria-hidden="true" /><span>整租</span><small>紫色</small></li>
        <li><i className="rental-legend-dot shared" aria-hidden="true" /><span>合租</span><small>黄色</small></li>
      </ul>
    </section>
    <div className="rental-controls">
      <div className="rental-types" role="group" aria-label="租房类型" aria-busy={loading}>
        {rentalTypes.map(value => <button key={value} aria-pressed={type === value} onClick={() => { if (value !== type) { closeListing(); setType(value) } }}>{value || '全部'}</button>)}
      </div>
      <div className={`rental-results-status ${result?.error ? 'error' : ''}`} role={result?.error ? 'alert' : 'status'}>
        {loading ? '正在加载房源' : result?.error ? <>{result.error}<button onClick={() => setAttempt(value => value + 1)}>重试房源</button></> : result?.total ? `${result.total} 套${type || ''}房源` : `暂无${type || ''}房源`}
      </div>
    </div>
    {detail && !publishing && <ListingPopover listing={detail.listing} loading={detail.loading} error={detail.error} onClose={closeListing} onRetry={() => { void openListing(detail.id) }} />}
  </>
}

import { useEffect, useRef, useState } from 'react'
import { LoaderCircle, RotateCcw } from 'lucide-react'
import { geoContains } from 'd3-geo'
import type { Marker, Region } from './types'
import type { MapFeature } from './map-data'
import { normalizeWinding } from './map-data'
import { loadAMap } from './amap'
import type { AMapInstance, AMapOverlay, AMapSDK, Ring } from './amap'

export default function DistrictMap({ region, feature, markers, onListing }: { region: Region; feature?: MapFeature; markers: Marker[]; onListing: (id: string) => void }) {
  const container = useRef<HTMLDivElement>(null)
  const mapRef = useRef<AMapInstance | null>(null)
  const sdkRef = useRef<AMapSDK | null>(null)
  const click = useRef(onListing)
  click.current = onListing
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    if (!container.current || !feature) return
    let disposed = false
    let map: AMapInstance | null = null
    let timeout: ReturnType<typeof setTimeout> | undefined
    let removeWheel: (() => void) | undefined
    setStatus('loading'); setError('')
    void loadAMap().then(sdk => {
      if (disposed || !container.current) return
      sdkRef.current = sdk
      map = new sdk.Map(container.current, {
        center: region.center, zoom: 12, zooms: [3, 20], viewMode: '3D', pitch: 0,
        mapStyle: 'amap://styles/darkblue', resizeEnable: true, dragEnable: true,
        zoomEnable: true, scrollWheel: false, doubleClickZoom: true, touchZoom: true,
        rotateEnable: false, pitchEnable: false, keyboardEnable: true,
        showBuildingBlock: true, showLabel: true, features: ['bg', 'road', 'building', 'point'],
        jogEnable: true, animateEnable: true, isHotspot: false,
      })
      mapRef.current = map
      const element = container.current
      const wheel = (event: WheelEvent) => {
        event.preventDefault(); event.stopPropagation()
        if (!map || (event.target as Element).closest('.rental-light')) return
        const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? element.clientHeight : 1)
        map.setZoom(Math.min(20, Math.max(3, map.getZoom() - Math.max(-1.5, Math.min(1.5, delta * .003)))), false, 180)
      }
      element.addEventListener('wheel', wheel, { passive: false, capture: true })
      removeWheel = () => element.removeEventListener('wheel', wheel, { capture: true })
      const geometry = feature.geometry
      const polygons: Ring[][] = geometry.type === 'Polygon' ? [geometry.coordinates as Ring[]] : geometry.type === 'MultiPolygon' ? geometry.coordinates as Ring[][] : []
      const boundaries = polygons.map(path => new sdk.Polygon({ path, strokeColor: '#4edafa', strokeWeight: 2, strokeOpacity: .85, fillColor: '#0d3b54', fillOpacity: .06, bubble: true, zIndex: 80 }))
      if (polygons.length) {
        map.setMask(polygons)
        map.add(boundaries)
        map.setFitView(boundaries, true, [70, 45, 45, 45], 16)
      }
      const syncCamera = () => {
        if (!map || !container.current) return
        const center = map.getCenter()
        container.current.dataset.zoom = String(map.getZoom())
        container.current.dataset.center = `${center.lng.toFixed(6)},${center.lat.toFixed(6)}`
      }
      map.on('zoomend', syncCamera); map.on('moveend', syncCamera); map.on('zoomchange', syncCamera)
      map.on('complete', () => {
        if (!disposed) { clearTimeout(timeout); syncCamera(); setStatus('ready') }
      })
      map.on('error', event => {
        if (!disposed) { clearTimeout(timeout); setError(event?.message || '高德底图加载失败，请检查网络和地图域名授权。'); setStatus('error') }
      })
      timeout = setTimeout(() => {
        if (!disposed) { setError('高德地图尚未完成加载，请确认 Key 已开通 Web JS API，并允许当前访问域名。'); setStatus('error') }
      }, 25000)
    }).catch(error => { if (!disposed) { setError(error.message); setStatus('error') } })
    return () => { disposed = true; clearTimeout(timeout); removeWheel?.(); map?.destroy(); mapRef.current = null; sdkRef.current = null }
  }, [region.code, region.center, feature, retry])

  useEffect(() => {
    const map = mapRef.current, sdk = sdkRef.current
    if (!map || !sdk || status !== 'ready') return
    const overlays: AMapOverlay[] = markers.map(marker => {
      const content = document.createElement('button')
      content.type = 'button'
      content.className = `rental-light ${marker.type === '合租' ? 'shared' : marker.type === '转租' ? 'sublet' : ''}`
      content.setAttribute('aria-label', `查看月租${marker.price}元房源`)
      content.dataset.listing = marker.id
      content.title = `${marker.type} · ¥${marker.price}/月`
      content.innerHTML = '<span class="light-halo"></span><span class="light-core"></span>'
      content.addEventListener('click', event => { event.stopPropagation(); click.current(marker.id) })
      content.addEventListener('pointerdown', event => event.stopPropagation())
      let position: [number, number] = [marker.longitude, marker.latitude]
      // Seed coordinates use administrative centers plus offsets, which may fall outside a narrow district.
      // Keep only demonstration pins inside their actual district mask; never relocate real listings.
      if (marker.isDemo && feature) {
        const normalized = normalizeWinding({ type: 'FeatureCollection', features: [feature] }).features[0]
        const centroid = feature.properties.centroid
        if (!geoContains(normalized, position) && centroid && geoContains(normalized, centroid)) position = centroid
      }
      return new sdk.Marker({ position, content, offset: new sdk.Pixel(0, 0), anchor: 'center', zIndex: 300, title: content.title })
    })
    map.add(overlays)
    return () => { if (mapRef.current === map && overlays.length) map.remove(overlays) }
  }, [markers, status, feature])

  return <div className="district-layer">
    <div className="district-map" ref={container} aria-label={`${region.name}高德街道地图`} data-provider="amap" />
    {status === 'loading' && feature && <div className="map-loading" role="status"><LoaderCircle size={20} className="spin" /><span>正在展开街道地图</span></div>}
    {status === 'error' && <div className="map-notice" role="alert"><p>{error}</p><button onClick={() => setRetry(value => value + 1)}><RotateCcw size={14} />重新加载</button></div>}
  </div>
}

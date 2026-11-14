import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { geoMercator, geoPath } from 'd3-geo'
import type { Region } from './types'
import type { MapCollection, MapFeature } from './map-data'
import { normalizeWinding } from './map-data'

interface View { x: number; y: number; k: number }
const initialView: View = { x: 0, y: 0, k: 1 }

export default function MapView({ region, collection, onNavigate }: { region: Region; collection: MapCollection | null; onNavigate: (region: Region) => void }) {
  const host = useRef<HTMLDivElement>(null)
  const svg = useRef<SVGSVGElement>(null)
  const [size, setSize] = useState({ width: window.innerWidth, height: window.innerHeight })
  const [view, setView] = useState<View>(initialView)
  const latestView = useRef<View>(initialView)
  const cameraFrame = useRef<number | null>(null)
  // Keep every input delta, but commit at most one camera update per display frame.
  const updateView = useCallback((next: View | ((previous: View) => View)) => {
    latestView.current = typeof next === 'function' ? next(latestView.current) : next
    if (cameraFrame.current !== null) return
    cameraFrame.current = requestAnimationFrame(() => {
      cameraFrame.current = null
      setView(latestView.current)
    })
  }, [])
  const [hovered, setHovered] = useState('')
  const [dragging, setDragging] = useState(false)
  const drag = useRef<{ id: number; x: number; y: number; origin: View; feature?: MapFeature; moved: boolean } | null>(null)
  const country = region.code === '100000'
  const raw = useMemo(() => {
    if (!collection) return null
    return !region.hasChildren ? { ...collection, features: collection.features.filter(feature => String(feature.properties.adcode) === region.code) } : collection
  }, [collection, region.code, region.hasChildren])
  const data = useMemo(() => raw ? normalizeWinding(raw) : null, [raw])
  useEffect(() => {
    if (!host.current) return
    const resize = new ResizeObserver(entries => {
      const { width, height } = entries[0].contentRect
      setSize({ width, height })
    })
    resize.observe(host.current)
    return () => resize.disconnect()
  }, [])
  useEffect(() => {
    if (cameraFrame.current !== null) cancelAnimationFrame(cameraFrame.current)
    cameraFrame.current = null
    latestView.current = initialView
    setView(initialView); setHovered(''); setDragging(false); drag.current = null
    return () => { if (cameraFrame.current !== null) cancelAnimationFrame(cameraFrame.current) }
  }, [region.code, size.width, size.height])
  useEffect(() => {
    const element = svg.current
    if (!element) return
    const wheel = (event: WheelEvent) => {
      event.preventDefault()
      const rect = element.getBoundingClientRect(), x = event.clientX - rect.left, y = event.clientY - rect.top
      const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rect.height : 1)
      updateView(previous => {
        const next = Math.max(.6, Math.min(18, previous.k * Math.exp(-Math.max(-250, Math.min(250, delta)) * .0016)))
        const ratio = next / previous.k
        return { k: next, x: x - (x - previous.x) * ratio, y: y - (y - previous.y) * ratio }
      })
    }
    element.addEventListener('wheel', wheel, { passive: false })
    return () => element.removeEventListener('wheel', wheel)
  }, [updateView])
  const { projection, path } = useMemo(() => {
    const projection = geoMercator(), { width, height } = size
    const pad = Math.max(18, Math.min(width, height) * .045)
    if (country) projection.fitExtent([[pad, pad], [width - pad, height - pad]], { type: 'MultiPoint', coordinates: [[73, 53.6], [135.2, 17.8]] })
    else if (data?.features.length) projection.fitExtent([[pad, pad + 35], [width - pad, height - pad]], data)
    return { projection, path: geoPath(projection) }
  }, [data, country, size])
  const inset = useMemo(() => {
    const width = Math.max(62, Math.min(105, size.width * .075)), height = width * 1.43
    const left = size.width - width - 25, top = size.height - height - 28
    const projection = geoMercator().fitExtent([[left + 6, top + 6], [left + width - 6, top + height - 23]], { type: 'MultiPoint', coordinates: [[105, 23], [123, 3]] })
    return { left, top, width, height, path: geoPath(projection) }
  }, [size])
  // Projection is independent of the camera. Reuse paths across hover, pan and zoom.
  const shapes = useMemo(() => data?.features.map(feature => ({ feature, d: path(feature) || '' })) || [], [data, path])
  const insetShapes = useMemo(() => country ? data?.features.map(feature => inset.path(feature) || '') || [] : [], [country, data, inset])
  function enter(feature: MapFeature) {
    if (!region.hasChildren) return
    const p = feature.properties, code = String(p.adcode)
    if (!/^\d{6}$/.test(code)) return
    onNavigate({ code, name: p.name, level: p.level || (country ? 'province' : region.level === 'province' ? 'city' : 'district'), parent: region.code, center: p.center || p.centroid || [0, 0], hasChildren: Boolean(p.childrenNum) })
  }
  const labelOffsets: Record<string, [number, number]> = country ? { '110000': [-2, -11], '120000': [17, 7], '310000': [22, 0], '810000': [23, 13], '820000': [-20, 13], '460000': [0, 7], '320000': [-2, -5], '330000': [5, 8] } : {}
  const fontSize = Math.max(10, Math.min(15, size.width / 95)) / view.k
  return <div className={`administrative-map ${dragging ? 'is-dragging' : ''}`} ref={host}>
    <svg ref={svg} className="atlas-svg" viewBox={`0 0 ${size.width} ${size.height}`} role="group" aria-label={`${region.name}行政区地图`}
      onPointerDown={event => {
        if (event.button !== 0 || !event.isPrimary) return
        const code = (event.target as Element).closest('[data-region]')?.getAttribute('data-region')
        drag.current = { id: event.pointerId, x: event.clientX, y: event.clientY, origin: latestView.current, moved: false, feature: data?.features.find(feature => String(feature.properties.adcode) === code) }
        event.currentTarget.setPointerCapture(event.pointerId)
      }}
      onPointerMove={event => {
        const start = drag.current
        if (!start || start.id !== event.pointerId) return
        const dx = event.clientX - start.x, dy = event.clientY - start.y
        if (!start.moved && Math.hypot(dx, dy) > 5) { start.moved = true; setDragging(true); setHovered('') }
        if (start.moved) updateView({ ...start.origin, x: start.origin.x + dx, y: start.origin.y + dy })
      }}
      onPointerUp={event => {
        const start = drag.current
        if (!start || start.id !== event.pointerId) return
        drag.current = null; setDragging(false)
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
        if (!start.moved && start.feature) enter(start.feature)
      }}
      onPointerCancel={() => { drag.current = null; setDragging(false) }} onLostPointerCapture={() => { drag.current = null; setDragging(false) }}>
      <defs>
        <linearGradient id="province-fill" x1="0" y1="0" x2="1" y2="1"><stop stopColor="#112f44" /><stop offset="1" stopColor="#0c2030" /></linearGradient>
        <linearGradient id="hover-fill" x1="0" y1="0" x2="1" y2="1"><stop stopColor="#12637c" /><stop offset="1" stopColor="#0d3a53" /></linearGradient>
        <pattern id="map-grain" width="6" height="6" patternUnits="userSpaceOnUse"><circle cx="1" cy="1" r=".5" fill="#7cd8ef" opacity=".08" /></pattern>
        <filter id="boundary-glow" x="-10%" y="-10%" width="120%" height="120%"><feGaussianBlur stdDeviation="3" /></filter>
        <clipPath id="sea-inset"><rect x={inset.left + 3} y={inset.top + 3} width={inset.width - 6} height={inset.height - 23} /></clipPath>
      </defs>
      <g className="map-camera" data-zoom={view.k.toFixed(4)} data-pan-x={view.x.toFixed(1)} data-pan-y={view.y.toFixed(1)} transform={`translate(${view.x} ${view.y}) scale(${view.k})`}>
        <g className="boundary-shadow" transform={`translate(0 ${4 / view.k})`} aria-hidden="true">{shapes.map(({ d }, index) => <path key={index} d={d} fill="#03121e" stroke="#15506a" strokeWidth="1" vectorEffect="non-scaling-stroke" />)}</g>
        <g filter="url(#boundary-glow)" opacity=".33" pointerEvents="none">{shapes.map(({ d }, index) => <path key={index} d={d} stroke="#34c4ec" strokeWidth="1.2" fill="none" vectorEffect="non-scaling-stroke" />)}</g>
        {shapes.map(({ feature, d }, index) => {
          const code = String(feature.properties.adcode), interactive = /^\d{6}$/.test(code) && region.hasChildren
          return <path key={code} d={d} data-region={code} className={interactive ? 'region-shape' : 'boundary-line'} fill={hovered === code ? 'url(#hover-fill)' : index % 4 === 0 ? '#143448' : 'url(#province-fill)'}
            stroke={hovered === code ? '#7ce9ff' : '#2a718f'} strokeWidth={hovered === code ? 1.6 : .85} vectorEffect="non-scaling-stroke"
            role={interactive ? 'button' : undefined} tabIndex={interactive ? 0 : undefined} aria-label={interactive ? `进入${feature.properties.name}` : undefined}
            onMouseEnter={() => !drag.current && setHovered(code)} onMouseLeave={() => setHovered('')}
            onKeyDown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); enter(feature) } }} />
        })}
        <g pointerEvents="none" aria-hidden="true">{shapes.map(({ d }, index) => <path key={index} d={d} fill="url(#map-grain)" />)}</g>
        <g className="region-labels" pointerEvents="none">{data?.features.filter(feature => /^\d{6}$/.test(String(feature.properties.adcode))).map(feature => {
          const center = feature.properties.centroid || feature.properties.center
          if (!center) return null
          const point = projection(center); if (!point) return null
          const code = String(feature.properties.adcode), offset = labelOffsets[code] || [0, 0]
          const name = country ? feature.properties.name.replace(/壮族自治区|回族自治区|维吾尔自治区|特别行政区|自治区|省|市$/g, '') : feature.properties.name
          return <text key={code} x={point[0] + offset[0] / view.k} y={point[1] + offset[1] / view.k} fontSize={fontSize} strokeWidth={3 / view.k} className={hovered === code ? 'label-active' : ''}>{name}</text>
        })}</g>
      </g>
      {country && data && <g className="south-sea" pointerEvents="none" aria-label="南海诸岛"><rect x={inset.left} y={inset.top} width={inset.width} height={inset.height} rx="3" /><g clipPath="url(#sea-inset)">{insetShapes.map((d, index) => <path key={index} d={d} fill="#102b3d" stroke="#397791" strokeWidth=".7" />)}</g><text x={inset.left + inset.width / 2} y={inset.top + inset.height - 8} textAnchor="middle">南海诸岛</text></g>}
    </svg>
  </div>
}

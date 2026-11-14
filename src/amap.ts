// Minimal types for the AMap 2.0 methods used by this application.
export type Position = [number, number]
export type Ring = Position[]
export interface AMapOverlay { setMap(map: AMapInstance | null): void }
export interface AMapInstance {
  destroy(): void
  add(overlay: AMapOverlay | AMapOverlay[]): void
  remove(overlay: AMapOverlay | AMapOverlay[]): void
  setMask(mask: Ring[][]): void
  setFitView(overlays: AMapOverlay[], immediately: boolean, padding: number[], maxZoom?: number): void
  on(event: string, handler: (event?: { message?: string }) => void): void
  off(event: string, handler: (event?: { message?: string }) => void): void
  getZoom(): number
  setZoom(zoom: number, immediately?: boolean, duration?: number): void
  getCenter(): { lng: number; lat: number }
}
interface MarkerOptions { position: Position; content: HTMLElement; offset: unknown; anchor: string; zIndex: number; title: string }
export interface AMapSDK {
  Map: new (element: HTMLElement, options: Record<string, unknown>) => AMapInstance
  Polygon: new (options: Record<string, unknown>) => AMapOverlay
  Polyline: new (options: Record<string, unknown>) => AMapOverlay
  Marker: new (options: MarkerOptions) => AMapOverlay
  Pixel: new (x: number, y: number) => unknown
}
declare global {
  interface Window { AMap?: AMapSDK; _AMapSecurityConfig?: { securityJsCode: string }; __rentalAtlasAMapReady?: () => void }
}
let pending: Promise<AMapSDK> | null = null

export function loadAMap(): Promise<AMapSDK> {
  if (window.AMap) return Promise.resolve(window.AMap)
  if (pending) return pending
  const key = import.meta.env.VITE_AMAP_KEY?.trim()
  const securityJsCode = import.meta.env.VITE_AMAP_SECURITY_CODE?.trim()
  if (!key || !securityJsCode) return Promise.reject(new Error('请在 .env.local 中填写高德地图 Key 与安全密钥，再重新构建前端。'))
  window._AMapSecurityConfig = { securityJsCode }
  const request = new Promise<AMapSDK>((resolve, reject) => {
    const script = document.createElement('script')
    script.id = 'rental-atlas-amap'
    script.async = true
    const cleanup = () => { clearTimeout(timer); delete window.__rentalAtlasAMapReady }
    const fail = () => { cleanup(); script.remove(); reject(new Error('高德地图加载失败，请检查网络及 Key 的域名授权后重试。')) }
    const timer = setTimeout(fail, 25000)
    window.__rentalAtlasAMapReady = () => {
      cleanup()
      if (window.AMap) resolve(window.AMap)
      else { script.remove(); reject(new Error('高德地图未初始化，请检查本地地图配置。')) }
    }
    script.onerror = fail
    script.src = `https://webapi.amap.com/maps?v=2.0&key=${encodeURIComponent(key)}&callback=__rentalAtlasAMapReady`
    document.head.appendChild(script)
  })
  pending = request.catch(error => { pending = null; throw error })
  return pending
}

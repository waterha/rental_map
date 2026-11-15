export interface Region {
  code: string
  name: string
  level: 'country' | 'province' | 'city' | 'district'
  parent: string
  center: [number, number]
  hasChildren: boolean
}

export interface Listing {
  id: string
  title: string
  description: string
  type: string
  price: number
  area: number
  rooms: number
  floor: string
  orientation: string
  community: string
  provinceCode: string
  cityCode: string
  districtCode: string
  city: string
  district: string
  address: string
  longitude: number
  latitude: number
  images?: string[]
  rentalSource?: string
  propertyType?: string
  halls?: number
  bathrooms?: number
  image: string
  tags: string[]
  contact: string
  createdAt: string
  isDemo: boolean
}

export interface RegionCount { code: string; name: string; count: number; average: number }
export interface Marker { id: string; longitude: number; latitude: number; price: number; type: string; isDemo: boolean }
export interface Results {
  items: Listing[]
  total: number
  page: number
  pageSize: number
  average: number
  median: number
  types: Record<string, number>
  regions: RegionCount[]
  cities: RegionCount[]
  markers: Marker[]
  demo: boolean
}

export interface Filters { type: string; budget: string; rooms: string; tag: string; query: string; sort: string }
export const national: Region = { code: '100000', name: '全国', level: 'country', parent: '', center: [104, 35], hasChildren: true }
export const emptyResults: Results = { items: [], total: 0, page: 1, pageSize: 9, average: 0, median: 0, types: {}, regions: [], cities: [], markers: [], demo: true }
export const initialFilters: Filters = { type: '', budget: '', rooms: '', tag: '', query: '', sort: 'newest' }
export const shortName = (name: string) => name.replace(/壮族自治区|回族自治区|维吾尔自治区|特别行政区|自治区|省|市$/g, '')
export const money = (value: number) => value.toLocaleString('zh-CN')

export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, options)
  if (!response.ok) {
    let message = '网络连接失败，请稍后重试'
    try { const result = await response.json(); message = result.error || message } catch { /* Non-JSON upstream errors keep a useful fallback. */ }
    throw new Error(message)
  }
  return response.json() as Promise<T>
}

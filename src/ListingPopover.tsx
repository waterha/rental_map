import { useEffect, useRef } from 'react'
import { LoaderCircle, X } from 'lucide-react'
import type { Listing } from './types'
import { money } from './types'
import PhotoCarousel from './PhotoCarousel'

export default function ListingPopover({ listing, loading, error, onClose, onRetry }: { listing: Listing | null; loading: boolean; error: string; onClose: () => void; onRetry: () => void }) {
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
    {loading ? <div className="detail-loading"><LoaderCircle size={20} className="spin" /></div> : error ? <div className="detail-error"><p>{error}</p><button onClick={onRetry}>重新加载房源</button></div> : listing && <>
      <PhotoCarousel key={listing.id} images={listing.images?.length ? listing.images : [listing.image]} name={listing.community} />
      <div className="listing-content"><div className="listing-meta"><span>{listing.type}{listing.rentalSource ? ` · ${listing.rentalSource}` : ''}</span><small>{listing.isDemo ? '示例房源' : '用户发布 · 未核验'}</small></div>
        <h1>{listing.community}</h1><p className="listing-subtitle">{listing.title}</p><p className="listing-address">{listing.city} · {listing.district}</p>
        <div className="listing-facts"><span>{listing.rooms} 室{listing.bathrooms ? ` ${listing.halls || 0} 厅 ${listing.bathrooms} 卫` : ''}</span><i /><span>{listing.area} m²</span><i /><span>{listing.propertyType || listing.orientation || '朝向待补充'}</span></div>
        <div className="listing-price"><strong>¥ {money(listing.price)}</strong><span>/ 月</span></div>
        <p className="listing-description">{listing.description}</p><p className="listing-contact">{listing.isDemo ? '示例位置与图片仅供体验，无真实出租信息。' : `联系方式：${listing.contact}`}</p>
      </div>
    </>}
  </section>
}

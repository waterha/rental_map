import { useEffect, useRef, useState } from 'react'
import { LoaderCircle, Pencil, Trash2, X } from 'lucide-react'
import type { Listing } from './types'
import { money } from './types'
import PhotoCarousel from './PhotoCarousel'

export default function ListingPopover({ listing, canManage, loading, error, onClose, onRetry, onEdit, onDeleted, onLoginRequired }: { listing: Listing | null; canManage: boolean; loading: boolean; error: string; onClose: () => void; onRetry: () => void; onEdit: () => void; onDeleted: () => void; onLoginRequired: () => void }) {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [actionError, setActionError] = useState('')
  const [needsLogin, setNeedsLogin] = useState(false)
  const pending = useRef(false)
  const panel = useRef<HTMLElement>(null)
  const close = useRef(onClose)
  close.current = onClose
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    panel.current?.focus({ preventScroll: true })
    const keydown = (event: KeyboardEvent) => { if (event.key === 'Escape' && !pending.current && !document.querySelector('[aria-modal="true"]')) close.current() }
    document.addEventListener('keydown', keydown)
    return () => {
      document.removeEventListener('keydown', keydown)
      // The marker may have moved off screen while the detail was open. Native
      // focus scrolling shifts AMap's canvas container without moving its camera.
      if (previous?.isConnected) previous.focus({ preventScroll: true })
    }
  }, [])
  async function remove() {
    if (!listing || !canManage || pending.current) return
    pending.current = true; setDeleting(true); setActionError(''); setNeedsLogin(false)
    try {
      const response = await fetch(`/api/listings/${encodeURIComponent(listing.id)}`, { method: 'DELETE' })
      if (!response.ok) {
        setNeedsLogin(response.status === 401)
        const result = await response.json()
        throw new Error(result.error || '删除失败，请重试')
      }
      onDeleted()
    } catch (error) { setActionError((error as Error).message || '网络连接失败，请重试') }
    finally { pending.current = false; setDeleting(false) }
  }
  return <section className="listing-popover" ref={panel} tabIndex={-1} role="dialog" aria-label="房源详情">
    <button className="close-listing" aria-label="关闭房源详情" onClick={onClose} disabled={deleting}><X size={18} /></button>
    {loading ? <div className="detail-loading"><LoaderCircle size={20} className="spin" /></div> : error ? <div className="detail-error"><p>{error}</p><button onClick={onRetry}>重新加载房源</button></div> : listing && <>
      <PhotoCarousel key={listing.id} images={listing.images?.length ? listing.images : [listing.image]} name={listing.title} />
      {canManage && <div className="listing-management" aria-label="管理房源">
        <button aria-label="编辑房源" title="编辑房源" onClick={onEdit} disabled={deleting}><Pencil size={17} /></button>
        <button aria-label="删除房源" title="删除房源" onClick={() => { setConfirmDelete(true); setActionError('') }} disabled={deleting}><Trash2 size={17} /></button>
      </div>}
      <div className="listing-content"><div className="listing-meta"><span>{listing.type}{listing.rentalSource ? ` · ${listing.rentalSource}` : ''}</span><small>{listing.isDemo ? '示例房源' : '用户发布 · 未核验'}</small></div>
        {confirmDelete && canManage && <div className="listing-delete-confirm" role="group" aria-label="确认删除房源"><p>确定删除此房源？删除后无法恢复。</p><button disabled={deleting} onClick={() => { void remove() }}>{deleting ? '正在删除…' : '确认删除'}</button><button disabled={deleting} onClick={() => { setConfirmDelete(false); setActionError('') }}>取消</button></div>}
        {actionError && <p className="publication-error" role="alert">{actionError}{needsLogin && <button onClick={onLoginRequired}>重新登录</button>}</p>}
        <h1 className="listing-subtitle">{listing.title}</h1><p className="listing-address">{listing.city} · {listing.district}</p>
        <div className="listing-facts"><span>{listing.rooms} 室{listing.bathrooms ? ` ${listing.halls || 0} 厅 ${listing.bathrooms} 卫` : ''}</span><i /><span>{listing.area} m²</span><i /><span>{listing.propertyType || listing.orientation || '朝向待补充'}</span></div>
        <div className="listing-price"><strong>¥ {money(listing.price)}</strong><span>/ 月</span></div>
        <p className="listing-description">{listing.description}</p><p className="listing-contact">{listing.isDemo ? '示例位置与图片仅供体验，无真实出租信息。' : `联系方式：${listing.contact}`}</p>
      </div>
    </>}
  </section>
}

import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { LoaderCircle, MapPin, Plus, X } from 'lucide-react'
import type { Position } from './amap'
import type { Listing, Region } from './types'

export default function PublicationForm({ region, position, hidden, onReselect, onCancel, onPublished, onLoginRequired, existing }: {
  region: Region; position: Position; hidden: boolean; onReselect: () => void; onCancel: () => void
  onPublished: (listing: Listing) => void; onLoginRequired: () => void
  existing?: Listing
}) {
  const [retained, setRetained] = useState<string[]>(existing?.images?.length ? existing.images : existing?.image ? [existing.image] : [])
  const [photos, setPhotos] = useState<File[]>([])
  const [previews, setPreviews] = useState<string[]>([])
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [needsLogin, setNeedsLogin] = useState(false)
  const pending = useRef(false)
  const panel = useRef<HTMLDivElement>(null)
  const cancel = useRef(onCancel)
  cancel.current = onCancel
  useEffect(() => {
    const urls = photos.map(file => URL.createObjectURL(file))
    setPreviews(urls)
    return () => urls.forEach(url => URL.revokeObjectURL(url))
  }, [photos])
  useEffect(() => {
    if (hidden) return
    const previous = document.activeElement as HTMLElement | null
    panel.current?.focus({ preventScroll: true })
    const keydown = (event: KeyboardEvent) => {
      if (document.querySelector('.account-backdrop')) return
      if (event.key === 'Escape' && !pending.current) cancel.current()
      if (event.key !== 'Tab') return
      const items = [...(panel.current?.querySelectorAll<HTMLElement>('input:not(:disabled),select:not(:disabled),textarea:not(:disabled),button:not(:disabled),[tabindex="0"]') || [])].filter(el => el.getClientRects().length)
      const first = items[0], last = items.at(-1)
      if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.current)) { event.preventDefault(); last?.focus() }
      else if (!event.shiftKey && (document.activeElement === last || document.activeElement === panel.current)) { event.preventDefault(); first?.focus() }
    }
    document.addEventListener('keydown', keydown)
    return () => { document.removeEventListener('keydown', keydown); if (previous?.isConnected) previous.focus({ preventScroll: true }) }
  }, [hidden])

  function addPhotos(files: FileList | null) {
    if (!files) return
    const additions = [...files]
    if (retained.length + photos.length + additions.length > 5) { setError('最多上传 5 张照片，请先移除多余照片'); return }
    if (additions.some(file => !['image/jpeg', 'image/png'].includes(file.type) || file.size > 5 * 1024 * 1024)) { setError('请选择每张不超过 5 MB 的 JPEG / PNG 照片'); return }
    setPhotos(current => [...current, ...additions]); setError('')
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending.current) return
    if (retained.length + photos.length < 1 || retained.length + photos.length > 5) { setError('请上传 1–5 张房源照片'); return }
    const fields = new FormData(event.currentTarget)
    const listing = {
      title: String(fields.get('title')).trim(),
      description: String(fields.get('description')).trim(), contact: String(fields.get('contact')).trim(),
      type: fields.get('type'), rentalSource: fields.get('rentalSource'), propertyType: fields.get('propertyType'),
      price: Number(fields.get('price')), area: Number(fields.get('area')), rooms: Number(fields.get('rooms')),
      halls: Number(fields.get('halls')), bathrooms: Number(fields.get('bathrooms')), districtCode: region.code,
      longitude: position[0], latitude: position[1], tags: existing?.tags || [],
      ...(existing ? { images: retained, floor: existing.floor, orientation: existing.orientation } : {}),
    }
    const body = new FormData()
    body.append('listing', JSON.stringify(listing))
    photos.forEach(file => body.append('photos', file))
    pending.current = true; setSaving(true); setError(''); setNeedsLogin(false)
    try {
      const response = await fetch(existing ? `/api/listings/${encodeURIComponent(existing.id)}` : '/api/listings', { method: existing ? 'PUT' : 'POST', body })
      const result = await response.json()
      if (!response.ok) { setNeedsLogin(response.status === 401); throw new Error(result.error || '保存失败，请重试') }
      onPublished(result as Listing)
    } catch (error) { setError((error as Error).message || '网络连接失败，请重试') }
    finally { pending.current = false; setSaving(false) }
  }

  return <div className="publication-backdrop" hidden={hidden}>
    <div className="publication-panel" ref={panel} role="dialog" aria-modal="true" aria-label={existing ? '编辑房源资料' : '发布房源资料'} tabIndex={-1}>
      <header><div><p className="publication-eyebrow">YOUR NEXT COORDINATE</p><h1>{existing ? '编辑房源' : '让好房，亮在地图上'}</h1></div><button type="button" aria-label={existing ? '取消编辑' : '取消发布'} onClick={onCancel} disabled={saving}><X size={20} /></button></header>
      <div className="publication-location"><MapPin size={16} /><span>{region.name} · {existing ? '保留原房源位置' : '已选定位置'}</span>{!existing && <button type="button" onClick={onReselect} disabled={saving}>重新选点</button>}</div>
      <form onSubmit={event => { void submit(event) }}>
        <fieldset disabled={saving}>
          <legend className="sr-only">房源资料</legend>
          <div className="photo-heading"><span>房源照片 <small>1–5 张 · 可左右滑动预览</small></span><span>{retained.length + photos.length}/5</span></div>
          <div className="publication-photos" aria-label="已选房源照片">
            {retained.map((url, i) => <div className="publication-photo" key={url}><img src={url} alt={`原房源照片 ${i + 1}`} /><button type="button" aria-label={`移除原照片 ${i + 1}`} onClick={() => setRetained(current => current.filter(image => image !== url))}><X size={14} /></button></div>)}
            {previews.map((url, i) => <div className="publication-photo" key={url}><img src={url} alt={`待发布照片 ${i + 1}`} /><button type="button" aria-label={`移除照片 ${i + 1}`} onClick={() => setPhotos(current => current.filter((_, index) => index !== i))}><X size={14} /></button></div>)}
            {retained.length + photos.length < 5 && <label className="add-photo"><Plus size={23} /><span>添加照片</span><input aria-label="上传房源照片" type="file" accept="image/jpeg,image/png" multiple onChange={event => { addPhotos(event.target.files); event.target.value = '' }} /></label>}
          </div>
          <p className="publication-hint">JPEG / PNG，每张 ≤ 5 MB、≤ 1200 万像素。上传时移除照片定位信息。</p>
          <label>房源<input name="title" defaultValue={existing?.title} placeholder="例如：西湖区近地铁阳光一居" required minLength={5} maxLength={60} /></label>
          <div className="publication-grid"><label>出租类型<select aria-label="出租类型" name="type" defaultValue={existing?.type || '整租'}><option>整租</option><option>转租</option><option>合租</option></select></label><label>出租身份<select aria-label="出租身份" name="rentalSource" defaultValue={existing?.rentalSource || '房东直租'}><option>房东直租</option><option>个人转租</option><option>合租招室友</option></select></label></div>
          <div className="publication-grid"><label>月租（元）<input name="price" defaultValue={existing?.price} type="number" min={100} max={1000000} step={1} placeholder="3000" required /></label><label>面积（m²）<input name="area" defaultValue={existing?.area} type="number" min={5} max={2000} step="0.1" placeholder="45" required /></label></div>
          <div className="publication-layout"><label>卧室<input name="rooms" aria-label="卧室数" type="number" min={1} max={10} defaultValue={existing?.rooms ?? 1} required /></label><label>客厅<input name="halls" aria-label="客厅数" type="number" min={0} max={10} defaultValue={existing?.halls ?? 0} required /></label><label>卫生间<input name="bathrooms" aria-label="卫生间数" type="number" min={1} max={10} defaultValue={existing?.bathrooms ?? 1} required /></label><label>房屋类型<select aria-label="房屋类型" name="propertyType" defaultValue={existing?.propertyType || '住宅'}><option>住宅</option><option>公寓</option><option>其他</option></select></label></div>
          <label>房源描述<textarea name="description" defaultValue={existing?.description} placeholder="介绍采光、交通、入住时间等（至少 10 个字）" minLength={10} maxLength={2000} required rows={3} /></label>
          <label>联系方式<input name="contact" defaultValue={existing?.contact} placeholder="电话、微信号或联系邮箱" minLength={3} maxLength={100} required /></label>
          <p className="publication-hint">发布后，位置、照片及联系方式将公开展示。请确认你有权发布这些内容。</p>
        </fieldset>
        {error && <p className="publication-error" role="alert">{error}{needsLogin && <button type="button" onClick={onLoginRequired}>重新登录</button>}</p>}
        <button className="publication-submit" type="submit" disabled={saving}>{saving ? <><LoaderCircle size={17} className="spin" />正在保存…</> : existing ? '保存房源修改' : '发布房源，点亮地图'}</button>
      </form>
    </div>
  </div>
}

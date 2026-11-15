import { useRef, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'

export default function PhotoCarousel({ images, name }: { images: string[]; name: string }) {
  const track = useRef<HTMLDivElement>(null)
  const [index, setIndex] = useState(0)
  function go(next: number) { track.current?.scrollTo({ left: Math.max(0, Math.min(images.length - 1, next)) * track.current.clientWidth, behavior: 'smooth' }) }
  return <div className="photo-carousel" role="region" aria-label="房源照片轮播">
    <div className="photo-track" ref={track} tabIndex={0} onKeyDown={event => {
      if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') { event.preventDefault(); go(index + (event.key === 'ArrowRight' ? 1 : -1)) }
    }} onScroll={() => { if (track.current) setIndex(Math.max(0, Math.min(images.length - 1, Math.round(track.current.scrollLeft / track.current.clientWidth)))) }}>
      {images.map((src, i) => <img className="listing-photo" key={`${src}-${i}`} src={src} alt={`${name}室内展示 ${i + 1}`} draggable={false} />)}
    </div>
    {images.length > 1 && <>
      <button className="photo-previous" aria-label="上一张照片" disabled={index === 0} onClick={() => go(index - 1)}><ChevronLeft size={18} /></button>
      <button className="photo-next" aria-label="下一张照片" disabled={index === images.length - 1} onClick={() => go(index + 1)}><ChevronRight size={18} /></button>
      <span className="photo-counter" aria-live="polite">{index + 1} / {images.length}</span>
    </>}
  </div>
}

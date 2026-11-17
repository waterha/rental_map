import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { Search, X, MapPin } from 'lucide-react'
import type { Region } from './types'
import { buildRegionSearch } from './region-search'
import './region-search.css'

export default function RegionSearch({ regions, current, onNavigate }: { regions: Region[]; current: string; onNavigate: (region: Region) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const host = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const listId = useId()
  const search = useMemo(() => buildRegionSearch(regions), [regions])
  const results = useMemo(() => search(query), [search, query])
  const selected = Math.min(active, Math.max(0, results.length - 1))
  function close(restoreFocus = false) {
    setOpen(false)
    if (restoreFocus) trigger.current?.focus({ preventScroll: true })
  }
  function choose(region: Region) { close(true); onNavigate(region) }
  useEffect(() => { setOpen(false) }, [current])
  useEffect(() => {
    if (!open) return
    input.current?.focus({ preventScroll: true })
    const outside = (event: PointerEvent) => { if (!host.current?.contains(event.target as Node)) setOpen(false) }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { setOpen(false); trigger.current?.focus({ preventScroll: true }) }
    }
    document.addEventListener('pointerdown', outside)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape) }
  }, [open])
  useEffect(() => {
    if (open) document.getElementById(`${listId}-${selected}`)?.scrollIntoView({ block: 'nearest' })
  }, [listId, selected, open])
  return <div className="region-search" ref={host}>
    <button className="region-search-trigger" ref={trigger} aria-label="搜索地区" title="搜索地区" aria-expanded={open} aria-controls={`${listId}-panel`}
      onClick={() => setOpen(value => !value)}><Search size={19} /></button>
    {open && <section className="region-search-panel" id={`${listId}-panel`} aria-label="地区搜索">
      <form role="search" onSubmit={event => { event.preventDefault(); if (results[selected]) choose(results[selected].region) }}>
        <Search size={17} aria-hidden="true" />
        <input ref={input} type="search" role="combobox" aria-label="搜索省、市、区县" aria-autocomplete="list" aria-expanded={results.length > 0}
          aria-controls={listId} aria-activedescendant={results.length ? `${listId}-${selected}` : undefined}
          placeholder="搜索省、市、区县" maxLength={80} value={query} autoComplete="off" spellCheck={false}
          onChange={event => { setQuery(event.target.value); setActive(0) }}
          onKeyDown={event => {
            if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) {
              if (event.key === 'Enter') event.preventDefault()
              return
            }
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              if (results.length) setActive((selected + (event.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length)
            }
          }} />
        <button type="button" aria-label="关闭地区搜索" onClick={() => close(true)}><X size={16} /></button>
      </form>
      <p className="region-search-hint">搜索省、市、区县，例如“杭州西湖”</p>
      <ul id={listId} role="listbox" aria-label="匹配地区">
        {results.map((item, index) => <li key={item.region.code} role="option" id={`${listId}-${index}`} aria-selected={index === selected}>
          <button type="button" tabIndex={-1} onPointerDown={event => event.preventDefault()} onClick={() => choose(item.region)}>
            <MapPin size={15} aria-hidden="true" /><span><strong>{item.region.name}</strong><small>{item.path}</small></span>
            <i>{item.region.level === 'province' ? '省级' : item.region.level === 'city' ? '市级' : '区县'}</i>
          </button>
        </li>)}
      </ul>
      {!!query.trim() && results.length === 0 && <p className="region-search-empty" role="status">未找到匹配地区，可尝试完整的省、市、区县名称。</p>}
      {results.length > 0 && <span className="sr-only" role="status">找到 {results.length} 个地区，使用上下方向键选择，回车进入。</span>}
    </section>}
  </div>
}

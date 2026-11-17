import { Combine, X, Check } from 'lucide-react'
import type { Region } from './types'
import type { DistrictMerge } from './district-merge'
import { maxMergedDistricts } from './district-merge'
import './district-merge.css'

export default function DistrictMergeControl({ merge, available, onStart, onToggle, onConfirm, onEdit, onExit }: {
  merge: DistrictMerge | null; available: Region[]; onStart: () => void; onToggle: (region: Region) => void;
  onConfirm: () => void; onEdit: () => void; onExit: () => void
}) {
  return <>
    <button className={`merge-trigger ${merge ? 'active' : ''}`} aria-label={merge ? '重新选择合区' : '合区找房'} title="合区找房"
      aria-pressed={Boolean(merge)} onClick={merge ? onEdit : onStart}><Combine size={19} /></button>
    {merge && <section className={`merge-panel ${merge.phase === 'view' ? 'viewing' : ''}`} aria-label="合区找房">
      <header><strong>{merge.phase === 'select' ? '选择一起查看的区县' : '正在合区找房'}</strong><button onClick={onExit} aria-label="退出合区返回市级" title="退出合区 · ESC"><X size={17} /></button></header>
      {merge.phase === 'select' ? <>
        <p>点击地图选择或取消区县，再点击“一起看房”。</p>
        <div className="merge-districts" role="group" aria-label="可合并区县">
          {available.map(region => <button key={region.code} aria-pressed={merge.codes.includes(region.code)}
            disabled={merge.codes.length >= maxMergedDistricts && !merge.codes.includes(region.code)} onClick={() => onToggle(region)}>
            {merge.codes.includes(region.code) && <Check size={12} />}{region.name}
          </button>)}
        </div>
        <footer><span>已选 {merge.codes.length} 个区县 · 最多 {maxMergedDistricts} 个</span><button className="merge-confirm" disabled={merge.codes.length < 2} onClick={onConfirm}>一起看房</button></footer>
      </> : <><p className="merge-selected-names">{merge.codes.map(code => available.find(region => region.code === code)?.name).filter(Boolean).join(' + ')}</p><button className="merge-edit" onClick={onEdit}>重新选区</button></>}
      <small>按 ESC 返回{merge.city.name}地图</small>
    </section>}
  </>
}

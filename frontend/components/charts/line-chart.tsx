'use client'

import { useRef, useState } from 'react'
import { GRIDLINE } from '@/lib/chart-colors'
import { cx } from '@/lib/cx'

export interface LineSeries {
  key: string
  label: string
  color: string
  values: number[]
}

// Fixed internal coordinate system — the SVG scales to its rendered width via
// viewBox, so every x/y math below is in these units regardless of screen size.
const VB_W = 600
const VB_H = 200
const PAD_L = 40
const PAD_B = 4

function niceCeil(max: number): number {
  if (max <= 0) return 1
  const magnitude = 10 ** Math.floor(Math.log10(max))
  const norm = max / magnitude
  const niceNorm = norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10
  return niceNorm * magnitude
}

// One shared line/area chart for every "change over time" chart on the leads
// dashboard (leads/consultations, revenue, site traffic) — was three
// near-identical bar-chart functions; a trend over time is a line's job, not
// a bar's (see the dataviz skill's choosing-a-form guide), and one component
// means one hover/crosshair/gridline implementation to get right instead of
// three copies drifting apart.
export function LineChart({
  series,
  xLabels,
  formatValue = (v) => String(v),
  height = 180,
  area = false,
  labelStep,
}: {
  series: LineSeries[]
  xLabels: string[]
  formatValue?: (v: number) => string
  height?: number
  area?: boolean
  labelStep?: number
}) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)

  const n = xLabels.length
  if (n === 0 || series.every((s) => s.values.length === 0)) {
    return <div className='text-sm text-gray-400'>Нет данных за период.</div>
  }

  const max = Math.max(1, ...series.flatMap((s) => s.values))
  const niceMax = niceCeil(max)
  const plotW = VB_W - PAD_L
  const plotH = VB_H - PAD_B
  const xAt = (i: number) => PAD_L + (n <= 1 ? plotW / 2 : (i / (n - 1)) * plotW)
  const yAt = (v: number) => plotH - (v / niceMax) * plotH
  const step = labelStep ?? Math.max(1, Math.ceil(n / 14))

  function pointsFor(values: number[]): string {
    return values.map((v, i) => `${xAt(i)},${yAt(v)}`).join(' ')
  }

  function updateFromClientX(clientX: number) {
    const svg = svgRef.current
    if (!svg) return
    const rect = svg.getBoundingClientRect()
    const xUser = ((clientX - rect.left) / rect.width) * VB_W
    const idx = Math.round(((xUser - PAD_L) / plotW) * (n - 1))
    setHoverIdx(Math.min(n - 1, Math.max(0, idx)))
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowRight') setHoverIdx((i) => Math.min(n - 1, (i ?? -1) + 1))
    else if (e.key === 'ArrowLeft') setHoverIdx((i) => Math.max(0, (i ?? 1) - 1))
    else if (e.key === 'Escape') setHoverIdx(null)
    else return
    e.preventDefault()
  }

  const gridFracs = [0, 0.5, 1]

  return (
    <div>
      {series.length >= 2 && (
        <div className='mb-2 flex flex-wrap items-center gap-4 text-xs text-gray-500'>
          {series.map((s) => (
            <span key={s.key} className='flex items-center gap-1.5'>
              <span className='h-0.5 w-3 rounded-full' style={{ backgroundColor: s.color }} />
              {s.label}
            </span>
          ))}
        </div>
      )}

      <div className='relative'>
        <svg
          ref={svgRef}
          viewBox={`0 0 ${VB_W} ${VB_H}`}
          preserveAspectRatio='none'
          className='block w-full touch-none outline-none'
          style={{ height }}
          tabIndex={0}
          role='img'
          aria-label={`${series.map((s) => s.label).join(', ')} по периоду — используйте стрелки для навигации по точкам`}
          onPointerMove={(e) => updateFromClientX(e.clientX)}
          onPointerDown={(e) => updateFromClientX(e.clientX)}
          onPointerLeave={() => setHoverIdx(null)}
          onKeyDown={onKeyDown}
        >
          {gridFracs.map((g) => {
            const y = plotH - g * plotH
            return (
              <g key={g}>
                <line x1={PAD_L} x2={VB_W} y1={y} y2={y} stroke={GRIDLINE} strokeWidth={1} />
                <text x={PAD_L - 6} y={y === plotH ? y - 2 : y} textAnchor='end' dominantBaseline='middle' className='fill-gray-400' fontSize={9}>
                  {formatValue(Math.round(niceMax * g))}
                </text>
              </g>
            )
          })}

          {area && series.length === 1 && (
            <polygon
              points={`${xAt(0)},${plotH} ${pointsFor(series[0].values)} ${xAt(n - 1)},${plotH}`}
              fill={series[0].color}
              opacity={0.1}
            />
          )}

          {series.map((s) => (
            <polyline key={s.key} points={pointsFor(s.values)} fill='none' stroke={s.color} strokeWidth={2} strokeLinecap='round' strokeLinejoin='round' />
          ))}

          {series.map((s) =>
            s.values.map((v, i) => (
              <circle key={`${s.key}-${i}`} cx={xAt(i)} cy={yAt(v)} r={i === hoverIdx ? 4 : 2.5} fill={s.color} stroke='#fcfcfb' strokeWidth={2} />
            ))
          )}

          {hoverIdx !== null && <line x1={xAt(hoverIdx)} x2={xAt(hoverIdx)} y1={0} y2={plotH} stroke='#c3c2b7' strokeWidth={1} />}
        </svg>

        {hoverIdx !== null && (
          <div
            className={cx(
              'pointer-events-none absolute top-0 z-10 rounded-md bg-gray-900 px-2.5 py-1.5 text-xs whitespace-nowrap text-white shadow-lg',
              hoverIdx / (n - 1 || 1) > 0.75 ? '-translate-x-full' : hoverIdx === 0 ? '' : '-translate-x-1/2'
            )}
            style={{ left: `${(xAt(hoverIdx) / VB_W) * 100}%` }}
          >
            <div className='mb-0.5 text-gray-300'>{xLabels[hoverIdx]}</div>
            <div className='space-y-0.5'>
              {series.map((s) => (
                <div key={s.key} className='flex items-center gap-1.5'>
                  <span className='h-0.5 w-2.5 shrink-0 rounded-full' style={{ backgroundColor: s.color }} />
                  <span className='font-semibold tabular-nums'>{formatValue(s.values[hoverIdx])}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className='mt-1 flex' style={{ paddingLeft: `${(PAD_L / VB_W) * 100}%` }}>
        {xLabels.map((l, i) => (
          <div key={i} className='min-w-0 flex-1 truncate text-center text-[10px] text-gray-400'>
            {i % step === 0 ? l : ''}
          </div>
        ))}
      </div>
    </div>
  )
}

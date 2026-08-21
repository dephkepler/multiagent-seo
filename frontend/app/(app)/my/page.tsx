'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Section } from '@/components/ui/section'
import { StatTile } from '@/components/ui/stat-tile'
import { money } from '@/lib/format'
import { CasesTable } from './cases-table'
import { MoneyPanel } from './money-panel'
import { CASE_STATUS_LABEL, type MyCase, type MyCaseList, type MyClientList, type MySettlement, type MyStats } from './types'

export default function MyPage() {
  const qc = useQueryClient()
  const [busyCaseID, setBusyCaseID] = useState<string | null>(null)

  const cases = useQuery({ queryKey: ['my-cases'], queryFn: () => api<MyCaseList>('/my/cases') })
  const clients = useQuery({ queryKey: ['my-clients'], queryFn: () => api<MyClientList>('/my/clients') })
  const settlement = useQuery({ queryKey: ['my-settlement'], queryFn: () => api<MySettlement>('/my/settlement') })
  const stats = useQuery({ queryKey: ['my-stats'], queryFn: () => api<MyStats>('/my/stats') })

  const setStatus = useMutation({
    mutationFn: (vars: { id: string; status: string }) =>
      api(`/my/cases/${vars.id}/status`, { method: 'PATCH', body: JSON.stringify({ status: vars.status }) }),
    onSuccess: () => {
      toast.success('Статус дела обновлён')
      // The status change moves money between "in progress" and "done" on every
      // one of these panels, so they all go stale together.
      qc.invalidateQueries({ queryKey: ['my-cases'] })
      qc.invalidateQueries({ queryKey: ['my-stats'] })
      qc.invalidateQueries({ queryKey: ['my-settlement'] })
    },
    onError: (e: Error) => toast.error(e.message),
    onSettled: () => setBusyCaseID(null),
  })

  function changeStatus(c: MyCase, status: string) {
    setBusyCaseID(c.id)
    setStatus.mutate({ id: c.id, status })
  }

  const failed = cases.isError || settlement.isError
  const name = settlement.data?.full_name

  return (
    <div className='space-y-4'>
      <div>
        <h1 className='text-xl font-semibold tracking-tight text-gray-900'>Мой кабинет</h1>
        <p className='mt-1 text-sm text-gray-500'>
          {name ? `${name} — ` : ''}свои дела, свои клиенты и свои деньги. Чужих дел и общих финансов фирмы здесь нет.
        </p>
      </div>

      {failed && (
        <div className='rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700'>
          Не удалось загрузить данные. Если это повторяется — возможно, логин не привязан к адвокату в ростере, скажи
          админу.
        </div>
      )}

      <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
        <StatTile label='Дел всего' value={String(stats.data?.cases ?? cases.data?.items.length ?? 0)} />
        <StatTile label='В работе' value={String(inProgress(stats.data))} />
        <StatTile label='Собрано по моим делам' value={money(settlement.data?.collected ?? 0)} />
        <StatTile
          label='Осталось получить мне'
          value={money(settlement.data?.outstanding ?? 0)}
          accent={(settlement.data?.outstanding ?? 0) > 0 ? 'good' : undefined}
        />
      </div>

      <Section title='Мои деньги' summary={<span className='text-sm text-gray-500'>{money(settlement.data?.outstanding ?? 0)}</span>}>
        {settlement.isLoading ? (
          <div className='text-sm text-gray-500'>Загрузка…</div>
        ) : settlement.data ? (
          <MoneyPanel data={settlement.data} />
        ) : (
          <div className='text-sm text-gray-500'>Нет данных.</div>
        )}
      </Section>

      <Section
        title='Мои дела'
        summary={
          <span className='text-sm text-gray-500'>
            {cases.data ? `${cases.data.items.length} · долг клиентов ${money(cases.data.total_owed)}` : ''}
          </span>
        }
      >
        {cases.isLoading ? (
          <div className='text-sm text-gray-500'>Загрузка…</div>
        ) : (
          <CasesTable cases={cases.data?.items ?? []} busyCaseID={busyCaseID} onSetStatus={changeStatus} />
        )}
      </Section>

      <Section title='Мои клиенты' summary={<span className='text-sm text-gray-500'>{clients.data?.items.length ?? 0}</span>} defaultOpen={false}>
        {clients.isLoading ? (
          <div className='text-sm text-gray-500'>Загрузка…</div>
        ) : (clients.data?.items.length ?? 0) === 0 ? (
          <div className='text-sm text-gray-500'>Клиентов пока нет — они появляются здесь вместе с делами.</div>
        ) : (
          <div className='-mx-6 overflow-x-auto sm:mx-0'>
            <table className='w-full min-w-max text-sm'>
              <thead>
                <tr className='border-b border-gray-200 text-left text-xs text-gray-500'>
                  <th className='py-2 pr-3 font-medium'>Клиент</th>
                  <th className='py-2 pr-3 font-medium'>Телефон</th>
                  <th className='py-2 pr-3 text-right font-medium'>Дел</th>
                  <th className='py-2 pr-3 text-right font-medium'>Гонорары</th>
                  <th className='py-2 pr-3 text-right font-medium'>Оплачено</th>
                  <th className='py-2 text-right font-medium'>Должен</th>
                </tr>
              </thead>
              <tbody>
                {clients.data!.items.map((c) => (
                  <tr key={c.id} className='border-b border-gray-100 transition-colors hover:bg-emerald-50'>
                    <td className='py-2 pr-3'>
                      <Link href={`/my/clients/${c.id}`} className='font-medium text-emerald-700 hover:underline'>
                        {c.name || 'без имени'}
                      </Link>
                    </td>
                    <td className='py-2 pr-3 whitespace-nowrap'>{c.phone || <span className='text-gray-400'>—</span>}</td>
                    <td className='py-2 pr-3 text-right tabular-nums'>{c.cases}</td>
                    <td className='py-2 pr-3 text-right tabular-nums'>{money(c.fee)}</td>
                    <td className='py-2 pr-3 text-right tabular-nums'>{money(c.paid)}</td>
                    <td className={'py-2 text-right tabular-nums ' + (c.owed > 0 ? 'text-rose-600' : 'text-gray-400')}>
                      {c.owed > 0 ? money(c.owed) : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <Section title='Статистика' defaultOpen={false}>
        {stats.isLoading ? (
          <div className='text-sm text-gray-500'>Загрузка…</div>
        ) : stats.data ? (
          <div className='space-y-4'>
            <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
              <StatTile label='Клиентов' value={String(stats.data.clients)} />
              <StatTile label='Средний гонорар' value={money(stats.data.avg_fee)} />
              <StatTile label='Гонорары по делам' value={money(stats.data.fee_total)} />
              <StatTile
                label='Должны клиенты'
                value={money(stats.data.client_debt)}
                accent={stats.data.client_debt > 0 ? 'bad' : undefined}
              />
            </div>
            <div className='flex flex-wrap gap-2'>
              {stats.data.by_status.map((s) => (
                <Badge key={s.status} variant={s.status === 'completed' ? 'success' : s.status === 'cancelled' ? 'neutral' : 'info'}>
                  {(CASE_STATUS_LABEL[s.status] ?? s.status) + ': ' + s.count}
                </Badge>
              ))}
            </div>
            {stats.data.last_payment_at && (
              <div className='text-xs text-gray-500'>Последняя оплата по моим делам: {stats.data.last_payment_at.slice(0, 10)}</div>
            )}
          </div>
        ) : (
          <div className='text-sm text-gray-500'>Нет данных.</div>
        )}
      </Section>
    </div>
  )
}

function inProgress(stats?: MyStats): number {
  return stats?.by_status.find((s) => s.status === 'in_progress')?.count ?? 0
}

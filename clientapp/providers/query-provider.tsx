'use client'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { StaleLaunchError, NotAClientError } from '@/lib/api'

export function QueryProvider({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // A dead launch and "the CRM does not know you" are both answers,
            // not failures — retrying either just burns a phone's battery.
            retry: (count, error) =>
              !(error instanceof StaleLaunchError) && !(error instanceof NotAClientError) && count < 2,
            staleTime: 30_000,
            refetchOnWindowFocus: false,
          },
        },
      })
  )
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

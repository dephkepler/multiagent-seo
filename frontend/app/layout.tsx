import type { Metadata } from 'next'
import { Toaster } from 'sonner'
import { QueryProvider } from '@/providers/query-provider'
import './globals.css'

export const metadata: Metadata = {
  title: 'multiagent-seo',
  description: 'SEO content generation & link building',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang='en'>
      <body className='antialiased'>
        <QueryProvider>
          <Toaster position='top-right' richColors />
          {children}
        </QueryProvider>
      </body>
    </html>
  )
}

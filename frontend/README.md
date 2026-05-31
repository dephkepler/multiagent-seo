# multiagent-seo — frontend

Next.js 16 + React 19 + Tailwind 4 + TanStack Query. MVP dashboard for the SEO
content-generation and link-building backend.

```bash
npm install
npm run dev   # http://localhost:3000
```

Set `NEXT_PUBLIC_API_URL` to point at the backend (default `http://localhost:8889`).
Auth is a bearer JWT issued by `POST /auth/login`; the token is kept in
`localStorage` and attached to every request.

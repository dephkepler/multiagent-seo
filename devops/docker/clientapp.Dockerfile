# The client Mini App: a static export served by Caddy, which also proxies /api
# to the backend. TLS is terminated by the host's front-door Caddy — see the
# Caddyfile for why this container has none.
#
# No Node in the final image. Every page is a client component authenticating
# with a signed launch the server here could not use anyway, so there is nothing
# to render at request time — and a static bundle behind Caddy is one less
# runtime to keep patched.
FROM node:24-alpine AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci || npm install

FROM node:24-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
# Baked in at build time, as every NEXT_PUBLIC_ value is. The default is a
# same-origin path, which is what the Caddyfile serves.
ARG NEXT_PUBLIC_API_BASE=/api
ARG NEXT_PUBLIC_TG_LINK=
ENV NEXT_PUBLIC_API_BASE=${NEXT_PUBLIC_API_BASE}
ENV NEXT_PUBLIC_TG_LINK=${NEXT_PUBLIC_TG_LINK}
RUN npm run build

FROM caddy:2-alpine AS runner
COPY --from=builder /app/out /srv
COPY Caddyfile /etc/caddy/Caddyfile
EXPOSE 3002

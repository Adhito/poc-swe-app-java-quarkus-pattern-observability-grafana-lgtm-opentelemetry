# Stage 1 — bundle the SPA (OTel JS + app) with esbuild
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY build.mjs ./
COPY src ./src
RUN npm run build

# Stage 2 — serve static assets with nginx; config.js is rendered at start
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
COPY config.js.template /etc/frontend/config.js.template
COPY docker-entrypoint.sh /docker-entrypoint-custom.sh
RUN chmod +x /docker-entrypoint-custom.sh
EXPOSE 80
# envsubst (gettext) ships in the nginx alpine image
ENTRYPOINT ["/docker-entrypoint-custom.sh"]

FROM node:24-alpine AS build

WORKDIR /src
COPY package.json package-lock.json tsconfig.json ./
COPY frontend ./frontend
RUN npm ci && npm run build --workspace @vps-billing/admin-web

FROM nginx:1.29-alpine
COPY deploy/nginx/admin-web.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/frontend/admin-web/dist /usr/share/nginx/html/admin
EXPOSE 80

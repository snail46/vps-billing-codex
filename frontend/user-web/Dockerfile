FROM node:24-alpine AS build

WORKDIR /src
COPY package.json package-lock.json tsconfig.json ./
COPY frontend ./frontend
RUN npm ci && npm run build --workspace @vps-billing/user-web

FROM nginx:1.29-alpine
COPY deploy/nginx/user-web.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/frontend/user-web/dist /usr/share/nginx/html
EXPOSE 80

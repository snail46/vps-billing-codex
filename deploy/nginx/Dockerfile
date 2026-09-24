FROM nginx:1.29-alpine

COPY deploy/nginx/gateway.conf /etc/nginx/conf.d/default.conf

EXPOSE 80 81

HEALTHCHECK --interval=10s --timeout=3s --retries=5 \
  CMD wget -qO- http://127.0.0.1/gateway-health >/dev/null || exit 1

FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine
ENV PODORA_BACKEND_URL=http://localhost:8080
# Only substitute our variable, leaving nginx's own $vars intact.
ENV NGINX_ENVSUBST_FILTER=PODORA_
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 3000

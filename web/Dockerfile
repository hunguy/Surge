FROM node:22-alpine AS build

WORKDIR /app

# Install first so the layer caches across source edits.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# -slim rather than the default nginx alpine: same nginx and same mime config,
# without the extra njs/dynamic-module packages the image never loads. 74MB -> 20MB.
FROM nginx:1.27-alpine-slim AS serve

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

# nginx always listens on 1777 here; the published host port is set in compose,
# and rewriting this one to match the host would leave the port mapping dangling.
EXPOSE 1777

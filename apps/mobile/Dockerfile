# Web build of the Billsplit client, served by nginx.
#
#   docker build -t billsplit-web --build-arg EXPO_PUBLIC_API_URL=https://api.example.com apps/mobile
#   docker run -p 8080:80 billsplit-web
#
# EXPO_PUBLIC_* variables are inlined into the JS bundle at build time.

# ---- stage 1: build ----
FROM node:22-alpine AS build
WORKDIR /app
ARG EXPO_PUBLIC_API_URL=http://localhost:8088
ENV EXPO_PUBLIC_API_URL=${EXPO_PUBLIC_API_URL} \
    CI=1 \
    EXPO_NO_TELEMETRY=1
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npx expo export --platform web --output-dir dist

# ---- stage 2: serve ----
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
# Expo copies public/ into dist/, but copy .well-known explicitly so the files are
# present even if that behaviour changes (dotfiles/dirs are easy to lose).
COPY public/.well-known /usr/share/nginx/html/.well-known
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

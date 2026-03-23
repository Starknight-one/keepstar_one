# Build frontend
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

# Production — serves API + static admin UI
FROM node:22-alpine
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install --omit=dev
COPY server.js ./
COPY --from=build /app/dist ./dist
ENV NODE_ENV=production
EXPOSE 3001
CMD ["node", "server.js"]

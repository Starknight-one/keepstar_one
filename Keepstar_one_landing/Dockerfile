# Build stage
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Serve stage — lightweight static server
FROM node:22-alpine
RUN npm install -g serve@14
WORKDIR /app
COPY --from=build /app/dist ./dist
EXPOSE 3000
CMD ["serve", "dist", "-l", "3000", "-s"]

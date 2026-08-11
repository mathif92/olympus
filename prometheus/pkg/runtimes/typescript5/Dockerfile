FROM node:20-alpine AS build
WORKDIR /app
COPY . /app
RUN npm install --global typescript@5.6.3 --silent && tsc -p /app/tsconfig.json

FROM node:20-alpine
WORKDIR /app
COPY --from=build /app/dist /app
CMD ["node", "/app/launch.js"]

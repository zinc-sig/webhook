FROM node:lts-alpine
ENV PORT=80

RUN set -ex && \
    apk add --no-cache --virtual unrar curl unzip

WORKDIR /usr/src/app
COPY package*.json ./
RUN npm install
COPY ./ .
RUN npm run build

CMD ["npm", "run", "start"]

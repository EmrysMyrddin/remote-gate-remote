FROM golang:1.22 as builder

ENV NODE_VERSION 22.6.0

RUN apt update && apt install xz-utils
RUN curl -fsSLO --compressed -o "node.tar.xz" "https://nodejs.org/dist/v$NODE_VERSION/node-v$NODE_VERSION-linux-arm64.tar.xz"
RUN tar -xJf "./node-v$NODE_VERSION-linux-arm64.tar.xz" --strip-components=1 --no-same-owner
RUN ln -s /usr/local/bin/node /usr/local/bin/nodejs
RUN corepack enable

WORKDIR /usr/src/app

COPY package.json yarn.lock .yarnrc.yml ./
RUN yarn install

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/src/app/build/ -a -ldflags '-extldflags "-static"' ./...

FROM scratch
COPY --from=builder /usr/src/app/build/cmd /app
COPY --from=builder /usr/src/app/static /static
COPY --from=builder /usr/src/app/static/js/htmx.min.js /static/js/htmx.min.js

EXPOSE 80
CMD ["/app"]
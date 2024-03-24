FROM golang:1.22 as builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -o /usr/src/app/app -a -ldflags '-extldflags "-static"' ./...

FROM scratch
COPY --from=builder /usr/src/app/app /app
COPY views /views

CMD ["/app"]
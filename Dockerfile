FROM golang:latest

WORKDIR /api

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY server/*.go ./

RUN go build -o /bin/api-server

EXPOSE 8080

ENTRYPOINT ["/bin/api-server"]
CMD ["-h"]

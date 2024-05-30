FROM golang:1.22-alpine

WORKDIR /app

COPY . ./

RUN go mod download \
  && go build -o /app/cider \
  && go clean -cache -modcache

ENTRYPOINT [ "/app/cider"]

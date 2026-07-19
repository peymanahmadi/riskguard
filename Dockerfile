FROM golang:1.26.5-alpine AS build
WORKDIR /src

# Use the proxy in China or a backup proxy
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/riskguard-server ./cmd/server

FROM scratch
COPY --from=build /out/riskguard-server /riskguard-server
EXPOSE 8080
ENTRYPOINT ["/riskguard-server"]
# le-ezviz-vs bundled with MediaMTX, ffmpeg and cctvd so the cameras can be watched over HLS, see docker/ and docker-compose.yaml
FROM golang:1.27-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/le-ezviz-vs . && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cctvd ./cmd/cctvd

FROM bluenviron/mediamtx:1.21.2 AS mediamtx

FROM alpine:3.24
RUN apk add --no-cache bash ca-certificates ffmpeg tzdata
COPY --from=mediamtx /mediamtx /usr/local/bin/mediamtx
COPY --from=build /out/le-ezviz-vs /out/cctvd /usr/local/bin/
COPY docker/entrypoint.sh docker/camera.sh /app/
RUN chmod +x /app/entrypoint.sh /app/camera.sh
ENV EZVIZ_REGION=Asia \
    TZ=Asia/Makassar
WORKDIR /data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]

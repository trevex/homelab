FROM golang:1.22 AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /vxlan

FROM debian:12
ARG DEBIAN_FRONTEND="noninteractive"
WORKDIR /
COPY --from=build /vxlan /vxlan
COPY entrypoint.sh /entrypoint.sh
RUN chmod u+x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]

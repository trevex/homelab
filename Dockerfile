FROM golang:1.22 AS build

WORKDIR /app
COPY vxlan-cni/go.mod vxlan-cni/go.sum ./
RUN go mod download
COPY vxlan-cni/*.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /vxlan

FROM debian:12
ARG DEBIAN_FRONTEND="noninteractive"
WORKDIR /
COPY --from=build /vxlan /vxlan
COPY vxlan-cni/entrypoint.sh /entrypoint.sh
RUN chmod u+x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]

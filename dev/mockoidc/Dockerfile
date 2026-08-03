# The dev identity provider, for the all-in-Docker loop. Its own module, so
# gvisor and the tailscale netstack never enter Headboard's go.sum.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mockoidc .

FROM alpine:3.22
COPY --from=build /out/mockoidc /usr/local/bin/mockoidc
EXPOSE 9998
ENTRYPOINT ["mockoidc"]

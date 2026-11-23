FROM golang:1.26.4 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY auth/ auth/
COPY handlers/ handlers/
COPY k8s/ k8s/
COPY persist/ persist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/roudash .
RUN mkdir -m 700 /out/data

FROM gcr.io/distroless/static-debian12:nonroot
ENV PORT=8080 LISTEN_ADDR=0.0.0.0
EXPOSE 8080
ENTRYPOINT ["/app/roudash"]
COPY --from=build /out/roudash /app/roudash
COPY --from=build --chown=65532:65532 /out/data /data

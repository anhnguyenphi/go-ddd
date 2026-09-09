# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.26.7 AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG TARGET=api
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /out/app ./cmd/${TARGET}

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/app /app/app
COPY configs /app/configs
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/app"]

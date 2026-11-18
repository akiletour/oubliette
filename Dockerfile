FROM --platform=$BUILDPLATFORM node:24-alpine AS interface
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS binary
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=interface /src/web/dist web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /oubliette . && mkdir /data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=binary /oubliette /usr/local/bin/oubliette
COPY --from=binary --chown=nonroot:nonroot /data /data
ENV OUBLIETTE_DATA=/data/oubliette.db
VOLUME /data
EXPOSE 8080 1025
ENTRYPOINT ["oubliette"]
CMD ["serve"]

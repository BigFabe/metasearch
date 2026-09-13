FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags="-s -w" -o /metasearch .

FROM scratch
COPY --from=build /metasearch /metasearch
USER 65532:65532
ENV GOTRACEBACK=none
EXPOSE 8080
ENTRYPOINT ["/metasearch"]
CMD ["-config", "/config/config.json", "-listen", ":8080"]

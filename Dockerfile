FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/work-graph ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/work-graph-runner ./cmd/runner
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/work-graph-bash-runner ./cmd/bashrunner
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/work-graph-gateway ./cmd/gateway

FROM build AS test
RUN go test ./...

FROM alpine:3.23
RUN addgroup -S workgraph && adduser -S workgraph -G workgraph
COPY --from=build /out/work-graph /usr/local/bin/work-graph
COPY --from=build /out/work-graph-runner /usr/local/bin/work-graph-runner
COPY --from=build /out/work-graph-bash-runner /usr/local/bin/work-graph-bash-runner
COPY --from=build /out/work-graph-gateway /usr/local/bin/work-graph-gateway
USER workgraph
EXPOSE 8080
ENTRYPOINT ["work-graph"]

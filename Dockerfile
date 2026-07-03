FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /relampo-tickets .

FROM scratch
COPY --from=build /relampo-tickets /relampo-tickets
EXPOSE 8080
ENTRYPOINT ["/relampo-tickets"]

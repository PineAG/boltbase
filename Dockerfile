FROM golang:alpine AS build
COPY . /app
WORKDIR /app
RUN go install
RUN go build boltbase.go
USER 1000:1000

FROM alpine:latest
RUN mkdir /app
RUN mkdir /data
COPY --chown=1000:1000 --from=build /app/boltbase /app/boltbase
RUN chown -R 1000:1000 /app
RUN chown -R 1000:1000 /data
USER 1000:1000
ENV DB_ROOT=/data
ENV DB_FILE=store.db
ENV PORT=3000
EXPOSE 3000
CMD ["/app/boltbase"]
FROM golang:1.20-bullseye as build
RUN apt update && apt install -y libolm3 libolm-dev

WORKDIR /src
COPY . ./
RUN go mod download
RUN go build -o /matrix-gptzoo ./main.go

FROM debian:bullseye
RUN apt update && apt install -y libolm3 ca-certificates openssl

COPY --from=build /matrix-gptzoo /matrix-gptzoo
CMD /matrix-gptzoo
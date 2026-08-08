FROM btwiuse/arch:golang AS builder

COPY . /app

WORKDIR /app

ENV GONOSUMDB="*"

RUN go mod tidy

RUN CGO_ENABLED=0 GOBIN=/usr/local/bin go install -v ./cmd/agw

FROM btwiuse/arch

WORKDIR /app

COPY --from=builder /usr/local/bin/agw /usr/bin/agw

EXPOSE 8080

CMD ["agw"]

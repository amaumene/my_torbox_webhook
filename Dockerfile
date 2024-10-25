FROM registry.access.redhat.com/ubi9/go-toolset AS builder

COPY ./go.mod .
COPY ./main.go .

RUN go build

FROM registry.access.redhat.com/ubi9/ubi-minimal

COPY --from=builder /opt/app-root/src/my_torbox_webhook /app/my_torbox_webhook

USER 1001

VOLUME /data
EXPOSE 3000/tcp

CMD [ "/app/my_torbox_webhook" ]

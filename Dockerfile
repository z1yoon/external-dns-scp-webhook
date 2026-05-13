FROM gcr.io/distroless/static:nonroot
COPY webhook /webhook
ENTRYPOINT ["/webhook"]

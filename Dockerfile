FROM alpine:latest

RUN apk add --no-cache ca-certificates e2fsprogs findmnt
ADD csi-upcloud-plugin /
ENTRYPOINT ["/csi-upcloud-plugin"]
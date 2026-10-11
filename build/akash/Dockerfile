# The tested upstream runtime, with only the controller binary replaced.
FROM ghcr.io/nginx/nginx-gateway-fabric:2.7.2@sha256:8c966cbb00ec379edb815b25663a71186f78201fd7a54b668bf1588182c729cd
ARG TARGETARCH
COPY --chmod=0755 build/out/${TARGETARCH}/gateway /usr/bin/gateway

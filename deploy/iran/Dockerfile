FROM nginx:1.27-alpine

# Replaces the image's welcome server. Traefik reaches port 80 on the
# Coolify network; this image publishes nothing on the host.
COPY nginx-coolify.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

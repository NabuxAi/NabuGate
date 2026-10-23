FROM nginx:1.27-alpine

# Replaces the image's welcome server with a template the image renders at
# start (docker-entrypoint.d/20-envsubst-on-templates.sh). Which template set
# is rendered is NGINX_ENVSUBST_TEMPLATE_DIR: templates/ passes requests
# through, templates-rewrite/ also rewrites the upstream's absolute URLs.
RUN rm -f /etc/nginx/conf.d/default.conf
COPY templates/ /etc/nginx/templates/
COPY templates-rewrite/ /etc/nginx/templates-rewrite/

# Without overrides this is the NabuGate mirror it has always been.
ENV UPSTREAM_HOST=gate.nabuxai.com \
    MIRROR_HOST=gate.nabuxai.ir \
    NGINX_ENVSUBST_TEMPLATE_DIR=/etc/nginx/templates

# Traefik reaches port 80 on the Coolify network; this image publishes
# nothing on the host.
EXPOSE 80
